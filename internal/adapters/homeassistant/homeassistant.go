// Package homeassistant reads Home Assistant: its version, the automations
// that ran today and those that are broken, the devices that dropped off,
// and, as chores, the updates it waits to install and the batteries to
// change.
package homeassistant

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// low is the charge, in percent, below which a battery is a chore.
const low = 20

// Config is one Home Assistant, under sources.homeassistant in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Home Assistant, such as
	// http://homeassistant:8123.
	URL string `yaml:"url"`
	// Token is a long-lived access token, of a user of Overlook's own.
	Token string `yaml:"token"`
	// Link is where the page sends the owner to install an update, if not
	// URL.
	Link string `yaml:"link"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: http.Header{"Authorization": {"Bearer " + c.Token}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Token == "" {
		return errors.New("token is missing")
	}
	c.Name = cmp.Or(c.Name, "Home Assistant")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return nil
}

// entity is one of /api/states: a light, a sensor, an automation, an
// update, each with its state and the attributes of its kind.
type entity struct {
	ID          string    `json:"entity_id"`
	State       string    `json:"state"`
	LastChanged time.Time `json:"last_changed"`
	Attributes  struct {
		Name             string     `json:"friendly_name"`
		DeviceClass      string     `json:"device_class"`
		Unit             string     `json:"unit_of_measurement"`
		LastTriggered    *time.Time `json:"last_triggered"`
		InstalledVersion string     `json:"installed_version"`
		LatestVersion    string     `json:"latest_version"`
	} `json:"attributes"`
}

func (e entity) domain() string { d, _, _ := strings.Cut(e.ID, "."); return d }

func (e entity) name() string { return cmp.Or(e.Attributes.Name, e.ID) }

// Read reads the Home Assistant at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var config struct {
		Version      string `json:"version"`
		SafeMode     bool   `json:"safe_mode"`
		RecoveryMode bool   `json:"recovery_mode"`
	}
	var states []entity
	if err := c.source.JSON(ctx, "/api/config", &config); err != nil {
		return snapshot.Parts{}, err
	}
	if err := c.source.JSON(ctx, "/api/states", &states); err != nil {
		return snapshot.Parts{}, err
	}

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var automations, ran, off int
	var broken, gone []entity
	var chores []snapshot.Chore
	for _, e := range states {
		switch {
		case e.domain() == "automation":
			automations++
			switch {
			case e.State == "unavailable":
				broken = append(broken, e)
			case e.State == "off":
				off++
			case e.Attributes.LastTriggered != nil && !e.Attributes.LastTriggered.Before(midnight):
				ran++
			}
		case e.State == "unavailable":
			gone = append(gone, e)
		case e.domain() == "update" && e.State == "on":
			// An update waiting since the day it came out, done on Home
			// Assistant's page of updates.
			chores = append(chores, snapshot.Chore{
				Name: e.name(), From: e.LastChanged.In(now.Location()).Format(time.DateOnly),
				Note: e.Attributes.InstalledVersion + " → " + e.Attributes.LatestVersion,
				URL:  strings.TrimSuffix(c.Link, "/") + "/config/updates",
			})
		case e.Attributes.DeviceClass == "battery":
			// A battery to change: one that reads its charge below low, or
			// says it's low, as a binary sensor does.
			charge, err := strconv.ParseFloat(e.State, 64)
			switch {
			case e.domain() == "sensor" && e.Attributes.Unit == "%" && err == nil && charge < low:
				chores = append(chores, snapshot.Chore{Name: e.name(), Note: strconv.FormatFloat(charge, 'f', -1, 64) + "%"})
			case e.domain() == "binary_sensor" && e.State == "on":
				chores = append(chores, snapshot.Chore{Name: e.name()})
			}
		}
	}
	slices.SortFunc(chores, func(a, b snapshot.Chore) int { return cmp.Compare(a.Name, b.Name) })

	version := snapshot.Row{Title: snapshot.Text{Key: "homeassistant.version"}, Value: &snapshot.Text{Name: config.Version}}
	switch {
	case config.RecoveryMode:
		version.Value, version.Tone, version.Detail = snapshot.Msg("homeassistant.recovery_mode", nil), "red", &snapshot.Text{Name: config.Version}
	case config.SafeMode:
		version.Value, version.Tone, version.Detail = snapshot.Msg("homeassistant.safe_mode", nil), "orange", &snapshot.Text{Name: config.Version}
	}
	rows := []snapshot.Row{version, {
		Title:  snapshot.Text{Key: "homeassistant.automations"},
		Value:  snapshot.Msg("homeassistant.automations_n", map[string]any{"n": automations}),
		Detail: snapshot.Msg("homeassistant.automations_detail", map[string]any{"n": ran, "off": off}),
	}}
	// An automation whose config Home Assistant couldn't load never runs.
	slices.SortFunc(broken, func(a, b entity) int { return cmp.Compare(a.name(), b.name()) })
	for _, e := range broken {
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Name: e.name()}, Tone: "red",
			Value: snapshot.Msg("homeassistant.broken", nil), Detail: snapshot.Msg("homeassistant.broken_detail", nil),
		})
	}
	sections := []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "homeassistant.home"}, Rows: rows}}

	// The devices that dropped off, the newest first, so a long list still
	// starts with what went lately.
	if len(gone) > 0 {
		slices.SortStableFunc(gone, func(a, b entity) int { return b.LastChanged.Compare(a.LastChanged) })
		var rows []snapshot.Row
		for _, e := range gone {
			rows = append(rows, snapshot.Row{
				Title: snapshot.Text{Name: e.name()}, Detail: &snapshot.Text{Name: e.ID},
				Value: snapshot.Msg("homeassistant.since", map[string]any{"at": e.LastChanged}),
			})
		}
		sections = append(sections, snapshot.Section{Area: Area, Title: snapshot.Text{Key: "homeassistant.unavailable"}, Rows: rows})
	}
	return snapshot.Parts{Sections: sections, Chores: chores}, nil
}
