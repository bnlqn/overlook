// Package sonarr reads Sonarr: the shows and episodes it holds and how much
// room they take, the episodes it imported this week, what it's downloading
// and warns about, and the episodes that air in the next seven days.
package sonarr

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/servarr"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is one Sonarr, under sources.sonarr in overlook.yaml.
type Config struct {
	servarr.Config `yaml:",inline"`

	shows *servarr.Cache[[]series]
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.shows = &servarr.Cache[[]series]{}
	return c.Config.Check("Sonarr")
}

// series is a show of Sonarr's, from /api/v3/series.
type series struct {
	Statistics struct {
		EpisodeFileCount int   `json:"episodeFileCount"`
		SizeOnDisk       int64 `json:"sizeOnDisk"`
	} `json:"statistics"`
}

// episode is an episode of Sonarr's calendar, from /api/v3/calendar.
type episode struct {
	Title         string    `json:"title"`
	SeasonNumber  int       `json:"seasonNumber"`
	EpisodeNumber int       `json:"episodeNumber"`
	AirDateUTC    time.Time `json:"airDateUtc"`
	HasFile       bool      `json:"hasFile"`
	Series        struct {
		Title string `json:"title"`
	} `json:"series"`
}

// Read reads the Sonarr at c, which Check has filled.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	shows, err := c.shows.Get(ctx, func(ctx context.Context) ([]series, error) {
		var all []series
		err := c.Get(ctx, "/api/v3/series", &all)
		return all, err
	})
	if err != nil {
		return snapshot.Parts{}, err
	}
	// The episodes imported this week. A better copy of an episode is
	// imported again, so an episode counts once however many records it has,
	// as a film whose file was replaced does.
	var history []struct {
		EpisodeID int `json:"episodeId"`
	}
	since := url.Values{"date": {now.AddDate(0, 0, -7).UTC().Format(time.RFC3339)}, "eventType": {"downloadFolderImported"}}
	if err := c.Get(ctx, "/api/v3/history/since?"+since.Encode(), &history); err != nil {
		return snapshot.Parts{}, err
	}
	imported := map[int]bool{}
	for _, h := range history {
		imported[h.EpisodeID] = true
	}
	status, err := servarr.Status(ctx, c.Config, "sonarr")
	if err != nil {
		return snapshot.Parts{}, err
	}
	// Today and the seven days after it, by the server's clock.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := url.Values{
		"start":         {start.UTC().Format(time.RFC3339)},
		"end":           {start.AddDate(0, 0, 8).UTC().Format(time.RFC3339)},
		"includeSeries": {"true"},
	}
	var calendar []episode
	if err := c.Get(ctx, "/api/v3/calendar?"+days.Encode(), &calendar); err != nil {
		return snapshot.Parts{}, err
	}

	var episodes int
	var size int64
	for _, s := range shows {
		episodes += s.Statistics.EpisodeFileCount
		size += s.Statistics.SizeOnDisk
	}
	library := snapshot.Row{
		Title:  snapshot.Text{Key: "servarr.in_library"},
		Value:  &snapshot.Text{Key: "sonarr.shows_n", Args: map[string]any{"n": len(shows)}},
		Detail: &snapshot.Text{Key: "sonarr.library_detail", Args: map[string]any{"n": episodes, "size": size, "week": len(imported)}},
	}
	// Today's figure: the shows, sharing the library's tile with the films
	// when Radarr is read too, their part of it the room they take.
	join := servarr.Library
	figure := snapshot.Figure{
		Area: "apps", Icon: "film", Label: snapshot.Text{Key: "sonarr.shows"}, Value: float64(len(shows)),
		Unit:   &snapshot.Text{Key: "sonarr.unit", Args: map[string]any{"n": len(shows)}},
		Detail: &snapshot.Text{Key: "sonarr.this_week", Args: map[string]any{"n": len(imported)}},
		Join:   &join,
		Part: &snapshot.Share{Label: snapshot.Text{Key: "sonarr.shows"}, Value: float64(size), Tone: "blue", Rows: []snapshot.Row{
			{Title: snapshot.Text{Key: "servarr.on_disk"}, Value: &snapshot.Text{Key: "servarr.bytes", Args: map[string]any{"n": size}}},
			{Title: snapshot.Text{Key: "servarr.in_library"}, Value: &snapshot.Text{Key: "sonarr.shows_n", Args: map[string]any{"n": len(shows)}}},
			{Title: snapshot.Text{Key: "sonarr.episodes"}, Value: &snapshot.Text{Key: "sonarr.n", Args: map[string]any{"n": episodes}}},
			{Title: snapshot.Text{Key: "servarr.week"}, Value: &snapshot.Text{Key: "servarr.plus", Args: map[string]any{"n": len(imported)}}},
		}},
	}
	return snapshot.Parts{
		Figures: []snapshot.Figure{figure},
		Sections: []snapshot.Section{
			{Area: "apps", Title: snapshot.Text{Key: "sonarr.shows"}, Rows: append([]snapshot.Row{library}, status...)},
			{Area: "apps", Title: snapshot.Text{Key: "sonarr.coming"}, Rows: coming(calendar)},
		},
	}, nil
}

// coming is a row for each episode on the calendar, soonest first.
func coming(calendar []episode) []snapshot.Row {
	slices.SortStableFunc(calendar, func(a, b episode) int { return a.AirDateUTC.Compare(b.AirDateUTC) })
	rows := make([]snapshot.Row, 0, len(calendar))
	for _, e := range calendar {
		value := snapshot.Text{Key: "servarr.on", Args: map[string]any{"at": e.AirDateUTC}}
		if e.HasFile {
			value = snapshot.Text{Key: "servarr.have"}
		}
		detail := snapshot.Text{Name: e.Title}
		if e.Title == "" {
			detail = snapshot.Text{Key: "sonarr.untitled"}
		}
		rows = append(rows, snapshot.Row{
			// As Sonarr itself writes an episode: Show 1×02.
			Title:  snapshot.Text{Name: fmt.Sprintf("%s %d×%02d", e.Series.Title, e.SeasonNumber, e.EpisodeNumber)},
			Value:  &value,
			Detail: &detail,
		})
	}
	if len(rows) == 0 {
		rows = []snapshot.Row{{Title: snapshot.Text{Key: "servarr.coming_none"}}}
	}
	return rows
}
