package homeassistant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from the author's Home Assistant 2026.10.3,
// with every name, place and number its own replaced, and every time moved
// by the same amount: 1,355 entities, 38 automations, one of whose config is
// broken, 31 entities that dropped off, and four updates waiting. The people
// and where they are were left out, and the attributes the adapter doesn't
// read, but for a few that give each kind its shape. Two batteries were
// nudged below 20%, since the house had none that day.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "0123abcd", Link: "https://home.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 27, 11, 18, 0, 0, london))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != "Bearer 0123abcd" {
			t.Errorf("%s: read without the token", r.URL)
		}
	}

	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "homeassistant.home" || p.Sections[1].Title.Key != "homeassistant.unavailable" {
		t.Fatalf("Home, and what's unavailable: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("the version, the automations and the broken one: %+v", rows)
	}
	if r := rows[0]; r.Value.Name != "2026.10.3" || r.Tone != "" {
		t.Errorf("the version, neither safe nor in recovery: %+v", r)
	}
	if r := rows[1]; r.Value.Args["n"] != 38 || r.Detail.Args["n"] != 16 || r.Detail.Args["off"] != 0 {
		t.Errorf("38 automations, 16 of them run today: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Attic bedtime" || r.Tone != "red" {
		t.Errorf("the attic's bedtime, broken: %+v", r)
	}
	gone := p.Sections[1].Rows
	if len(gone) != 31 || gone[0].Title.Name != "Garden last seen 2" || gone[0].Detail.Name != "sensor.garden_last_seen_2" {
		t.Errorf("31 dropped off, the newest first, and the broken automation isn't listed twice: %+v", gone)
	}

	var updates, batteries []string
	for _, ch := range p.Chores {
		if ch.URL != "" {
			updates = append(updates, ch.Name)
			if ch.URL != "https://home.example.com/config/updates" || ch.From < "2026-10-25" || !strings.Contains(ch.Note, "→") {
				t.Errorf("an update, installed from the page of updates, waiting since the day it came: %+v", ch)
			}
		} else {
			batteries = append(batteries, ch.Name+" "+ch.Note)
		}
	}
	if strings.Join(updates, ", ") != "Bin Day Update, Energy Tariff Update, Home Assistant Core Update, Home Assistant Operating System Update" {
		t.Errorf("four updates waiting, the 34 up to date and the 2 unknown left out: %v", updates)
	}
	if strings.Join(batteries, ", ") != "Bedroom battery 14%, Bedroom battery 2 6%" {
		t.Errorf("the two batteries below 20%%, by name, of the 30 that give their charge: %v", batteries)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://homeassistant:8123", Token: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Home Assistant" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Token: "0123abcd"}, "url is missing"},
		{Config{URL: "homeassistant:8123", Token: "0123abcd"}, "url needs"},
		{Config{URL: "http://homeassistant:8123"}, "token is missing"},
		{Config{URL: "http://homeassistant:8123", Token: "0123abcd", Link: "home"}, "link needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
