// Package prowlarr reads Prowlarr: each indexer, whether it's enabled and
// works, what it was asked and grabbed this week, and what Prowlarr warns
// about.
package prowlarr

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/servarr"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Config is one Prowlarr, under sources.prowlarr in overlook.yaml.
type Config struct {
	servarr.Config `yaml:",inline"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error { return c.Config.Check("Prowlarr") }

// indexer is one of /api/v1/indexer.
type indexer struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
}

// failing is one of /api/v1/indexerstatus, which lists only the indexers
// Prowlarr leaves out for now because they failed.
type failing struct {
	IndexerID      int       `json:"indexerId"`
	InitialFailure time.Time `json:"initialFailure"`
}

// stats are one indexer's of /api/v1/indexerstats, over the week asked for.
type stats struct {
	IndexerID           int     `json:"indexerId"`
	AverageResponseTime float64 `json:"averageResponseTime"`
	NumberOfQueries     int     `json:"numberOfQueries"`
	NumberOfRssQueries  int     `json:"numberOfRssQueries"`
	NumberOfGrabs       int     `json:"numberOfGrabs"`
}

// Read reads the Prowlarr at c, which Check has filled.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var indexers []indexer
	var failed []failing
	var week struct {
		Indexers []stats `json:"indexers"`
	}
	since := url.Values{"startDate": {now.AddDate(0, 0, -7).UTC().Format(time.RFC3339)}}
	for _, r := range []struct {
		path string
		v    any
	}{
		{"/api/v1/indexer", &indexers},
		{"/api/v1/indexerstatus", &failed},
		{"/api/v1/indexerstats?" + since.Encode(), &week},
	} {
		if err := c.Get(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	health, err := servarr.Health(ctx, c.Config, "/api/v1")
	if err != nil {
		return snapshot.Parts{}, err
	}

	// An indexer each, by name: disabled in grey, failing in red since when
	// it started, and otherwise what it was asked and grabbed this week.
	slices.SortFunc(indexers, func(a, b indexer) int { return cmp.Compare(a.Name, b.Name) })
	rows := []snapshot.Row{}
	for _, ix := range indexers {
		row := snapshot.Row{Title: snapshot.Text{Name: ix.Name}, Value: snapshot.Msg("prowlarr.enabled", nil)}
		if i := slices.IndexFunc(week.Indexers, func(s stats) bool { return s.IndexerID == ix.ID }); i >= 0 {
			s := week.Indexers[i]
			row.Detail = snapshot.Msg("prowlarr.week", map[string]any{
				"n": s.NumberOfQueries + s.NumberOfRssQueries, "grabs": s.NumberOfGrabs, "ms": s.AverageResponseTime,
			})
		}
		if i := slices.IndexFunc(failed, func(f failing) bool { return f.IndexerID == ix.ID }); i >= 0 {
			row.Value, row.Tone = snapshot.Msg("prowlarr.failing", nil), "red"
			row.Detail = snapshot.Msg("prowlarr.failing_since", map[string]any{"at": failed[i].InitialFailure})
		}
		if !ix.Enable {
			row.Value, row.Tone, row.Detail = snapshot.Msg("prowlarr.disabled", nil), "gray", nil
		}
		rows = append(rows, row)
	}
	return snapshot.Parts{Sections: []snapshot.Section{
		{Area: Area, Title: snapshot.Text{Key: "prowlarr.indexers"}, Rows: append(rows, health...)},
	}}, nil
}
