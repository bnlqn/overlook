package homeassistant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Home Assistant 2026.10.4
// (ADR 0008), with Home Assistant's own invented devices and
// testdata/configuration.yaml's: three batteries, two of them low, a garage
// door that dropped off, two automations, one of which ran, and one whose
// config is broken. Its demo updates wait for five installs. The people and
// where they are, and the cameras' tokens, were left out as it was recorded.
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
	p, err := read(context.Background(), c, time.Date(2026, 10, 29, 10, 55, 0, 0, london))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "homeassistant.home" || p.Sections[1].Title.Key != "homeassistant.unavailable" {
		t.Fatalf("Home, and what's unavailable: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("the version, the automations and the broken one: %+v", rows)
	}
	if r := rows[0]; r.Value.Name != "2026.10.4" || r.Tone != "" {
		t.Errorf("the version, neither safe nor in recovery: %+v", r)
	}
	if r := rows[1]; r.Value.Args["n"] != 3 || r.Detail.Args["n"] != 1 || r.Detail.Args["off"] != 0 {
		t.Errorf("3 automations, the porch light's run today: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Heating boost" || r.Tone != "red" {
		t.Errorf("the heating boost, broken: %+v", r)
	}
	if gone := p.Sections[1].Rows; len(gone) != 1 || gone[0].Title.Name != "Garage door" || gone[0].Detail.Name != "binary_sensor.garage_door" {
		t.Errorf("the garage door dropped off, and the broken automation isn't listed twice: %+v", gone)
	}

	var updates, batteries []string
	for _, ch := range p.Chores {
		if ch.URL != "" {
			updates = append(updates, ch.Name)
			if ch.URL != "https://home.example.com/config/updates" || ch.From != "2026-10-29" || !strings.Contains(ch.Note, "→") {
				t.Errorf("an update, installed from the page of updates, waiting since today: %+v", ch)
			}
		} else {
			batteries = append(batteries, ch.Name+" "+ch.Note)
		}
	}
	if len(updates) != 5 {
		t.Errorf("five updates waiting, the one up to date left out: %v", updates)
	}
	if strings.Join(batteries, ", ") != "Garden gate battery 6%, Hall motion battery 12%, Outside Temperature Battery 12%" {
		t.Errorf("the three batteries below 20%%, by name: %v", batteries)
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
