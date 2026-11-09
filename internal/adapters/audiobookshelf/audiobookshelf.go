// Package audiobookshelf reads Audiobookshelf, the audiobooks and podcasts:
// who listens now, each library by what it holds and the items gone from
// it, the episodes waiting to download, and today's listening, by person.
package audiobookshelf

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "audiobookshelf", Area: Area, Read: Read}

// reads are the only paths the adapter asks Audiobookshelf for, each of
// which reads, with * for a library's id. Its admin's key, which the
// sessions need, may also GET paths that change things, such as
// /api/backups/:id/apply, which restores a backup, and
// /api/libraries/:id/matchall, so this list, not the GET-only client, is
// what keeps the key harmless (ADR 0011).
var reads = []string{
	"/api/libraries", "/api/sessions/open", "/api/sessions",
	"/api/libraries/*/items", "/api/libraries/*/episode-downloads",
}

// Config is one Audiobookshelf, under sources.audiobookshelf in
// overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Audiobookshelf, such as
	// http://audiobookshelf, on its container's port 80.
	URL string `yaml:"url" setup:"text" example:"http://audiobookshelf.example.com:13378"`
	// Key is an API key, from Settings, API Keys, for an admin of
	// Overlook's own, with no expiry.
	Key string `yaml:"key" setup:"key"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: http.Header{"Authorization": {"Bearer " + c.Key}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Audiobookshelf")
	return nil
}

// get reads the answer at p, one of reads, with its query, into v.
func (c Config) get(ctx context.Context, p, query string, v any) error {
	if !slices.ContainsFunc(reads, func(r string) bool { ok, _ := path.Match(r, p); return ok }) {
		return fmt.Errorf("%s isn't a path that reads", p)
	}
	return c.source.JSON(ctx, p+query, v)
}

type library struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"` // book or podcast
	Stats     struct {
		Items    int64   `json:"totalItems"`
		Files    int64   `json:"numAudioFiles"`
		Duration float64 `json:"totalDuration"` // seconds
		Size     int64   `json:"totalSize"`     // bytes
	} `json:"stats"`
}

// session is someone listening to a book or an episode, from their first
// play to their last, on one of their devices.
type session struct {
	User struct {
		Name string `json:"username"`
	} `json:"user"`
	Title     string  `json:"displayTitle"`
	Listening float64 `json:"timeListening"` // seconds
	Updated   int64   `json:"updatedAt"`     // milliseconds
	Device    struct {
		Name   string `json:"deviceName"`
		Client string `json:"clientName"`
	} `json:"deviceInfo"`
}

// download is an episode Audiobookshelf downloads or waits to.
type download struct {
	Podcast string `json:"podcastTitle"`
	Episode string `json:"episodeDisplayTitle"`
}

