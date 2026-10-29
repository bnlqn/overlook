// Package qbittorrent reads qBittorrent: what it downloads and seeds and how
// fast, the torrents in error, the room left on its disk, and whether peers
// can reach it.
package qbittorrent

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Config is one qBittorrent, under sources.qbittorrent in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads qBittorrent's web UI, such as
	// http://qbittorrent:8080.
	URL string `yaml:"url"`
	// Key is an API key, which qBittorrent makes from 5.2 on.
	Key string `yaml:"key"`
	// User and Password log in to a qBittorrent before 5.2, which has no
	// keys, with one POST (ADR 0009). With neither a key nor a user,
	// qBittorrent must let Overlook's address in without a login.
	User     string `yaml:"user"`
	Password string `yaml:"password"`

	source fetch.Source
	totals *fetch.Cache[totals]
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL}
	switch {
	case c.Key != "":
		c.source.Header = http.Header{"Authorization": {"Bearer " + c.Key}}
	case c.User != "":
		c.source.Login = &fetch.Login{
			Path: "/api/v2/auth/login", Credentials: map[string]string{"username": c.User, "password": c.Password}, Form: true, Take: cookie,
		}
	}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key != "" && c.User != "" {
		return errors.New("key or user, not both: a key from qBittorrent 5.2 on, a login before")
	}
	if c.Password != "" && c.User == "" {
		return errors.New("password without a user")
	}
	c.Name = cmp.Or(c.Name, "qBittorrent")
	c.totals = &fetch.Cache[totals]{}
	return nil
}

// cookie takes the session from a login's answer, which qBittorrent gives
// as a cookie, SID unless its config names another. A login it refuses
// answers "Fails." without one.
func cookie(h http.Header, _ []byte) (http.Header, error) {
	cookies := (&http.Response{Header: h}).Cookies()
	if len(cookies) == 0 {
		return nil, errors.New("refused")
	}
	return http.Header{"Cookie": {cookies[0].Name + "=" + cookies[0].Value}}, nil
}

// torrent is one of qBittorrent's torrents.
type torrent struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	SavePath string `json:"save_path"`
}

// totals are how many torrents download and seed, and the room left on the
// disk they're saved to.
type totals struct {
	downloading, seeding int
	free                 int64
}

// The states of a torrent that downloads, waiting or not, and of one that
// seeds; a stopped one, or one in error, does neither.
var (
	downloading = []string{"downloading", "metaDL", "forcedMetaDL", "forcedDL", "stalledDL", "queuedDL", "checkingDL", "allocating"}
	seeding     = []string{"uploading", "forcedUP", "stalledUP", "queuedUP", "checkingUP"}
)

// Read reads the qBittorrent at c, which Check has filled: how fast it moves
// and whether peers reach it, and the torrents in error, with every
// snapshot, and the totals at most every ten minutes, from the list of every
// torrent, which is megabytes for a thousand.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var transfer struct {
		Status string `json:"connection_status"`
		Down   int64  `json:"dl_info_speed"`
		Up     int64  `json:"up_info_speed"`
	}
	var errored []torrent
	if err := c.source.JSON(ctx, "/api/v2/transfer/info", &transfer); err != nil {
		return snapshot.Parts{}, err
	}
	if err := c.source.JSON(ctx, "/api/v2/torrents/info?filter=errored", &errored); err != nil {
		return snapshot.Parts{}, err
	}
	t, err := c.totals.Get(ctx, now, func(ctx context.Context) (totals, error) {
		var all struct {
			Server struct {
				Free int64 `json:"free_space_on_disk"`
			} `json:"server_state"`
			Torrents map[string]torrent `json:"torrents"`
		}
		if err := c.source.JSON(ctx, "/api/v2/sync/maindata", &all); err != nil {
			return totals{}, err
		}
		t := totals{free: all.Server.Free}
		for _, x := range all.Torrents {
			switch {
			case slices.Contains(downloading, x.State):
				t.downloading++
			case slices.Contains(seeding, x.State):
				t.seeding++
			}
		}
		return t, nil
	})
	if err != nil {
		return snapshot.Parts{}, err
	}

	rows := []snapshot.Row{
		{
			Title:  snapshot.Text{Key: "qbittorrent.downloading"},
			Value:  snapshot.Msg("qbittorrent.torrents_n", map[string]any{"n": t.downloading}),
			Detail: snapshot.Msg("qbittorrent.speed", map[string]any{"n": transfer.Down}),
		},
		{
			Title:  snapshot.Text{Key: "qbittorrent.seeding"},
			Value:  snapshot.Msg("qbittorrent.torrents_n", map[string]any{"n": t.seeding}),
			Detail: snapshot.Msg("qbittorrent.speed", map[string]any{"n": transfer.Up}),
		},
		{Title: snapshot.Text{Key: "qbittorrent.free"}, Value: snapshot.Msg("qbittorrent.bytes", map[string]any{"n": t.free})},
	}
	// Whether peers reach it: firewalled, it can still download, from peers
	// it reaches itself, but slowly.
	connection := snapshot.Row{Title: snapshot.Text{Key: "qbittorrent.connection"}}
	switch transfer.Status {
	case "connected":
		connection.Value = snapshot.Msg("qbittorrent.connected", nil)
	case "firewalled":
		connection.Value, connection.Tone = snapshot.Msg("qbittorrent.firewalled", nil), "orange"
		connection.Detail = snapshot.Msg("qbittorrent.firewalled_detail", nil)
	case "disconnected":
		connection.Value, connection.Tone = snapshot.Msg("qbittorrent.disconnected", nil), "red"
	default:
		connection.Value, connection.Tone = &snapshot.Text{Name: transfer.Status}, "gray"
	}
	rows = append(rows, connection)
	// The torrents in error, by name, each with where it's saved: a torrent
	// whose files are missing is often one whose disk isn't mounted.
	slices.SortFunc(errored, func(a, b torrent) int { return cmp.Compare(a.Name, b.Name) })
	for _, x := range errored {
		key := "qbittorrent.error"
		if x.State == "missingFiles" {
			key = "qbittorrent.missing_files"
		}
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Name: x.Name}, Value: snapshot.Msg(key, nil), Tone: "red", Detail: &snapshot.Text{Name: x.SavePath}})
	}

	// Today: the torrents downloading, which adds up with another
	// qBittorrent's.
	f := snapshot.Figure{
		Area: Area, Icon: "download", Label: snapshot.Text{Key: "qbittorrent.torrents"}, Value: float64(t.downloading),
		Unit:   snapshot.Msg("qbittorrent.downloading_unit", nil),
		Detail: snapshot.Msg("qbittorrent.figure_detail", map[string]any{"n": t.seeding, "down": transfer.Down}),
		Adds:   true,
	}
	if len(errored) > 0 || transfer.Status != "connected" {
		f.Tone = "orange"
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "qbittorrent.torrents"}, Rows: rows}},
		Figures:  []snapshot.Figure{f},
	}, nil
}
