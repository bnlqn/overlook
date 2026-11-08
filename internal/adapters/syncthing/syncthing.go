// Package syncthing reads Syncthing: each folder, up to date or how far
// behind, in red with its errors; each other device, connected or since when
// it isn't, and how far behind it is; Syncthing's version and whether a
// newer one is out; and, as chores, the devices and folders waiting to be
// accepted. Everything it gives goes in Storage.
package syncthing

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "storage"

// Config is one Syncthing device, under sources.syncthing in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Syncthing's GUI and API, such as
	// http://syncthing:8384.
	URL string `yaml:"url" setup:"text" example:"http://syncthing.example.com:8384"`
	// Key is the GUI's API key, from its settings.
	Key string `yaml:"key" setup:"key"`
	// CA is a PEM file of the GUI's certificate, when it serves HTTPS with
	// its own, and ServerName the name it is for, the device's host name,
	// when URL reaches it by another.
	CA         string `yaml:"ca" setup:"text,optional" example:"/etc/overlook/syncthing.pem"`
	ServerName string `yaml:"server_name" setup:"text,optional" example:"nas"`
	// Link is where the page sends the owner to accept a device or a
	// folder, if not URL.
	Link string `yaml:"link"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, CA: c.CA, ServerName: c.ServerName, Header: http.Header{"X-Api-Key": {c.Key}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Syncthing")
	return fetch.CheckLink(&c.Link, c.URL)
}

// folder is one of /rest/config/folders.
type folder struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Paused bool   `json:"paused"`
}

// device is one of /rest/config/devices, this one among them.
type device struct {
	ID     string `json:"deviceID"`
	Name   string `json:"name"`
	Paused bool   `json:"paused"`
}

// state is a folder's /rest/db/status: what it holds, what it still needs
// from the other devices, and how it is, idle, syncing or stopped by an
// error, such as its path missing, which only this answer says.
type state struct {
	State      string `json:"state"`
	Error      string `json:"error"`
	PullErrors int    `json:"pullErrors"`
	Files      int    `json:"globalFiles"`
	Bytes      int64  `json:"globalBytes"`
	NeedItems  int    `json:"needTotalItems"`
	NeedBytes  int64  `json:"needBytes"`
}

// failure is one of a folder's /rest/folder/errors: an item it couldn't
// sync, and why.
type failure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// notice is one of /rest/system/error, which Syncthing keeps until the
// owner clears them in its GUI.
type notice struct {
	When    time.Time `json:"when"`
	Message string    `json:"message"`
}

// read is everything Read reads, by folder and by device.
type read struct {
	folders []folder
	// devices are the config's, by id, for their names.
	devices map[string]device
	// connections are the other devices', by id: every one the config
	// names, connected or not.
	connections map[string]struct {
		Connected bool `json:"connected"`
		Paused    bool `json:"paused"`
	}
	seen map[string]struct {
		LastSeen time.Time `json:"lastSeen"`
	}
	states   map[string]state
	failures map[string][]failure
	// completions are each other device's, in percent, of every folder it
	// shares.
	completions map[string]float64
	notices     []notice
	version     string
	newer       string
	// pendingDevices are the devices that asked to connect, by id, and
	// pendingFolders the folders offered, by id, each by the devices
	// offering it.
	pendingDevices map[string]struct {
		Time    time.Time `json:"time"`
		Name    string    `json:"name"`
		Address string    `json:"address"`
	}
	pendingFolders map[string]struct {
		OfferedBy map[string]struct {
			Time  time.Time `json:"time"`
			Label string    `json:"label"`
		} `json:"offeredBy"`
	}
}

// Read reads the Syncthing at c.
//
// ponytail: each folder's state and each device's completion are read one
// after another, which Syncthing on the same network answers in
// milliseconds; read them side by side if a homelab of many folders ever
// waits on it.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var r read
	var devices []device
	var conns struct {
		Connections map[string]struct {
			Connected bool `json:"connected"`
			Paused    bool `json:"paused"`
		} `json:"connections"`
	}
	var notices struct {
		Errors []notice `json:"errors"`
	}
	var version struct {
		Version string `json:"version"`
	}
	for _, at := range []struct {
		path string
		v    any
	}{
		{"/rest/config/folders", &r.folders},
		{"/rest/config/devices", &devices},
		{"/rest/system/connections", &conns},
		{"/rest/stats/device", &r.seen},
		{"/rest/system/error", &notices},
		{"/rest/system/version", &version},
		{"/rest/cluster/pending/devices", &r.pendingDevices},
		{"/rest/cluster/pending/folders", &r.pendingFolders},
	} {
		if err := c.source.JSON(ctx, at.path, at.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	r.connections, r.notices, r.version = conns.Connections, notices.Errors, version.Version
	r.devices = map[string]device{}
	for _, d := range devices {
		r.devices[d.ID] = d
	}

	// Syncthing built without upgrades, as its Docker image and most
	// distributions' packages are, refuses to say whether a newer one is
	// out, which leaves its version alone on the page.
	var up struct {
		Latest string `json:"latest"`
		Newer  bool   `json:"newer"`
	}
	if c.source.JSON(ctx, "/rest/system/upgrade", &up) == nil && up.Newer {
		r.newer = up.Latest
	}

	r.states, r.failures = map[string]state{}, map[string][]failure{}
	for _, f := range r.folders {
		if f.ID == "" || f.Paused {
			continue
		}
		var s state
		if err := c.source.JSON(ctx, "/rest/db/status?folder="+url.QueryEscape(f.ID), &s); err != nil {
			return snapshot.Parts{}, fmt.Errorf("%s: %w", cmp.Or(f.Label, f.ID), err)
		}
		r.states[f.ID] = s
		if s.PullErrors > 0 {
			var errs struct {
				Errors []failure `json:"errors"`
			}
			if err := c.source.JSON(ctx, "/rest/folder/errors?folder="+url.QueryEscape(f.ID), &errs); err != nil {
				return snapshot.Parts{}, fmt.Errorf("%s: %w", cmp.Or(f.Label, f.ID), err)
			}
			r.failures[f.ID] = errs.Errors
		}
	}
	r.completions = map[string]float64{}
	for id := range r.connections {
		var done struct {
			Completion float64 `json:"completion"`
		}
		if err := c.source.JSON(ctx, "/rest/db/completion?device="+url.QueryEscape(id), &done); err != nil {
			return snapshot.Parts{}, err
		}
		r.completions[id] = done.Completion
	}
	return parts(r, c.Link, now), nil
}

// percent is how much of something is in sync, rounded down, so that a
// folder or a device a byte behind never reads 100%.
func percent(p float64) float64 { return max(0, math.Floor(p)) }

func parts(r read, link string, now time.Time) snapshot.Parts {
	// A folder each, by label: stopped by an error, in red with it; paused;
	// with items that failed to sync, in red with the first; syncing, or
	// out of sync with nothing to sync from, with what it needs; or up to
	// date, with what it holds.
	folders := slices.Clone(r.folders)
	label := func(f folder) string { return cmp.Or(f.Label, f.ID) }
	slices.SortFunc(folders, func(a, b folder) int { return cmp.Compare(label(a), label(b)) })
	frows := []snapshot.Row{}
	for _, f := range folders {
		if f.ID == "" {
			continue
		}
		row := snapshot.Row{Title: snapshot.Text{Name: label(f)}}
		s, read := r.states[f.ID]
		switch {
		case f.Paused:
			row.Value, row.Tone = snapshot.Msg("syncthing.paused", nil), "gray"
		case !read:
			continue
		case s.State == "error":
			row.Value, row.Tone = snapshot.Msg("syncthing.stopped", nil), "red"
			if s.Error != "" {
				row.Detail = &snapshot.Text{Name: s.Error}
			}
		case s.PullErrors > 0:
			row.Value, row.Tone = snapshot.Msg("syncthing.failed", map[string]any{"n": s.PullErrors}), "red"
			if fs := r.failures[f.ID]; len(fs) > 0 {
				row.Detail = snapshot.Msg("syncthing.failed_at", map[string]any{"path": fs[0].Path, "error": fs[0].Error})
			}
		case s.NeedItems > 0:
			p := 0.0
			if s.Bytes > 0 {
				p = percent(100 * float64(s.Bytes-s.NeedBytes) / float64(s.Bytes))
			}
			row.Value, row.Tone = snapshot.Msg("syncthing.out_of_sync", map[string]any{"percent": p}), "orange"
			if s.State == "syncing" || s.State == "sync-preparing" {
				row.Value, row.Tone = snapshot.Msg("syncthing.syncing", map[string]any{"percent": p}), "blue"
			}
			row.Detail = snapshot.Msg("syncthing.to_sync", map[string]any{"n": s.NeedItems, "bytes": s.NeedBytes})
		default:
			row.Value = snapshot.Msg("syncthing.up_to_date", nil)
			row.Detail = snapshot.Msg("syncthing.holds", map[string]any{"n": s.Files, "bytes": s.Bytes})
		}
		frows = append(frows, row)
	}

	// Each other device, by name: connected, paused or not connected, with
	// when it was last seen, and how much of what it shares it has.
	name := func(id string) string {
		if d := r.devices[id]; d.Name != "" {
			return d.Name
		}
		short, _, _ := strings.Cut(id, "-")
		return short
	}
	ids := slices.Collect(maps.Keys(r.connections))
	slices.SortFunc(ids, func(a, b string) int { return cmp.Or(cmp.Compare(name(a), name(b)), cmp.Compare(a, b)) })
	drows := []snapshot.Row{}
	for _, id := range ids {
		conn := r.connections[id]
		row := snapshot.Row{Title: snapshot.Text{Name: name(id)}}
		p := percent(r.completions[id])
		args := map[string]any{"percent": p}
		synced := "syncthing.in_sync"
		if p < 100 {
			synced = "syncthing.behind"
		}
		switch seen := r.seen[id].LastSeen; {
		case r.devices[id].Paused || conn.Paused:
			row.Value, row.Tone = snapshot.Msg("syncthing.paused", nil), "gray"
		case conn.Connected:
			row.Value = snapshot.Msg("syncthing.connected", nil)
		case seen.Unix() > 0:
			row.Value, row.Tone = snapshot.Msg("syncthing.disconnected", nil), "gray"
			args["at"], synced = seen, synced+"_seen"
		default:
			row.Value, row.Tone = snapshot.Msg("syncthing.never_connected", nil), "gray"
		}
		row.Detail = snapshot.Msg(synced, args)
		drows = append(drows, row)
	}

	// Its version, or the newer one out in orange, then its errors, in red
	// with the newest, until they're cleared.
	if r.version != "" {
		drows = append(drows, snapshot.VersionRow(r.version, r.newer))
	}
	notices := slices.DeleteFunc(slices.Clone(r.notices), func(n notice) bool { return n.Message == "" })
	if len(notices) > 0 {
		newest := slices.MaxFunc(notices, func(a, b notice) int { return a.When.Compare(b.When) })
		drows = append(drows, snapshot.Row{
			Title: snapshot.Text{Key: "syncthing.errors"}, Tone: "red",
			Value:  snapshot.Msg("syncthing.errors_n", map[string]any{"n": len(notices)}),
			Detail: &snapshot.Text{Name: newest.Message},
		})
	}

	// The chores: each device that asked to connect, by the name it gave
	// and from its address, and each folder offered, by its label and who
	// offers it, accepted or ignored in Syncthing's GUI.
	day := func(t time.Time) string { return t.In(now.Location()).Format(time.DateOnly) }
	chores := []snapshot.Chore{}
	for id, p := range r.pendingDevices {
		chores = append(chores, snapshot.Chore{Name: cmp.Or(p.Name, name(id)), Note: p.Address, From: day(p.Time), URL: link})
	}
	for id, p := range r.pendingFolders {
		var by []string
		var first time.Time
		label := id
		for d, offer := range p.OfferedBy {
			by = append(by, name(d))
			if first.IsZero() || offer.Time.Before(first) {
				first, label = offer.Time, cmp.Or(offer.Label, id)
			}
		}
		slices.Sort(by)
		chores = append(chores, snapshot.Chore{Name: label, Note: strings.Join(by, ", "), From: day(first), URL: link})
	}
	slices.SortFunc(chores, func(a, b snapshot.Chore) int { return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.Name, b.Name)) })

	return snapshot.Parts{
		Sections: []snapshot.Section{
			{Area: Area, Title: snapshot.Text{Key: "syncthing.folders"}, Rows: frows},
			{Area: Area, Title: snapshot.Text{Key: "syncthing.devices"}, Rows: drows},
		},
		Chores: chores,
	}
}