// Read reads the Audiobookshelf at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var (
		libs struct {
			Libraries []library `json:"libraries"`
		}
		open, recent struct {
			Sessions []session `json:"sessions"`
		}
	)
	// ponytail: the newest hundred sessions, which hold a day's for a
	// household; a homelab that starts more a day needs a second page.
	for _, r := range []struct {
		path, query string
		v           any
	}{
		{"/api/libraries", "?include=stats", &libs},
		{"/api/sessions/open", "", &open},
		{"/api/sessions", "?itemsPerPage=100&sort=updatedAt&desc=1", &recent},
	} {
		if err := c.get(ctx, r.path, r.query, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	// Who listens now, by person, and on what.
	slices.SortFunc(open.Sessions, func(a, b session) int { return cmp.Compare(a.User.Name, b.User.Name) })
	var rows []snapshot.Row
	for _, s := range open.Sessions {
		if s.User.Name == "" || s.Title == "" {
			continue
		}
		r := snapshot.Row{Title: snapshot.Text{Name: s.User.Name}, Value: &snapshot.Text{Name: s.Title}}
		if d := cmp.Or(s.Device.Name, s.Device.Client); d != "" {
			r.Detail = &snapshot.Text{Name: d}
		}
		rows = append(rows, r)
	}

	// Each library, by what it holds, red with items gone from the disk or
	// that it can't read; then a podcast library's episodes waiting.
	var (
		downloads   []download
		downloading = map[string]string{} // podcast: the episode downloading
	)
	for _, l := range libs.Libraries {
		if l.ID == "" {
			continue
		}
		id := "/api/libraries/" + url.PathEscape(l.ID)
		var issues struct {
			Total int64 `json:"total"`
		}
		if err := c.get(ctx, id+"/items", "?filter=issues&limit=1&minified=1", &issues); err != nil {
			return snapshot.Parts{}, err
		}
		length := map[string]any{"n": l.Stats.Files, "length": l.Stats.Duration * 1000, "size": l.Stats.Size}
		r := snapshot.Row{Title: snapshot.Text{Name: l.Name}, Value: snapshot.Msg("audiobookshelf.books", map[string]any{"n": l.Stats.Items}), Detail: snapshot.Msg("audiobookshelf.length", length)}
		if l.MediaType == "podcast" {
			r.Value, r.Detail = snapshot.Msg("audiobookshelf.podcasts", map[string]any{"n": l.Stats.Items}), snapshot.Msg("audiobookshelf.episodes", length)
			var queue struct {
				Current *download  `json:"currentDownload"`
				Queue   []download `json:"queue"`
			}
			if err := c.get(ctx, id+"/episode-downloads", "", &queue); err != nil {
				return snapshot.Parts{}, err
			}
			if queue.Current != nil {
				downloads = append(downloads, *queue.Current)
				downloading[queue.Current.Podcast] = queue.Current.Episode
			}
			downloads = append(downloads, queue.Queue...)
		}
		if issues.Total > 0 {
			r.Detail, r.Tone = snapshot.Msg("audiobookshelf.issues", map[string]any{"n": issues.Total}), "red"
		}
		rows = append(rows, r)
	}

	// The episodes waiting, by podcast, the first of them downloading.
	var podcasts []string
	waiting := map[string][]string{}
	for _, d := range downloads {
		if d.Podcast == "" {
			continue
		}
		if waiting[d.Podcast] == nil {
			podcasts = append(podcasts, d.Podcast)
		}
		waiting[d.Podcast] = append(waiting[d.Podcast], d.Episode)
	}
	for _, p := range podcasts {
		r := snapshot.Row{Title: snapshot.Text{Name: p}, Value: snapshot.Msg("audiobookshelf.to_download", map[string]any{"n": len(waiting[p])})}
		if e := downloading[p]; e != "" {
			r.Detail = snapshot.Msg("audiobookshelf.downloading", map[string]any{"title": e})
		}
		rows = append(rows, r)
	}

	// Today's listening, since midnight where Overlook runs, by person and
	// what they listened to.
	// ponytail: a session counts whole on the day it last played, so one
	// that runs past midnight counts all on the second day; split it at
	// midnight if an owner ever minds.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	type listen struct{ who, title string }
	var (
		everyone float64
		listens  []listen
		seconds  = map[listen]float64{}
	)
	for _, s := range recent.Sessions {
		if time.UnixMilli(s.Updated).Before(midnight) || s.Listening <= 0 {
			continue
		}
		everyone += s.Listening
		k := listen{s.User.Name, s.Title}
		if k.who == "" {
			continue
		}
		if _, ok := seconds[k]; !ok {
			listens = append(listens, k)
		}
		seconds[k] += s.Listening
	}
	var people []string
	minutes := map[string]float64{}
	heard := map[string][]snapshot.Row{}
	for _, k := range listens {
		if _, ok := minutes[k.who]; !ok {
			people = append(people, k.who)
		}
		minutes[k.who] += seconds[k] / 60
		if k.title != "" {
			heard[k.who] = append(heard[k.who], snapshot.Row{Title: snapshot.Text{Name: k.title}, Value: snapshot.Msg("audiobookshelf.span", map[string]any{"ms": seconds[k] * 1000})})
		}
	}
	figure := snapshot.Figure{
		Area: Area, Icon: "play", Label: snapshot.Text{Key: "audiobookshelf.listened"},
		Value: math.Round(everyone / 60), Unit: snapshot.Msg("audiobookshelf.minutes", nil),
		Detail: snapshot.Msg("audiobookshelf.listening_now", map[string]any{"n": len(open.Sessions)}),
	}
	if len(people) > 1 {
		slices.Sort(people)
		for _, p := range people {
			figure.Shares = append(figure.Shares, snapshot.Share{Label: snapshot.Text{Name: p}, Value: math.Round(minutes[p]), Rows: heard[p]})
		}
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "audiobookshelf.title"}, Rows: rows}},
		Figures:  []snapshot.Figure{figure},
	}, nil
}
