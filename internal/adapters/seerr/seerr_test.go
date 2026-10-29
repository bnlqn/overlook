package seerr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Seerr 3.5.1 (ADR 0008),
// which testdata/fill.sh fills: Kim asked for Nosferatu and Metropolis, and
// Sam for The General, which the owner approved, Night of the Living Dead,
// which the owner declined, and The Twilight Zone's first season. Sam
// reported that The General's picture freezes.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd", Link: "https://requests.example.com"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	for _, r := range src.Sent() {
		if r.Header.Get("X-Api-Key") != "0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
	}

	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("the requests, the problem reported and the version: %+v", rows)
	}
	if r := rows[0]; r.Value.Args["n"] != 3 || r.Detail.Args["n"] != 1 || r.Detail.Args["available"] != 0 {
		t.Errorf("3 waiting, 1 on its way: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Tone != "orange" || r.Value.Args["n"] != 1 {
		t.Errorf("one problem reported: %+v", r)
	}
	if r := rows[2]; r.Value.Name != "3.5.1" || r.Tone != "" {
		t.Errorf("the version: %+v", r)
	}

	var names []string
	for _, ch := range p.Chores {
		names = append(names, ch.Name+" by "+ch.Note)
	}
	if got := strings.Join(names, ", "); got != "The Twilight Zone (1959) by Sam, Metropolis (1927) by Kim, Nosferatu (1922) by Kim" {
		t.Errorf("the three waiting, the newest first, by title and who asked: %s", got)
	}
	if ch := p.Chores[2]; ch.URL != "https://requests.example.com/movie/653" || ch.From != time.Date(2026, 10, 29, 12, 45, 53, 0, time.UTC).In(time.Local).Format(time.DateOnly) {
		t.Errorf("Nosferatu, approved on its page, waiting since the day it was asked for: %+v", ch)
	}
}

// A film's title is read once, and kept, however often its request is read.
func TestTitlesOnce(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := Read(context.Background(), c, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	films := 0
	for _, r := range src.Sent() {
		if strings.HasPrefix(r.URL.Path, "/api/v1/movie/") || strings.HasPrefix(r.URL.Path, "/api/v1/tv/") {
			films++
		}
	}
	if films != 3 {
		t.Errorf("three reads asked for %d titles, want each of the three once", films)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://seerr:5055", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Seerr" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "k"}, "url is missing"},
		{Config{URL: "seerr:5055", Key: "k"}, "url needs"},
		{Config{URL: "http://seerr:5055"}, "key is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
