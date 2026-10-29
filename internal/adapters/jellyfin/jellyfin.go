// Package jellyfin reads Jellyfin: what each person is playing, the films,
// shows and music it holds, the room left on their disks, the plays and
// failed logins of the last day, and the tasks whose last run failed.
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

	// What each person plays now, by person: paused in grey, and how it
	// reaches them, as it is or transcoded, which takes the processor.
	slices.SortFunc(sessions, func(a, b session) int {
		return cmp.Or(cmp.Compare(a.UserName, b.UserName), cmp.Compare(a.DeviceName, b.DeviceName))
	})
	var rows []snapshot.Row
	playing := 0
	for _, s := range sessions {
		if s.NowPlayingItem == nil {
			continue
		}
		playing++
		row := snapshot.Row{Title: snapshot.Text{Name: s.UserName}, Value: &snapshot.Text{Name: s.NowPlayingItem.Name}}
		if s.NowPlayingItem.SeriesName != "" {
			row.Value = snapshot.Msg("jellyfin.episode", map[string]any{"series": s.NowPlayingItem.SeriesName, "name": s.NowPlayingItem.Name})
		}
		how := "jellyfin.direct"
		switch {
		case s.PlayState.IsPaused:
			how, row.Tone = "jellyfin.paused", "gray"
		case s.PlayState.PlayMethod == "Transcode":
			how = "jellyfin.transcoding"
		}
		row.Detail = snapshot.Msg(how, map[string]any{"device": s.DeviceName})
		rows = append(rows, row)
	}
	if playing == 0 {
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "jellyfin.playing"}, Value: snapshot.Msg("jellyfin.nothing", nil)})
	}

	// What it holds, leaving out the kinds it has none of.
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
			rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: k.key}, Value: snapshot.Msg(k.key+"_n", k.args)})
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
			rows = append(rows, snapshot.Row{
				Title: snapshot.Text{Name: f.DeviceID},
				Value: snapshot.Msg("jellyfin.free", map[string]any{"n": f.FreeSpace}),
				Meter: &snapshot.Meter{Used: float64(f.UsedSpace), Total: float64(f.FreeSpace + f.UsedSpace)},
			})
		}
	}

	// The plays of the last day by the hour, and the failed logins, which
	// Jellyfin words itself.
	// By the hours of Overlook's time zone, which a half-hour offset sets
	// apart from UTC's.
	hour := func(t time.Time) time.Time {
		t = t.In(now.Location())
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
	}
	this := hour(now)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	hours, today := make([]float64, 24), 0
	var failed []entry
	for _, e := range log.Items {
		switch e.Type {
		case "VideoPlayback", "AudioPlayback":
			if ago := int(this.Sub(hour(e.Date)) / time.Hour); ago >= 0 && ago < 24 {
				hours[23-ago]++
			}
			if !e.Date.Before(midnight) {
				today++
			}
		case "AuthenticationFailed":
			failed = append(failed, e)
		}
	}
	if len(failed) > 0 {
		newest := slices.MaxFunc(failed, func(a, b entry) int { return a.Date.Compare(b.Date) })
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Key: "jellyfin.failed_logins"}, Tone: "orange",
			Value:  snapshot.Msg("jellyfin.failed_logins_n", map[string]any{"n": len(failed)}),
			Detail: &snapshot.Text{Name: newest.Name},
		})
	}

	// The tasks whose last run failed, such as a scan of a library, as
	// Jellyfin words why.
	broken := false
	for _, t := range tasks {
		r := t.LastExecutionResult
		if r == nil || r.Status != "Failed" {
			continue
		}
		broken = true
		row := snapshot.Row{Title: snapshot.Text{Name: t.Name}, Value: snapshot.Msg("jellyfin.task_failed", nil), Tone: "red"}
		row.Detail = snapshot.Msg("jellyfin.task_failed_at", map[string]any{"at": r.EndTimeUtc})
		if r.ErrorMessage != "" {
			row.Detail = &snapshot.Text{Name: r.ErrorMessage}
		}
		rows = append(rows, row)
	}

	version := snapshot.Row{Title: snapshot.Text{Key: "jellyfin.version"}, Value: &snapshot.Text{Name: sys.Version}}
	if sys.HasPendingRestart {
		version.Value, version.Tone = snapshot.Msg("jellyfin.restart", nil), "orange"
		version.Detail = &snapshot.Text{Name: sys.Version}
	}
	rows = append(rows, version)

	// Today: the plays since midnight, which add up with any other media
	// server's, and what plays now.
	f := snapshot.Figure{
		Area: Area, Icon: "play", Label: snapshot.PlaysFigure, Value: float64(today), Hours: hours, Adds: true,
		Detail: snapshot.Msg(snapshot.PlayingNow, map[string]any{"n": playing}),
	}
	if broken {
		f.Tone = "orange"
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.MediaSection, Rows: rows}},
		Figures:  []snapshot.Figure{f},
	}, nil
}
