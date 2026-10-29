// Package immich reads Immich, the photos: how many each person keeps, what
// each uploaded this week and the space they take, then Immich's version and
// its jobs.
package immich

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Config is one Immich, under sources.immich in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Immich, such as http://immich:2283.
	URL string `yaml:"url"`
	// Key is an API key with the read permissions the README names.
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
	c.Name = cmp.Or(c.Name, "Immich")
	return nil
}

// statistics is Immich's answer at /api/server/statistics: everyone's photos
// and videos, then each person's.
type statistics struct {
	Photos int64 `json:"photos"`
	Videos int64 `json:"videos"`
	Usage  int64 `json:"usage"` // bytes
	People []struct {
		ID     string `json:"userId"`
		Name   string `json:"userName"`
		Photos int64  `json:"photos"`
		Videos int64  `json:"videos"`
		Usage  int64  `json:"usage"`
	} `json:"usageByUser"`
}

// storage is Immich's answer at /api/server/storage: the disk its library is
// on.
type storage struct {
	Size int64 `json:"diskSizeRaw"`
	Used int64 `json:"diskUseRaw"`
}

// version is Immich's answer at /api/server/about, its own version, and at
// /api/server/version-check, the newest released, empty while it checks for
// none.
type version struct {
	Version        string `json:"version"`
	ReleaseVersion string `json:"releaseVersion"`
}

// queue is one of Immich's answers at /api/queues.
type queue struct {
	Name       string `json:"name"`
	Statistics struct {
		Active  int64 `json:"active"`
		Waiting int64 `json:"waiting"`
	} `json:"statistics"`
}

// Read reads the Immich at c. Its statistics are all it needs: any other read
// that fails leaves out what it would have shown.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	api := strings.TrimSuffix(c.URL, "/") + "/api"
	key := http.Header{"X-Api-Key": {c.Key}}
	get := func(path string, v any) bool { return fetch.JSON(ctx, api+path, key, v) == nil }

	var (
		stats    statistics
		err      error
		disk     storage
		hasDisk  bool
		about    version
		hasAbout bool
		release  version
		queues   []queue
		hasQueue bool
		wg       sync.WaitGroup
	)
	wg.Go(func() { err = fetch.JSON(ctx, api+"/server/statistics", key, &stats) })
	wg.Go(func() { hasDisk = get("/server/storage", &disk) })
	wg.Go(func() { hasAbout = get("/server/about", &about) })
	wg.Go(func() { get("/server/version-check", &release) })
	wg.Go(func() { hasQueue = get("/queues", &queues) })
	wg.Wait()
	if err != nil {
		return snapshot.Parts{}, err
	}

	// What each person uploaded in the 7 days to today, by Immich's days,
	// which are UTC's. A person whose read fails goes without it.
	today := time.Now().UTC()
	days := url.Values{"from": {today.AddDate(0, 0, -6).Format(time.DateOnly)}, "to": {today.Format(time.DateOnly)}, "type": {"Upload"}}.Encode()
	week := make([]*int64, len(stats.People))
	for i, p := range stats.People {
		wg.Go(func() {
			var heatmap struct {
				TotalCount int64 `json:"totalCount"`
			}
			if get("/admin/users/"+url.PathEscape(p.ID)+"/calendar-heatmap?"+days, &heatmap) {
				week[i] = &heatmap.TotalCount
			}
		})
	}
	wg.Wait()

	rows := []snapshot.Row{}
	var everyone int64
	for i, p := range stats.People {
		r := snapshot.Row{Title: snapshot.Text{Name: p.Name}, Detail: snapshot.Msg("immich.person", map[string]any{"photos": p.Photos, "videos": p.Videos, "usage": p.Usage})}
		if week[i] != nil {
			r.Value = snapshot.Msg("immich.this_week", map[string]any{"n": *week[i]})
			everyone += *week[i]
		}
		rows = append(rows, r)
	}
	if hasDisk && disk.Size > 0 {
		rows = append(rows, snapshot.Row{
			Title:  snapshot.Text{Key: "immich.space"},
			Value:  snapshot.Msg("immich.space_used", map[string]any{"used": disk.Used, "size": disk.Size}),
			Detail: snapshot.Msg("immich.space_photos", map[string]any{"usage": stats.Usage}),
			Meter:  &snapshot.Meter{Used: float64(disk.Used), Total: float64(disk.Size)},
		})
	}
	if hasAbout {
		r := snapshot.Row{Title: snapshot.Text{Name: c.Name}, Value: &snapshot.Text{Name: about.Version}}
		switch release.ReleaseVersion {
		case "":
		case about.Version:
			r.Detail = snapshot.Msg("immich.current", nil)
		default:
			r.Detail, r.Tone = snapshot.Msg("immich.newer", map[string]any{"version": release.ReleaseVersion}), "orange"
		}
		rows = append(rows, r)
	}
	if hasQueue {
		idle := true
		for _, q := range queues {
			if n := q.Statistics.Active + q.Statistics.Waiting; n > 0 {
				rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "immich.jobs_of", Args: map[string]any{"name": q.Name}}, Value: snapshot.Msg("immich.queued", map[string]any{"n": n})})
				idle = false
			}
		}
		if idle {
			rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "immich.jobs"}, Value: snapshot.Msg("immich.idle", nil), Detail: snapshot.Msg("immich.nothing_waiting", nil)})
		}
	}

	// Today's figure is everyone's photos and videos, with what they
	// uploaded this week if every person's read answered, and, when there's
	// more than one person, each one's share of the room they take.
	figure := snapshot.Figure{Area: Area, Icon: "photos", Label: snapshot.Text{Key: "immich.photos"}, Value: float64(stats.Photos + stats.Videos)}
	figure.Detail = snapshot.Msg("immich.this_week", map[string]any{"n": everyone})
	if slices.Contains(week, nil) {
		figure.Detail = snapshot.Msg("immich.videos", map[string]any{"n": stats.Videos})
	}
	if len(stats.People) > 1 && stats.Usage > 0 {
		for i, p := range stats.People {
			rows := []snapshot.Row{
				{Title: snapshot.Text{Key: "immich.photos"}, Value: snapshot.Number(p.Photos)},
				{Title: snapshot.Text{Key: "immich.videos_title"}, Value: snapshot.Number(p.Videos)},
				{Title: snapshot.Text{Key: "immich.space"}, Value: snapshot.Msg("immich.bytes", map[string]any{"n": p.Usage})},
			}
			if week[i] != nil {
				rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "immich.week"}, Value: snapshot.Msg("immich.plus", map[string]any{"n": *week[i]})})
			}
			figure.Shares = append(figure.Shares, snapshot.Share{Label: snapshot.Text{Name: p.Name}, Value: float64(p.Usage), Rows: rows})
		}
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "immich.photos"}, Rows: rows}},
		Figures:  []snapshot.Figure{figure},
	}, nil
}
