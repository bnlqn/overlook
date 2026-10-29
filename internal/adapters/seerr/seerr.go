// Package seerr reads Seerr: the requests for films and shows, and, as
// chores, those waiting for the owner's approval; the problems people
// reported and haven't had fixed; and whether a newer Seerr is out.
package seerr

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// waiting is the most requests waiting for approval the page lists.
const waiting = 20

// Config is one Seerr, under sources.seerr in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Seerr, such as http://seerr:5055.
	URL string `yaml:"url"`
	// Key is Seerr's API key, from Settings, General.
	Key string `yaml:"key"`
	// Link is where the page sends the owner to approve a request, if not
	// URL.
	Link string `yaml:"link"`

	source fetch.Source
	titles *titles
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: http.Header{"X-Api-Key": {c.Key}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Seerr")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	c.titles = &titles{by: map[string]string{}}
	return nil
}

// titles are the films' and shows' titles with their year, by their kind
// and TMDB id, such as movie/653, as Seerr reads them from TMDB. A request
// holds only the id, and a film's own answer is its whole page, cast and
// all, so each title is read once and kept.
//
// ponytail: kept in memory for as long as Overlook runs, one per film or
// show ever waiting, which a title that never changes allows. If a homelab
// ever waits on thousands, keep only those waiting now.
type titles struct {
	mu sync.Mutex
	by map[string]string
}

// of returns the title of the film or show at kind/id, reading it if it
// isn't kept, or the id when it can't be read.
func (t *titles) of(ctx context.Context, c Config, kind string, id int) string {
	key := kind + "/" + strconv.Itoa(id)
	t.mu.Lock()
	title, ok := t.by[key]
	t.mu.Unlock()
	if ok {
		return title
	}
	var m struct {
		Title       string `json:"title"`
		Name        string `json:"name"`
		ReleaseDate string `json:"releaseDate"`
		FirstAir    string `json:"firstAirDate"`
	}
	if err := c.source.JSON(ctx, "/api/v1/"+key, &m); err != nil {
		return key
	}
	title = m.Title + m.Name
	if year, _, _ := strings.Cut(m.ReleaseDate+m.FirstAir, "-"); len(year) == 4 {
		title += " (" + year + ")"
	}
	t.mu.Lock()
	t.by[key] = title
	t.mu.Unlock()
	return title
}

// Read reads the Seerr at c, which Check has filled.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	var (
		status struct {
			Version         string `json:"version"`
			UpdateAvailable bool   `json:"updateAvailable"`
			RestartRequired bool   `json:"restartRequired"`
		}
		counts struct {
			Pending    int `json:"pending"`
			Approved   int `json:"approved"`
			Processing int `json:"processing"`
			Available  int `json:"available"`
		}
		issues struct {
			Open int `json:"open"`
		}
		pending struct {
			Results []struct {
				Created time.Time `json:"createdAt"`
				Is4K    bool      `json:"is4k"`
				Media   struct {
					Kind string `json:"mediaType"`
					TMDB int    `json:"tmdbId"`
				} `json:"media"`
				By struct {
					Name string `json:"displayName"`
				} `json:"requestedBy"`
			} `json:"results"`
		}
	)
	for _, r := range []struct {
		path string
		v    any
	}{
		{"/api/v1/status", &status},
		{"/api/v1/request/count", &counts},
		{"/api/v1/issue/count", &issues},
		{"/api/v1/request?filter=pending&sort=added&take=" + strconv.Itoa(waiting), &pending},
	} {
		if err := c.source.JSON(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	rows := []snapshot.Row{{
		Title:  snapshot.Text{Key: "seerr.requests"},
		Value:  snapshot.Msg("seerr.waiting", map[string]any{"n": counts.Pending}),
		Detail: snapshot.Msg("seerr.requests_detail", map[string]any{"n": counts.Processing, "available": counts.Available}),
	}}
	// A problem someone reported, such as a film that stops, waits on the
	// owner too, but as a fix, not an approval.
	if issues.Open > 0 {
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "seerr.issues"}, Value: snapshot.Msg("seerr.open", map[string]any{"n": issues.Open}), Tone: "orange"})
	}
	version := snapshot.Row{Title: snapshot.Text{Key: "seerr.version"}, Value: &snapshot.Text{Name: status.Version}}
	switch {
	case status.RestartRequired:
		version.Value, version.Tone, version.Detail = snapshot.Msg("seerr.restart", nil), "orange", &snapshot.Text{Name: status.Version}
	case status.UpdateAvailable:
		version.Value, version.Tone, version.Detail = snapshot.Msg("seerr.newer", nil), "orange", &snapshot.Text{Name: status.Version}
	}
	rows = append(rows, version)

	// Each request waiting for approval is a chore, from the day it was
	// made, done on the film's or show's page in Seerr, with who asked.
	var chores []snapshot.Chore
	for _, r := range pending.Results {
		note := r.By.Name
		if r.Is4K {
			note += " · 4K"
		}
		chores = append(chores, snapshot.Chore{
			Name: c.titles.of(ctx, c, r.Media.Kind, r.Media.TMDB),
			From: r.Created.In(time.Local).Format(time.DateOnly),
			Note: note,
			URL:  strings.TrimSuffix(c.Link, "/") + "/" + r.Media.Kind + "/" + strconv.Itoa(r.Media.TMDB),
		})
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "seerr.requests"}, Rows: rows}},
		Chores:   chores,
	}, nil
}
