// Package servarr is what the Radarr, Sonarr and Prowlarr adapters share,
// because the three are built on the same code: their config and key, their
// queue and health, and the cache their libraries are read through.
package servarr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Library is the tile Radarr's films and Sonarr's shows share on the
// overview, split by the room each takes, when both are read.
var Library = snapshot.Text{Key: "servarr.library"}

// Config is one Radarr or Sonarr.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads it, such as http://radarr:7878.
	URL string `yaml:"url"`
	// Key is its API key, from Settings, General. The key can do anything,
	// and no lesser one exists; Overlook's GET-only reads keep it harmless
	// (ADR 0001).
	Key string `yaml:"key"`
}

// Check fills c's defaults, naming it name if the config doesn't, and
// refuses what can't work.
func (c *Config) Check(name string) error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	if c.Name == "" {
		c.Name = name
	}
	return nil
}

// Get reads the answer at path, such as /api/v3/health, into v.
func (c Config) Get(ctx context.Context, path string, v any) error {
	return fetch.JSON(ctx, strings.TrimSuffix(c.URL, "/")+path, http.Header{"X-Api-Key": {c.Key}}, v)
}

// Status is what the source is downloading and what it warns about, as
// rows. words is the adapter's, whose table says what it downloads, as films
// or episodes.
func Status(ctx context.Context, c Config, words string) ([]snapshot.Row, error) {
	var queue struct {
		TotalCount int `json:"totalCount"`
	}
	if err := c.Get(ctx, "/api/v3/queue/status", &queue); err != nil {
		return nil, err
	}
	health, err := Health(ctx, c, "/api/v3")
	if err != nil {
		return nil, err
	}
	value := snapshot.Text{Key: "servarr.nothing"}
	if queue.TotalCount > 0 {
		value = snapshot.Text{Key: words + ".downloading_n", Args: map[string]any{"n": queue.TotalCount}}
	}
	return append([]snapshot.Row{{Title: snapshot.Text{Key: "servarr.downloading"}, Value: &value}}, health...), nil
}

// Health is every warning the source gives at its API api, such as /api/v3,
// as a row each, as the source words it: its health lists only what's
// wrong.
func Health(ctx context.Context, c Config, api string) ([]snapshot.Row, error) {
	var health []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := c.Get(ctx, api+"/health", &health); err != nil {
		return nil, err
	}
	var rows []snapshot.Row
	for _, h := range health {
		level, tone := snapshot.Text{Key: "servarr.warning"}, "orange"
		if h.Type == "error" {
			level, tone = snapshot.Text{Key: "servarr.error"}, "red"
		}
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Name: c.Name}, Value: &level, Tone: tone, Detail: &snapshot.Text{Name: h.Message}})
	}
	return rows, nil
}

// every is how long a library is kept: Radarr's film list is about a
// megabyte, too much to read every time the snapshot is.
const every = 10 * time.Minute

// Cache keeps a library's last answer for ten minutes. A read that fails
// isn't kept, so the next snapshot tries again.
//
// ponytail: an answer kept longer than a snapshot (ADR 0004), in memory and
// one per source, so a film can take ten minutes to count. If Radarr and
// Sonarr ever serve their library's totals on their own, read those with
// every snapshot and drop the cache.
type Cache[T any] struct {
	// Now is the clock, time.Now unless a test sets it.
	Now func() time.Time

	mu   sync.Mutex
	at   time.Time
	last T
}

// Get returns the kept answer, or read's if the kept one is ten minutes old.
func (c *Cache[T]) Get(ctx context.Context, read func(context.Context) (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	if !c.at.IsZero() && now().Sub(c.at) < every {
		return c.last, nil
	}
	v, err := read(ctx)
	if err != nil {
		return v, err
	}
	c.at, c.last = now(), v
	return v, nil
}
