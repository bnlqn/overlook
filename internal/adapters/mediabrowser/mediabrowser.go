// Package mediabrowser is what the Jellyfin and Emby adapters share, because
// Jellyfin is built on Emby's code, from when Emby was Media Browser, and the
// two still answer alike. It holds their config and key, and reads what both
// give the same way: what each person is playing and the plays of the last
// day, as any media server's (snapshot.MediaServer), the films, shows and
// music it holds, the failed logins of the last day, and the tasks whose last
// run failed.
package mediabrowser

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the home area of both adapters.
const Area = "apps"

// Config is one Jellyfin or Emby.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads it, such as http://jellyfin:8096.
	URL string `yaml:"url" setup:"text"`
	// Key is an API key, from the dashboard's API Keys. It can do anything
	// an administrator can, and no lesser one exists; Overlook's GET-only
	// reads keep it harmless (ADR 0001).
	Key string `yaml:"key" setup:"key"`

	source fetch.Source
}

// Check fills c's defaults, naming it name if the config doesn't, and
// refuses what can't work. header sends the key, the way the source takes
// it.
func (c *Config) Check(name string, header http.Header) error {
	c.source = fetch.Source{URL: c.URL, Header: header}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, name)
	return nil
}

// Get reads the answer at path, such as /System/Info, into v.
func (c Config) Get(ctx context.Context, path string, v any) error {
	return c.source.JSON(ctx, path, v)
}

// Log is which entries of the source's activity log are a play and which a
// failed login, by their type, which Jellyfin and Emby each name their own
// way.
type Log struct {
	Plays  []string
	Failed string
}

type info struct {
	Version           string `json:"Version"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
}

// session is one of /Sessions: a device someone uses the source on, and
// what it plays, if anything.
type session struct {
	UserName       string `json:"UserName"`
	DeviceName     string `json:"DeviceName"`
	NowPlayingItem *struct {
		Name       string `json:"Name"`
		SeriesName string `json:"SeriesName"`
	} `json:"NowPlayingItem"`
	PlayState struct {
		IsPaused   bool   `json:"IsPaused"`
		PlayMethod string `json:"PlayMethod"`
	} `json:"PlayState"`
}

type counts struct {
	MovieCount, SeriesCount, EpisodeCount, AlbumCount, SongCount, BookCount int
}

type task struct {
	Name                string `json:"Name"`
	LastExecutionResult *struct {
		Status       string    `json:"Status"`
		EndTimeUtc   time.Time `json:"EndTimeUtc"`
		ErrorMessage string    `json:"ErrorMessage"`
	} `json:"LastExecutionResult"`
}

// entry is one line of the source's activity log, worded by the source.
type entry struct {
	Name string    `json:"Name"`
	Type string    `json:"Type"`
	Date time.Time `json:"Date"`
}

// Read reads the Jellyfin or Emby at c, which Check has filled, its activity
// log by log's types, and gives back its parts as a media server's, with
// own, the adapter's own rows, after what it holds.
func Read(ctx context.Context, c Config, now time.Time, log Log, own []snapshot.Row) (snapshot.Parts, error) {
	var (
		sys      info
		sessions []session
		n        counts
		tasks    []task
		entries  struct {
			Items []entry `json:"Items"`
		}
	)
	day := url.Values{"minDate": {now.Add(-24 * time.Hour).UTC().Format(time.RFC3339)}}
	for _, r := range []struct {
		path string
		v    any
	}{
		{"/System/Info", &sys},
		{"/Sessions?activeWithinSeconds=960", &sessions},
		{"/Items/Counts", &n},
		{"/ScheduledTasks?isHidden=false", &tasks},
		{"/System/ActivityLog/Entries?" + day.Encode(), &entries},
	} {
		if err := c.Get(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	var m snapshot.MediaServer
	for _, s := range sessions {
		if s.NowPlayingItem == nil {
			continue
		}
		what := snapshot.Text{Name: s.NowPlayingItem.Name}
		if s.NowPlayingItem.SeriesName != "" {
			what = snapshot.Text{Key: "mediabrowser.episode", Args: map[string]any{"series": s.NowPlayingItem.SeriesName, "name": s.NowPlayingItem.Name}}
		}
		m.Playing = append(m.Playing, snapshot.Play{
			Who: s.UserName, Device: s.DeviceName, What: what,
			Paused: s.PlayState.IsPaused, Transcoding: s.PlayState.PlayMethod == "Transcode",
		})
	}

	// What it holds, leaving out films, shows, music or books it has none
	// of.
	for _, k := range []struct {
		key  string
		has  int
		args map[string]any
	}{
		{"mediabrowser.films", n.MovieCount, map[string]any{"n": n.MovieCount}},
		{"mediabrowser.shows", n.SeriesCount, map[string]any{"n": n.SeriesCount, "episodes": n.EpisodeCount}},
		{"mediabrowser.music", n.SongCount, map[string]any{"n": n.AlbumCount, "songs": n.SongCount}},
		{"mediabrowser.books", n.BookCount, map[string]any{"n": n.BookCount}},
	} {
		if k.has > 0 {
			m.Rows = append(m.Rows, snapshot.Row{Title: snapshot.Text{Key: k.key}, Value: snapshot.Msg(k.key+"_n", k.args)})
		}
	}
	m.Rows = append(m.Rows, own...)

	// The plays of the last day, and the failed logins, which the source
	// words itself.
	var failed []entry
	for _, e := range entries.Items {
		switch {
		case slices.Contains(log.Plays, e.Type):
			m.Plays = append(m.Plays, e.Date)
		case e.Type == log.Failed:
			failed = append(failed, e)
		}
	}
	if len(failed) > 0 {
		newest := slices.MaxFunc(failed, func(a, b entry) int { return a.Date.Compare(b.Date) })
		m.Rows = append(m.Rows, snapshot.Row{
			Title: snapshot.Text{Key: "mediabrowser.failed_logins"}, Tone: "orange",
			Value:  snapshot.Msg("mediabrowser.failed_logins_n", map[string]any{"n": len(failed)}),
			Detail: &snapshot.Text{Name: newest.Name},
		})
	}

	// The tasks whose last run failed, such as a scan of a library, as the
	// source words why.
	for _, t := range tasks {
		r := t.LastExecutionResult
		if r == nil || r.Status != "Failed" {
			continue
		}
		row := snapshot.Row{Title: snapshot.Text{Name: t.Name}, Value: snapshot.Msg("mediabrowser.task_failed", nil), Tone: "red"}
		row.Detail = snapshot.Msg("mediabrowser.task_failed_at", map[string]any{"at": r.EndTimeUtc})
		if r.ErrorMessage != "" {
			row.Detail = &snapshot.Text{Name: r.ErrorMessage}
		}
		m.Rows = append(m.Rows, row)
	}

	version := snapshot.VersionRow(sys.Version, "")
	if sys.HasPendingRestart {
		version.Value, version.Tone = snapshot.Msg("mediabrowser.restart", nil), "orange"
		version.Detail = &snapshot.Text{Name: sys.Version}
	}
	m.Rows = append(m.Rows, version)
	return m.Parts(Area, now), nil
}
