// Package jellyfin reads Jellyfin, a media server: what each person is
// playing and the plays of the last day, as any media server's
// (snapshot.MediaServer); and the films, shows and music it holds, the room
// left on their disks, the failed logins of the last day, and the tasks
// whose last run failed.
package jellyfin

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Config is one Jellyfin, under sources.jellyfin in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Jellyfin, such as http://jellyfin:8096.
	URL string `yaml:"url"`
	// Key is an API key, from the dashboard's API Keys. It can do anything
	// an administrator can, and no lesser one exists; Overlook's GET-only
	// reads keep it harmless (ADR 0001).
	Key string `yaml:"key"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Jellyfin")
	return nil
}

// get reads the answer at Jellyfin's path into v, with the key.
func (c Config) get(ctx context.Context, path string, v any) error {
	header := http.Header{"Authorization": {`MediaBrowser Token="` + c.Key + `"`}}
	return fetch.JSON(ctx, strings.TrimSuffix(c.URL, "/")+path, header, v)
}

type info struct {
	Version           string `json:"Version"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
}

// storage is where each library's folders are, and the room on their disks.
type storage struct {
	Libraries []struct {
		Folders []struct {
			FreeSpace int64  `json:"FreeSpace"`
			UsedSpace int64  `json:"UsedSpace"`
			DeviceID  string `json:"DeviceId"`
		} `json:"Folders"`
	} `json:"Libraries"`
}

// session is one of /Sessions: a device someone uses Jellyfin on, and what
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

// entry is one line of Jellyfin's activity log, worded by Jellyfin.
type entry struct {
	Name string    `json:"Name"`
	Type string    `json:"Type"`
	Date time.Time `json:"Date"`
}

// Read reads the Jellyfin at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var (
		sys      info
		disks    storage
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
		{"/System/Info/Storage", &disks},
		{"/Sessions?activeWithinSeconds=960", &sessions},
		{"/Items/Counts", &n},
		{"/ScheduledTasks?isHidden=false", &tasks},
		{"/System/ActivityLog/Entries?" + day.Encode(), &log},
	} {
		if err := c.get(ctx, r.path, r.v); err != nil {
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
			what = snapshot.Text{Key: "jellyfin.episode", Args: map[string]any{"series": s.NowPlayingItem.SeriesName, "name": s.NowPlayingItem.Name}}
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
		{"jellyfin.films", n.MovieCount, map[string]any{"n": n.MovieCount}},
		{"jellyfin.shows", n.SeriesCount, map[string]any{"n": n.SeriesCount, "episodes": n.EpisodeCount}},
		{"jellyfin.music", n.SongCount, map[string]any{"n": n.AlbumCount, "songs": n.SongCount}},
		{"jellyfin.books", n.BookCount, map[string]any{"n": n.BookCount}},
	} {
		if k.has > 0 {
			m.Rows = append(m.Rows, snapshot.Row{Title: snapshot.Text{Key: k.key}, Value: snapshot.Msg(k.key+"_n", k.args)})
		}
	}

	// The room on each disk the libraries are on, once each, by where it's
	// mounted.
	var seen []string
	for _, l := range disks.Libraries {
		for _, f := range l.Folders {
			if slices.Contains(seen, f.DeviceID) || f.FreeSpace+f.UsedSpace <= 0 {
				continue
			}
			seen = append(seen, f.DeviceID)
			m.Rows = append(m.Rows, snapshot.Row{
				Title: snapshot.Text{Name: f.DeviceID},
				Value: snapshot.Msg("jellyfin.free", map[string]any{"n": f.FreeSpace}),
				Meter: &snapshot.Meter{Used: float64(f.UsedSpace), Total: float64(f.FreeSpace + f.UsedSpace)},
			})
		}
	}

	// The plays of the last day, and the failed logins, which Jellyfin
	// words itself.
	var failed []entry
	for _, e := range log.Items {
		switch e.Type {
		case "VideoPlayback", "AudioPlayback":
			m.Plays = append(m.Plays, e.Date)
		case "AuthenticationFailed":
			failed = append(failed, e)
		}
	}
	if len(failed) > 0 {
		newest := slices.MaxFunc(failed, func(a, b entry) int { return a.Date.Compare(b.Date) })
		m.Rows = append(m.Rows, snapshot.Row{
			Title: snapshot.Text{Key: "jellyfin.failed_logins"}, Tone: "orange",
			Value:  snapshot.Msg("jellyfin.failed_logins_n", map[string]any{"n": len(failed)}),
			Detail: &snapshot.Text{Name: newest.Name},
		})
	}

	// The tasks whose last run failed, such as a scan of a library, as
	// Jellyfin words why.
	for _, t := range tasks {
		r := t.LastExecutionResult
		if r == nil || r.Status != "Failed" {
			continue
		}
		row := snapshot.Row{Title: snapshot.Text{Name: t.Name}, Value: snapshot.Msg("jellyfin.task_failed", nil), Tone: "red"}
		row.Detail = snapshot.Msg("jellyfin.task_failed_at", map[string]any{"at": r.EndTimeUtc})
		if r.ErrorMessage != "" {
			row.Detail = &snapshot.Text{Name: r.ErrorMessage}
		}
		m.Rows = append(m.Rows, row)
	}

	version := snapshot.Row{Title: snapshot.Text{Key: "jellyfin.version"}, Value: &snapshot.Text{Name: sys.Version}}
	if sys.HasPendingRestart {
		version.Value, version.Tone = snapshot.Msg("jellyfin.restart", nil), "orange"
		version.Detail = &snapshot.Text{Name: sys.Version}
	}
	m.Rows = append(m.Rows, version)
	return m.Parts(Area, now), nil
}
