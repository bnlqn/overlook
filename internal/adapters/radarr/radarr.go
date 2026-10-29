// Package radarr reads Radarr: the films it holds and how much room they
// take, the films it added this week, what it's downloading and warns about,
// and the films out to stream or on disc in the next seven days.
package radarr

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/servarr"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is one Radarr, under sources.radarr in overlook.yaml.
type Config struct {
	servarr.Config `yaml:",inline"`

	films *servarr.Cache[[]movie]
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.films = &servarr.Cache[[]movie]{}
	return c.Config.Check("Radarr")
}

// movie is a film of Radarr's, from /api/v3/movie and /api/v3/calendar.
type movie struct {
	Title      string `json:"title"`
	Year       int    `json:"year"`
	HasFile    bool   `json:"hasFile"`
	SizeOnDisk int64  `json:"sizeOnDisk"`
	MovieFile  *struct {
		DateAdded time.Time `json:"dateAdded"`
	} `json:"movieFile"`
	DigitalRelease  *time.Time `json:"digitalRelease"`
	PhysicalRelease *time.Time `json:"physicalRelease"`
}

// Read reads the Radarr at c, which Check has filled.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	films, err := c.films.Get(ctx, func(ctx context.Context) ([]movie, error) {
		var all []movie
		err := c.Get(ctx, "/api/v3/movie", &all)
		return all, err
	})
	if err != nil {
		return snapshot.Parts{}, err
	}
	status, err := servarr.Status(ctx, c.Config, "radarr")
	if err != nil {
		return snapshot.Parts{}, err
	}
	// Today and the seven days after it, by the server's clock.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 8)
	days := url.Values{"start": {start.UTC().Format(time.RFC3339)}, "end": {end.UTC().Format(time.RFC3339)}}
	var calendar []movie
	if err := c.Get(ctx, "/api/v3/calendar?"+days.Encode(), &calendar); err != nil {
		return snapshot.Parts{}, err
	}

	weekAgo := now.AddDate(0, 0, -7)
	var owned, week int
	var size int64
	for _, m := range films {
		if !m.HasFile {
			continue
		}
		owned++
		size += m.SizeOnDisk
		if m.MovieFile != nil && !m.MovieFile.DateAdded.Before(weekAgo) {
			week++
		}
	}
	library := snapshot.Row{
		Title:  snapshot.Text{Key: "servarr.in_library"},
		Value:  &snapshot.Text{Key: "radarr.films_n", Args: map[string]any{"n": owned}},
		Detail: &snapshot.Text{Key: "radarr.library_detail", Args: map[string]any{"size": size, "n": week}},
	}
	// Today's figure: the films, sharing the library's tile with the shows
	// when Sonarr is read too, their part of it the room they take.
	join := servarr.Library
	figure := snapshot.Figure{
		Area: "apps", Icon: "film", Label: snapshot.Text{Key: "radarr.films"}, Value: float64(owned),
		Unit:   &snapshot.Text{Key: "radarr.unit", Args: map[string]any{"n": owned}},
		Detail: &snapshot.Text{Key: "radarr.this_week", Args: map[string]any{"n": week}},
		Join:   &join,
		Part: &snapshot.Share{Label: snapshot.Text{Key: "radarr.films"}, Value: float64(size), Rows: []snapshot.Row{
			{Title: snapshot.Text{Key: "servarr.on_disk"}, Value: &snapshot.Text{Key: "servarr.bytes", Args: map[string]any{"n": size}}},
			{Title: snapshot.Text{Key: "servarr.in_library"}, Value: &snapshot.Text{Key: "radarr.films_n", Args: map[string]any{"n": owned}}},
			{Title: snapshot.Text{Key: "servarr.week"}, Value: &snapshot.Text{Key: "servarr.plus", Args: map[string]any{"n": week}}},
		}},
	}
	return snapshot.Parts{
		Figures: []snapshot.Figure{figure},
		Sections: []snapshot.Section{
			{Area: "apps", Title: snapshot.Text{Key: "radarr.films"}, Rows: append([]snapshot.Row{library}, status...)},
			{Area: "apps", Title: snapshot.Text{Key: "radarr.coming"}, Rows: coming(calendar, start, end)},
		},
	}, nil
}

// coming is a row for each film out to stream or on disc between start and
// end, soonest first: its first such release, as the calendar has every film
// with any release then, in cinemas too.
func coming(calendar []movie, start, end time.Time) []snapshot.Row {
	type release struct {
		at  time.Time
		row snapshot.Row
	}
	var all []release
	for _, m := range calendar {
		for _, r := range []struct {
			at   *time.Time
			what string
		}{{m.DigitalRelease, "radarr.to_stream"}, {m.PhysicalRelease, "radarr.on_disc"}} {
			if r.at == nil || r.at.Before(start) || !r.at.Before(end) {
				continue
			}
			value := snapshot.Text{Key: "servarr.on", Args: map[string]any{"at": *r.at}}
			if m.HasFile {
				value = snapshot.Text{Key: "servarr.have"}
			}
			all = append(all, release{*r.at, snapshot.Row{
				Title:  snapshot.Text{Name: fmt.Sprintf("%s (%d)", m.Title, m.Year)},
				Value:  &value,
				Detail: &snapshot.Text{Key: r.what},
			}})
			break
		}
	}
	slices.SortStableFunc(all, func(a, b release) int { return a.at.Compare(b.at) })
	rows := make([]snapshot.Row, len(all))
	for i, r := range all {
		rows[i] = r.row
	}
	if len(rows) == 0 {
		rows = []snapshot.Row{{Title: snapshot.Text{Key: "servarr.coming_none"}}}
	}
	return rows
}
