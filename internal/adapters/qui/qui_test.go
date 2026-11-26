package qui

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is when record.sh read the scratch Qui, 15 seconds after its
// rules ran.
var recorded = time.Date(2026, 11, 26, 10, 40, 6, 0, time.UTC)

// testdata holds the answers of a scratch Qui 1.36.1, recorded by record.sh
// with a key that only reads, from the instance fill.sh filled: Home, a
// firewalled qBittorrent 5.3.1, downloading two, seeding two, with one
// stopped and one whose file is missing, and two rules, Finished films, which
// removed two films, and Old episodes, in dry run, which would have removed
// one; and Seedbox, a qBittorrent Qui can't reach.
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
	adaptertest.Validate(t, Adapter, p)
	for _, r := range src.Sent() {
		if r.Method != http.MethodGet || r.Header.Get("X-API-Key") != "0123abcd" {
			t.Errorf("%s %s: not a GET, or without the key", r.Method, r.URL)
		}
		if r.URL.Path == "/api/instances/2/torrents" {
			t.Errorf("read the torrents of Seedbox, which Qui can't reach")
		}
	}

	if len(p.Sections) != 3 {
		t.Fatalf("Home's torrents, Seedbox's and Qui: %+v", p.Sections)
	}
	home := p.Sections[0]
	if home.Title.Key != "qui.torrents_on" || home.Title.Args["name"] != "Home" || len(home.Rows) != 5 {
		t.Fatalf("Home's torrents, named as one of two, and the one missing its file: %+v", home)
	}
	if r := home.Rows[0]; r.Title.Key != "qbittorrent.downloading" || r.Value.Args["n"] != 2 {
		t.Errorf("two waiting for their metadata are downloading, as qBittorrent counts them: %+v", r)
	}
	if r := home.Rows[1]; r.Value.Args["n"] != 2 {
		t.Errorf("two seeding, the stopped one left out: %+v", r.Value)
	}
	if r := home.Rows[2]; r.Value.Args["n"] != int64(893_434_880_000) {
		t.Errorf("the room left on its disk: %+v", r.Value)
	}
	if r := home.Rows[3]; r.Value.Key != "qbittorrent.firewalled" || r.Tone != "orange" {
		t.Errorf("firewalled, in orange: %+v", r)
	}
	if r := home.Rows[4]; r.Title.Name != "The.General.1926.1080p.mkv" || r.Value.Key != "qbittorrent.missing_files" || r.Detail.Name != "/downloads/films" {
		t.Errorf("the film whose file is missing, with where it's saved: %+v", r)
	}
	if s := p.Sections[1]; s.Title.Args["name"] != "Seedbox" || len(s.Rows) != 1 || s.Rows[0].Value.Key != "qui.unreached" ||
		s.Rows[0].Tone != "red" || !strings.HasSuffix(s.Rows[0].Detail.Name, "lookup seedbox on 127.0.0.11:53: no such host") {
		t.Errorf("Seedbox, which Qui can't reach, with why, from its last error: %+v %+v", s, s.Rows[0].Detail)
	}
	qui := p.Sections[2]
	if qui.Title.Key != "qui.title" || len(qui.Rows) != 3 || qui.Rows[0].Value.Name != "v1.36.1" || qui.Rows[0].Tone != "" {
		t.Fatalf("Qui's version, the newest, and Home's two rules: %+v", qui)
	}
	if r := qui.Rows[1]; r.Title.Name != "Finished films" || r.Value.Key != "qui.on" || r.Tone != "" || r.Detail.Args["name"] != "Home" {
		t.Errorf("the rule that removes, on Home: %+v", r)
	}
	if r := qui.Rows[2]; r.Title.Name != "Old episodes" || r.Value.Key != "qui.dry_run" || r.Tone != "gray" {
		t.Errorf("the rule in dry run, in grey: %+v", r)
	}

	if len(p.Events) != 2 {
		t.Fatalf("what the rules removed and would have: %+v", p.Events)
	}
	if e := p.Events[0]; e.Text.Key != "qui.would_remove" || e.Text.Args["n"] != 1 || e.Tone != "gray" {
		t.Errorf("the dry run, which Qui doesn't name the rule of: %+v", e)
	}
	if e := p.Events[1]; e.Text.Key != "qui.removed" || e.Text.Args["rule"] != "Finished films" || e.Text.Args["n"] != 2 ||
		!e.At.Equal(time.Date(2026, 11, 26, 10, 39, 51, 0, time.UTC)) {
		t.Errorf("the two films one rule removed, as one event: %+v", e)
	}
	if f := p.Figures[0]; f.Label.Key != "qbittorrent.torrents" || f.Value != 2 || f.Detail.Args["n"] != 2 || f.Tone != "orange" || !f.Adds {
		t.Errorf("Home's two downloading, adding up with every qBittorrent's: %+v", f)
	}

	week, err := Read(context.Background(), c, recorded.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if len(week.Events) != 0 {
		t.Errorf("a week on, nothing the rules did this week: %+v", week.Events)
	}
}

