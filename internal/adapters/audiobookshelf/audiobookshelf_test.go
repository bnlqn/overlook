package audiobookshelf

import (
	"context"
	"path"
	"slices"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is a moment just after testdata was recorded.
var recorded = time.Date(2026, 11, 9, 20, 37, 0, 0, time.UTC)

// testdata holds answers recorded from a scratch Audiobookshelf 2.37.2 on 9
// November 2026 (ADR 0008), which fill.sh fills: six books, Alice gone from
// the disk since its scan, and two podcasts, one with an episode
// downloading and two waiting; kim listening on their phone and the owner
// in a browser; and, since midnight, an hour of kim's and half an hour of
// sam's, who listened for two hours yesterday too.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Name != "Audiobookshelf" {
		t.Errorf("named Audiobookshelf: %q", c.Name)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)

	// Its key with every read, and only the paths on its list.
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != "Bearer 0123abcd" || !slices.ContainsFunc(reads, func(p string) bool { ok, _ := path.Match(p, r.URL.Path); return ok }) {
			t.Errorf("sent %s, without the key or not in reads", r.URL)
		}
	}

	rows := p.Sections[0].Rows
	if len(rows) != 5 {
		t.Fatalf("two listening, two libraries and a podcast's downloads: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "kim" || r.Value.Name != "Frankenstein" || r.Detail.Name != "Fairphone FP5" {
		t.Errorf("kim first, on their phone: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Title.Name != "owner" || r.Detail.Name != "Linux x86_64 Firefox" {
		t.Errorf("then the owner, in a browser: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Audiobooks" || r.Value.Args["n"] != int64(6) || r.Tone != "red" || r.Detail.Key != "audiobookshelf.issues" || r.Detail.Args["n"] != int64(1) {
		t.Errorf("6 books, Alice missing, in red: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[3]; r.Title.Name != "Podcasts" || r.Value.Key != "audiobookshelf.podcasts" || r.Detail.Args["n"] != int64(6) || r.Detail.Args["length"] != 3*3600e3 || r.Tone != "" {
		t.Errorf("2 podcasts of 6 episodes, 3 hours: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[4]; r.Title.Name != "Soldering at Night" || r.Value.Args["n"] != 3 || r.Detail.Args["title"] != "Flux, and Why" {
		t.Errorf("3 episodes to download, one downloading: %+v %+v", r.Value, r.Detail)
	}

	f := p.Figures[0]
	if f.Value != 105 || f.Detail.Args["n"] != 2 {
		t.Errorf("105 minutes since midnight, sam's yesterday left out, 2 listening now: %+v", f)
	}
	if len(f.Shares) != 3 || f.Shares[0].Label.Name != "kim" || f.Shares[0].Value != 70 || len(f.Shares[0].Rows) != 2 || f.Shares[2].Value != 30 {
		t.Errorf("kim's 70 minutes, of two books, the owner's and sam's 30: %+v", f.Shares)
	}
}

// A path off its list never reaches Audiobookshelf, such as these three,
// which restore a backup, match a whole library and empty a podcast's
// downloads, each on GET.
func TestReads(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata", `"total": 1`, `"total": 0`)
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/backups/1/apply", "/api/libraries/1/matchall", "/api/podcasts/1/clear-queue", "/api/libraries/1/items/2"} {
		var v any
		if err := c.get(context.Background(), p, "", &v); err == nil {
			t.Errorf("%s was sent", p)
		}
	}
	if len(src.Sent()) > 0 {
		t.Errorf("sent %v", src.Sent())
	}
	// Then a library with nothing missing isn't red.
	p, err := Read(context.Background(), c, recorded)
	if err != nil || p.Sections[0].Rows[2].Tone != "" || p.Sections[0].Rows[2].Detail.Key != "audiobookshelf.length" {
		t.Errorf("Audiobooks, all there: %+v %v", p.Sections, err)
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
