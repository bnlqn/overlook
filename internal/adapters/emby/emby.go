// Package emby reads Emby, a media server: what each person is playing and
// the plays of the last day, as any media server's (snapshot.MediaServer);
// and the films, shows and music it holds, the failed logins of the last
// day, and the tasks whose last run failed. Emby doesn't say how much room
// is left on its disks, as Jellyfin does, so this adapter shows none.
package emby

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

// Area is the adapter's home area.
const Area = "apps"

// Config is one Emby, under sources.emby in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Emby, such as http://emby:8096.
	URL string `yaml:"url"`
	// Key is an API key, from the dashboard's API Keys. It can do anything
	// an administrator can, and no lesser one exists; Overlook's GET-only
	// reads keep it harmless (ADR 0001).
	Key string `yaml:"key"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: http.Header{"X-Emby-Token": {c.Key}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Emby")
	return nil
}

type info struct {
	Version           string `json:"Version"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
}

// session is one of /Sessions: a device someone uses Emby on, and what
// it plays, if anything.
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

// entry is one line of Emby's activity log, worded by Emby.
type entry struct {
	Name string    `json:"Name"`
	Type string    `json:"Type"`
	Date time.Time `json:"Date"`
}

// Read reads the Emby at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var (
		sys      info
		sessions []session
		n        counts
		tasks    []task
		log      struct {
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
		{"/System/ActivityLog/Entries?" + day.Encode(), &log},
	} {
		if err := c.source.JSON(ctx, r.path, r.v); err != nil {
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
			what = snapshot.Text{Key: "emby.episode", Args: map[string]any{"series": s.NowPlayingItem.SeriesName, "name": s.NowPlayingItem.Name}}
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
		{"emby.films", n.MovieCount, map[string]any{"n": n.MovieCount}},
		{"emby.shows", n.SeriesCount, map[string]any{"n": n.SeriesCount, "episodes": n.EpisodeCount}},
		{"emby.music", n.SongCount, map[string]any{"n": n.AlbumCount, "songs": n.SongCount}},
		{"emby.books", n.BookCount, map[string]any{"n": n.BookCount}},
	} {
		if k.has > 0 {
			m.Rows = append(m.Rows, snapshot.Row{Title: snapshot.Text{Key: k.key}, Value: snapshot.Msg(k.key+"_n", k.args)})
		}
	}

	// The plays of the last day, and the failed logins, which Emby words
	// itself. Emby logs a play when it starts and again when it stops, and
	// only the start is a play.
	var failed []entry
	for _, e := range log.Items {
		switch e.Type {
		case "playback.start":
			m.Plays = append(m.Plays, e.Date)
		case "user.authenticationfailed":
			failed = append(failed, e)
		}
	}
	if len(failed) > 0 {
		newest := slices.MaxFunc(failed, func(a, b entry) int { return a.Date.Compare(b.Date) })
		m.Rows = append(m.Rows, snapshot.Row{
			Title: snapshot.Text{Key: "emby.failed_logins"}, Tone: "orange",
			Value:  snapshot.Msg("emby.failed_logins_n", map[string]any{"n": len(failed)}),
			Detail: &snapshot.Text{Name: newest.Name},
		})
	}

	// The tasks whose last run failed, such as a scan of a library, as
	// Emby words why.
	for _, t := range tasks {
		r := t.LastExecutionResult
		if r == nil || r.Status != "Failed" {
			continue
		}
		row := snapshot.Row{Title: snapshot.Text{Name: t.Name}, Value: snapshot.Msg("emby.task_failed", nil), Tone: "red"}
		row.Detail = snapshot.Msg("emby.task_failed_at", map[string]any{"at": r.EndTimeUtc})
		if r.ErrorMessage != "" {
			row.Detail = &snapshot.Text{Name: r.ErrorMessage}
		}
		m.Rows = append(m.Rows, row)
	}

	version := snapshot.Row{Title: snapshot.Text{Key: "emby.version"}, Value: &snapshot.Text{Name: sys.Version}}
	if sys.HasPendingRestart {
		version.Value, version.Tone = snapshot.Msg("emby.restart", nil), "orange"
		version.Detail = &snapshot.Text{Name: sys.Version}
	}
	m.Rows = append(m.Rows, version)
	return m.Parts(Area, now), nil
}
