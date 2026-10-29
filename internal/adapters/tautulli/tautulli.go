// Package tautulli reads Plex through Tautulli, a media server: what plays
// now and how, and the plays of the last day, as any media server's
// (snapshot.MediaServer); and the libraries, whether Tautulli reaches the
// Plex server, and whether a newer Plex is out.
package tautulli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// reads are the only commands the adapter sends Tautulli, and each of them
// reads. Tautulli takes a command that changes something as a GET too, such
// as delete_all_history, so this list, not the GET-only client, is what
// keeps its key harmless (ADR 0011).
var reads = []string{"get_activity", "server_status", "get_libraries", "get_history", "get_pms_update"}

// Config is one Tautulli, under sources.tautulli in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Tautulli, such as http://tautulli:8181.
	URL string `yaml:"url"`
	// Key is Tautulli's API key, from Settings, Web Interface.
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
	c.Name = cmp.Or(c.Name, "Plex")
	return nil
}

// get sends Tautulli the command cmd, one of reads, with its values, and
// reads its answer's data into v. Tautulli takes its key only in the
// address.
func (c Config) get(ctx context.Context, cmd string, values url.Values, v any) error {
	if !slices.Contains(reads, cmd) {
		return fmt.Errorf("%s isn't a command that reads", cmd)
	}
	q := url.Values{"apikey": {c.Key}, "cmd": {cmd}}
	for k, vs := range values {
		q[k] = vs
	}
	var answer struct {
		Response struct {
			Result  string          `json:"result"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		} `json:"response"`
	}
	if err := fetch.JSON(ctx, strings.TrimSuffix(c.URL, "/")+"/api/v2?"+q.Encode(), nil, &answer); err != nil {
		return err
	}
	if answer.Response.Result != "success" {
		return fmt.Errorf("%s: %s", cmd, cmp.Or(answer.Response.Message, answer.Response.Result))
	}
	if err := json.Unmarshal(answer.Response.Data, v); err != nil {
		return fmt.Errorf("an answer that isn't the JSON expected: %w", err)
	}
	return nil
}

// count is a number as Tautulli writes it, often as a string, or null.
type count int

func (n *count) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.Atoi(s)
	*n = count(v)
	return err
}

type activity struct {
	Bandwidth int       `json:"total_bandwidth"` // kilobits a second
	Remote    int       `json:"wan_bandwidth"`
	Sessions  []session `json:"sessions"`
}

// session is one person playing something, on a player of theirs.
type session struct {
	User      string `json:"friendly_name"`
	Title     string `json:"full_title"`
	Player    string `json:"player"`
	State     string `json:"state"`
	Transcode string `json:"transcode_decision"`
}

type library struct {
	Name     string `json:"section_name"`
	Type     string `json:"section_type"`
	Count    count  `json:"count"`
	Parents  count  `json:"parent_count"`
	Children count  `json:"child_count"`
}

// Read reads Plex through the Tautulli at c, which Check has filled.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var (
		act     activity
		status  struct{ Connected bool }
		libs    []library
		history struct {
			Data []struct {
				Started int64 `json:"started"`
			} `json:"data"`
		}
		// update is {} when Tautulli has no news of Plex's releases, as
		// for a server not signed in to Plex.
		update struct {
			Available *bool  `json:"update_available"`
			Version   string `json:"version"`
		}
	)
	// Two days back, as Tautulli counts its days, which holds the last 24
	// hours wherever Overlook is.
	day := url.Values{"after": {now.AddDate(0, 0, -2).Format(time.DateOnly)}, "length": {"1000"}}
	for _, r := range []struct {
		cmd    string
		values url.Values
		v      any
	}{
		{"get_activity", nil, &act},
		{"server_status", nil, &status},
		{"get_libraries", nil, &libs},
		{"get_history", day, &history},
		{"get_pms_update", nil, &update},
	} {
		if err := c.get(ctx, r.cmd, r.values, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	var m snapshot.MediaServer
	for _, s := range act.Sessions {
		m.Playing = append(m.Playing, snapshot.Play{
			Who: s.User, Device: s.Player, What: snapshot.Text{Name: s.Title},
			Paused: s.State == "paused", Transcoding: s.Transcode == "transcode",
		})
	}
	// Without the Plex server, Tautulli knows only what it kept.
	if !status.Connected {
		m.Rows = append(m.Rows, snapshot.Row{Title: snapshot.Text{Key: "tautulli.plex"}, Value: snapshot.Msg("tautulli.unreachable", nil), Tone: "red"})
	}
	if len(act.Sessions) > 0 {
		m.Rows = append(m.Rows, snapshot.Row{
			Title:  snapshot.Text{Key: "tautulli.bandwidth"},
			Value:  snapshot.Msg("tautulli.speed", map[string]any{"n": act.Bandwidth * 1000 / 8}),
			Detail: snapshot.Msg("tautulli.remote", map[string]any{"n": act.Remote * 1000 / 8}),
		})
	}

	// Each library, by what it holds.
	slices.SortFunc(libs, func(a, b library) int { return cmp.Compare(a.Name, b.Name) })
	for _, l := range libs {
		args := map[string]any{"n": int(l.Count), "parents": int(l.Parents), "children": int(l.Children)}
		key := map[string]string{"movie": "tautulli.films_n", "show": "tautulli.shows_n", "artist": "tautulli.music_n", "photo": "tautulli.photos_n"}[l.Type]
		if key == "" {
			key = "tautulli.items_n"
		}
		m.Rows = append(m.Rows, snapshot.Row{Title: snapshot.Text{Name: l.Name}, Value: snapshot.Msg(key, args)})
	}

	if update.Available != nil && status.Connected {
		plex := snapshot.Row{Title: snapshot.Text{Key: "tautulli.plex"}, Value: snapshot.Msg("tautulli.current", nil)}
		if *update.Available && update.Version != "" {
			plex.Value, plex.Tone = snapshot.Msg("tautulli.newer", map[string]any{"version": update.Version}), "orange"
		}
		m.Rows = append(m.Rows, plex)
	}

	// The plays of the last day, those still going too.
	for _, h := range history.Data {
		m.Plays = append(m.Plays, time.Unix(h.Started, 0))
	}
	return m.Parts(Area, now), nil
}
