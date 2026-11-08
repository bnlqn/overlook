package syncthing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is the moment testdata was recorded.
var recorded = time.Date(2026, 11, 7, 23, 30, 0, 0, time.UTC)

// testdata holds answers recorded from home, a device of a scratch cluster
// of Syncthing 2.1.7 (compose.yaml, fill.sh and record.sh beside them):
// Photos, up to date; Documents, with a file it couldn't write; Music,
// paused; Videos, on a path it couldn't make; laptop, connected; nas,
// stopped two photos behind; phone, asking to connect; and Recipes, a folder
// laptop offers. Syncthing's Docker image is built without upgrades, so it
// refuses to say whether a newer one is out.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	for _, r := range src.Sent() {
		if r.Header.Get("X-Api-Key") != "0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
	}

	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "syncthing.folders" || p.Sections[1].Title.Key != "syncthing.devices" {
		t.Fatalf("two sections, the folders and the devices: %+v", p.Sections)
	}
	folders := p.Sections[0].Rows
	want := []struct{ name, key, tone, detail string }{
		{"Documents", "syncthing.failed", "red", "syncthing.failed_at"},
		{"Music", "syncthing.paused", "gray", ""},
		{"Photos", "syncthing.up_to_date", "", "syncthing.holds"},
		{"Videos", "syncthing.stopped", "red", ""},
	}
	if len(folders) != len(want) {
		t.Fatalf("four folders, by label: %+v", folders)
	}
	for i, w := range want {
		r := folders[i]
		if r.Title.Name != w.name || r.Value.Key != w.key || r.Tone != w.tone || (w.detail != "" && r.Detail.Key != w.detail) {
			t.Errorf("%s: want %s in %q, got %+v", w.name, w.key, w.tone, r)
		}
	}
	if d := folders[0].Detail.Args; d["path"] != "Taxes/2026.txt" || !strings.HasSuffix(d["error"].(string), "permission denied") {
		t.Errorf("Documents' failure, with its path and why: %v", d)
	}
	if d := folders[2].Detail.Args; d["n"] != 8 || d["bytes"] != int64(240) {
		t.Errorf("Photos holds 8 files of 240 bytes: %v", d)
	}
	if d := folders[3].Detail; d == nil || d.Name != "folder path missing" {
		t.Errorf("Videos stopped, as Syncthing words why: %+v", d)
	}

	devices := p.Sections[1].Rows
	if len(devices) != 4 {
		t.Fatalf("two devices, the version and the errors: %+v", devices)
	}
	laptop, nas, version, errs := devices[0], devices[1], devices[2], devices[3]
	if laptop.Title.Name != "laptop" || laptop.Value.Key != "syncthing.connected" || laptop.Detail.Key != "syncthing.in_sync" {
		t.Errorf("laptop is connected and in sync: %+v", laptop)
	}
	if nas.Title.Name != "nas" || nas.Value.Key != "syncthing.disconnected" || nas.Tone != "gray" ||
		nas.Detail.Key != "syncthing.behind_seen" || nas.Detail.Args["percent"] != 75.0 ||
		!nas.Detail.Args["at"].(time.Time).Equal(time.Date(2026, 11, 7, 23, 29, 37, 0, time.UTC)) {
		t.Errorf("nas is disconnected, last seen when it stopped, 75%% in sync: %+v", nas)
	}
	if version.Value.Name != "v2.1.7" || version.Tone != "" {
		t.Errorf("Syncthing's version, alone, since the image can't say if a newer one is out: %+v", version)
	}
	if errs.Value.Args["n"] != 2 || errs.Tone != "red" || !strings.HasPrefix(errs.Detail.Name, "Failed initial scan") {
		t.Errorf("two errors, the newest shown: %+v", errs)
	}

	if len(p.Chores) != 2 {
		t.Fatalf("two chores, a folder and a device to accept: %+v", p.Chores)
	}
	recipes, phone := p.Chores[0], p.Chores[1]
	if recipes.Name != "Recipes" || recipes.Note != "laptop" || recipes.From != "2026-11-07" || recipes.URL != src.URL {
		t.Errorf("Recipes, offered by laptop, accepted in Syncthing: %+v", recipes)
	}
	if phone.Name != "phone" || phone.Note != "172.18.0.3:22000" || phone.From != "2026-11-07" {
		t.Errorf("phone, from its address: %+v", phone)
	}
}

// A release of Syncthing, built with upgrades, says when a newer one is out.
// Its answer is the one a release binary of 2.1.7 gave on 7 November 2026,
// with the latest moved on.
func TestReadNewer(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata",
		"HTTP/1.1 501 Not Implemented", "HTTP/1.1 200 OK",
		"text/plain; charset=utf-8", "application/json",
		"upgrade unsupported", `{"latest": "v2.1.8", "majorNewer": false, "newer": true, "running": "v2.1.7"}`)
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	if v := p.Sections[1].Rows[2]; v.Value.Key != "version.newer" || v.Value.Args["version"] != "v2.1.8" ||
		v.Tone != "orange" || v.Detail.Name != "v2.1.7" {
		t.Errorf("v2.1.8 is out, in orange, over v2.1.7: %+v", v)
	}
}

// A folder that needs what it can't get yet is out of sync, one syncing says
// so, and a device never seen says that too.
func TestParts(t *testing.T) {
	t.Parallel()
	r := read{
		folders: []folder{{ID: "a", Label: "Away"}, {ID: "b", Label: "Busy"}},
		states: map[string]state{
			"a": {State: "idle", Bytes: 1000, NeedBytes: 1, NeedItems: 1},
			"b": {State: "syncing", Bytes: 1000, NeedBytes: 500, NeedItems: 3},
		},
		connections: map[string]struct {
			Connected bool `json:"connected"`
			Paused    bool `json:"paused"`
		}{"ABCDEFG-HIJKLMN": {}},
	}
	p := parts(r, "http://syncthing.example.com:8384", recorded)
	adaptertest.Validate(t, Area, p)
	rows := p.Sections[0].Rows
	if rows[0].Value.Key != "syncthing.out_of_sync" || rows[0].Value.Args["percent"] != 99.0 || rows[0].Tone != "orange" {
		t.Errorf("a byte short is 99%%, out of sync: %+v", rows[0])
	}
	if rows[1].Value.Key != "syncthing.syncing" || rows[1].Value.Args["percent"] != 50.0 || rows[1].Tone != "blue" ||
		rows[1].Detail.Args["n"] != 3 {
		t.Errorf("syncing, halfway, 3 items to go: %+v", rows[1])
	}
	if d := p.Sections[1].Rows[0]; d.Title.Name != "ABCDEFG" || d.Value.Key != "syncthing.never_connected" || d.Detail.Key != "syncthing.behind" {
		t.Errorf("a device with no name, never connected, by its ID's first part: %+v", d)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://syncthing:8384", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Syncthing" || c.Link != "http://syncthing:8384" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "http://syncthing:8384"}, "key is missing"},
		{Config{URL: "http://syncthing:8384", Key: "0123abcd", Link: "syncthing.example.com"}, "link"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{URL: url, Key: "0123abcd"}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
