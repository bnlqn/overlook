// Package nut reads a UPS from NUT's upsd: whether it runs on mains or on its
// battery, how long the battery would last, and what the UPS's load draws.
package nut

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area: a UPS goes beside the machines it keeps
// running.
const Area = "servers"

// Config is one UPS, under sources.nut in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// Host is where upsd listens, as host or host:port; its port is 3493
	// unless it says.
	Host string `yaml:"host"`
	// UPS is the UPS's name in upsd, as in its ups.conf.
	UPS string `yaml:"ups"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.Host == "" {
		return errors.New("host is missing")
	}
	addr, err := fetch.CheckNUTHost(c.Host)
	if err != nil {
		return fmt.Errorf("host: %w", err)
	}
	c.Host = addr
	if c.UPS == "" {
		return errors.New("ups is missing")
	}
	if err := fetch.CheckNUTName(c.UPS); err != nil {
		return fmt.Errorf("ups: %w", err)
	}
	c.Name = cmp.Or(c.Name, "NUT")
	return nil
}

// Read reads the UPS at c.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	v, err := fetch.NUTVars(ctx, c.Host, c.UPS)
	if err != nil {
		return snapshot.Parts{}, err
	}
	return parts(v), nil
}

// parts is the UPS section on its area's page and, when upsd says what the
// load draws, its figure on the overview. A reading upsd doesn't give is left
// out.
func parts(v map[string]string) snapshot.Parts {
	power, tone := power(v["ups.status"])
	rows := []snapshot.Row{{Title: snapshot.Text{Key: "nut.power"}, Value: power, Tone: tone, Detail: about(v)}}
	var battery *snapshot.Meter
	if charge, ok := number(v, "battery.charge"); ok {
		battery = &snapshot.Meter{Used: charge, Total: 100, Tone: "green"}
		if charge < 50 {
			battery.Tone = "red"
		}
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "nut.battery"}, Value: snapshot.Percent(charge), Detail: runtime(v), Meter: battery})
	}
	load, hasLoad := number(v, "ups.load")
	w, hasWatts := watts(v)
	switch {
	case hasWatts:
		row := snapshot.Row{Title: snapshot.Text{Key: "nut.drawing"}, Value: snapshot.Msg("nut.watts", map[string]any{"n": w})}
		if hasLoad {
			row.Detail = snapshot.Msg("nut.load_of", map[string]any{"n": load})
		}
		rows = append(rows, row)
	case hasLoad:
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "nut.load"}, Value: snapshot.Percent(load)})
	}

	p := snapshot.Parts{Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "nut.ups"}, Rows: rows}}}
	if hasWatts {
		f := snapshot.Figure{Area: Area, Icon: "plug", Label: snapshot.Text{Key: "nut.ups"}, Value: w, Unit: snapshot.Msg("nut.watts_unit", nil), Detail: power}
		if tone == "red" {
			f.Tone = "red"
		}
		if battery != nil {
			m := *battery
			m.Label = snapshot.Msg("nut.battery", nil)
			f.Meters = []snapshot.Meter{m}
		}
		p.Figures = []snapshot.Figure{f}
	}
	return p
}

// power is what ups.status says of the UPS's power, in the tone it calls for:
// on mains, on battery, or upsd's own words for anything else, such as OFF.
func power(status string) (*snapshot.Text, string) {
	flags := strings.Fields(status)
	switch {
	case slices.Contains(flags, "OB"):
		return snapshot.Msg("nut.on_battery", nil), "red"
	case slices.Contains(flags, "OL"):
		return snapshot.Msg("nut.on_mains", nil), "green"
	}
	return &snapshot.Text{Name: status}, "gray"
}

// about is the UPS's model and the voltage it's fed.
func about(v map[string]string) *snapshot.Text {
	model := strings.TrimSpace(v["ups.model"])
	volts, ok := number(v, "input.voltage")
	switch {
	case ok && model != "":
		return &snapshot.Text{Key: "nut.model_in", Args: map[string]any{"model": model, "v": math.Round(volts)}}
	case ok:
		return snapshot.Msg("nut.in", map[string]any{"v": math.Round(volts)})
	case model != "":
		return &snapshot.Text{Name: model}
	}
	return nil
}

// runtime is how long the battery would last at this load, and the charge
// at which the UPS calls it low, when the machines it feeds shut down.
func runtime(v map[string]string) *snapshot.Text {
	secs, ok := number(v, "battery.runtime")
	if !ok {
		return nil
	}
	if low, ok := number(v, "battery.charge.low"); ok {
		return &snapshot.Text{Key: "nut.runtime_low", Args: map[string]any{"ms": secs * 1000, "low": low}}
	}
	return snapshot.Msg("nut.runtime", map[string]any{"ms": secs * 1000})
}

// watts is what the UPS's load draws: upsd's reading, or for a UPS that only
// says its load, that share of the power it can deliver.
func watts(v map[string]string) (float64, bool) {
	if w, ok := number(v, "ups.realpower"); ok {
		return w, true
	}
	load, ok := number(v, "ups.load")
	nominal, ok2 := number(v, "ups.realpower.nominal")
	return math.Round(load * nominal / 100), ok && ok2
}

func number(v map[string]string, name string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(v[name]), 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}