// read reads the recorded answers with each old text replaced by its new one.
func read(t *testing.T, replace ...string) snapshot.Parts {
	t.Helper()
	c := Config{URL: adaptertest.Replay(t, "testdata", replace...).URL}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	return p
}

// What Qui says beside: a newer Qui, why it can't reach a qBittorrent, an
// instance disabled, a rule off, and a removal that failed.
func TestSays(t *testing.T) {
	t.Parallel()
	p := read(t,
		`"updateAvailable": false`, `"updateAvailable": true, "latestVersion": "v1.37.0"`,
		`"name": "Seedbox",`, `"name": "Seedbox", "connectionError": "dial tcp: lookup seedbox: no such host",`,
		`"enabled": true,
    "dryRun": true`, `"enabled": false,
    "dryRun": true`,
		`"torrentName": "Nosferatu.1922.1080p.mkv",
    "action": "deleted_condition"`, `"torrentName": "Nosferatu.1922.1080p.mkv",
    "action": "delete_failed"`,
	)
	if r := p.Sections[2].Rows[0]; r.Value.Key != "version.newer" || r.Value.Args["version"] != "v1.37.0" || r.Tone != "orange" {
		t.Errorf("the newer Qui, in orange: %+v", r)
	}
	if r := p.Sections[1].Rows[0]; r.Detail == nil || r.Detail.Name != "dial tcp: lookup seedbox: no such host" {
		t.Errorf("why Qui can't reach Seedbox: %+v", r)
	}
	if r := p.Sections[2].Rows[2]; r.Value.Key != "qui.off" || r.Tone != "gray" {
		t.Errorf("the rule turned off, in grey: %+v", r)
	}
	if len(p.Events) != 3 || p.Events[1].Text.Key != "qui.remove_failed" || p.Events[1].Tone != "red" || p.Events[2].Text.Args["n"] != 1 {
		t.Errorf("the film it failed to remove in red, and the one it removed: %+v", p.Events)
	}

	p = read(t, `"isActive": true,
    "sshPort": 22,
    "sshHostKeyPinned": false,
    "filesystemMode": "none",
    "capabilities": {
      "read": false,
      "identity": false,
      "trustedIdentity": false,
      "write": false,
      "content": false
    },
    "reannounceSettings": {
      "enabled": false,
      "initialWaitSeconds": 15,
      "reannounceIntervalSeconds": 7,
      "maxAgeSeconds": 600,
      "maxRetries": 50,
      "aggressive": false,
      "monitorAll": true,
      "excludeCategories": false,
      "categories": [],
      "excludeTags": false,
      "tags": [],
      "excludeTrackers": false,
      "trackers": []
    },
    "publicPeerAddress": "",
    "resolvedPublicPeerAddress": ""
  }
]`, `"isActive": false
  }
]`)
	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "qbittorrent.torrents" || p.Figures[0].Tone != "orange" {
		t.Errorf("Seedbox disabled, so left out, and Home's torrents titled as qBittorrent's: %+v", p.Sections)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	if err := (&Config{}).Check(); err == nil || !strings.Contains(err.Error(), "url is missing") {
		t.Errorf("no url should be refused: %v", err)
	}
	c := Config{URL: "http://qui:7476"}
	if err := c.Check(); err != nil || c.Name != "Qui" || c.source.Header != nil {
		t.Errorf("a url is enough, named Qui, for a Qui whose login is off: %v %+v", err, c)
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
