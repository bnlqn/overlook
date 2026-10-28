// Package nut reads a UPS from NUT's upsd: whether it runs on mains or on its
// battery, how long the battery would last, and what the UPS's load draws.
package nut

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// area is where a UPS goes: beside the machines it keeps running.
const area = "servers"

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
	if c.Name == "" {
		c.Name = "NUT"
	}
	return nil
}

// Read reads the UPS at c.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
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
	rows := []snapshot.Row{{Title: key("nut.power"), Value: power, Tone: tone, Detail: about(v)}}
	var battery *snapshot.Meter
	if charge, ok := number(v, "battery.charge"); ok {
		battery = &snapshot.Meter{Used: charge, Total: 100, Tone: "green"}
		if charge < 50 {
			battery.Tone = "red"
		}
		rows = append(rows, snapshot.Row{Title: key("nut.battery"), Value: with("nut.percent", "n", charge), Detail: runtime(v), Meter: battery})
	}
	load, hasLoad := number(v, "ups.load")
	w, hasWatts := watts(v)
	switch {
	case hasWatts:
		row := snapshot.Row{Title: key("nut.drawing"), Value: with("nut.watts", "n", w)}
		if hasLoad {
			row.Detail = with("nut.load_of", "n", load)
		}
		rows = append(rows, row)
	case hasLoad:
		rows = append(rows, snapshot.Row{Title: key("nut.load"), Value: with("nut.percent", "n", load)})
	}

	p := snapshot.Parts{Sections: []snapshot.Section{{Area: area, Title: key("nut.ups"), Rows: rows}}}
	if hasWatts {
		f := snapshot.Figure{Area: area, Icon: "plug", Label: key("nut.ups"), Value: w, Unit: ptr(key("nut.watts_unit")), Detail: power}
		if tone == "red" {
			f.Tone = "red"
		}
		if battery != nil {
			m := *battery
			m.Label = ptr(key("nut.battery"))
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
		return ptr(key("nut.on_battery")), "red"
	case slices.Contains(flags, "OL"):
		return ptr(key("nut.on_mains")), "green"
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
		return with("nut.in", "v", math.Round(volts))
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
	return with("nut.runtime", "ms", secs*1000)
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

func key(k string) snapshot.Text { return snapshot.Text{Key: k} }

func with(k, arg string, n float64) *snapshot.Text {
	return &snapshot.Text{Key: k, Args: map[string]any{arg: n}}
}

func ptr[T any](x T) *T { return &x }
