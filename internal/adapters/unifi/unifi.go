// Package unifi reads UniFi Network on a UniFi OS console: each WAN, each
// device and its state, the clients connected and, as chores, the firmware
// waiting to be installed, the devices waiting to be adopted and the Network
// application's update. Everything it gives goes in Network.
package unifi

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
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "network"

// integration is where the console serves the Network application's
// documented API. older is where it serves the API before it, which nothing
// documents, but which alone says how the WANs are and whether the
// application has an update: the same key reads both.
const (
	integration = "/proxy/network/integration/v1"
	older       = "/proxy/network/api/s/"
)

// boot is how soon after the gateway starts a WAN that's down is taken to
// have been down since it started: its monitors start a minute or so later.
//
// ponytail: a guess from one console. If a WAN that dropped shows as never
// up, or the other way round, tune it.
const boot = 10 * time.Minute

// Config is one UniFi OS console, under sources.unifi in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads the console, such as https://192.168.1.1.
	URL string `yaml:"url"`
	// Key is an API key, from the Network application's Integrations.
	Key string `yaml:"key"`
	// CA is a PEM file of the console's certificate, which signs itself,
	// when the system doesn't trust it, and ServerName the name it is for,
	// unifi.local for a console's own, when URL reaches it by its address.
	CA         string `yaml:"ca"`
	ServerName string `yaml:"server_name"`
	// Link is where the page sends the owner to update or adopt a device,
	// if not URL, such as the console's page on unifi.ui.com.
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
	c.Name = cmp.Or(c.Name, "UniFi")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return nil
}

// list is one page of a list the Integration API gives, which counts them
// all in TotalCount.
type list[T any] struct {
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}

type site struct {
	ID string `json:"id"`
	// Ref is the site's short name, by which the older API knows it.
	Ref  string `json:"internalReference"`
	Name string `json:"name"`
}

// device is an adopted device: a gateway, a switch or an access point.
type device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	State    string `json:"state"`
	Firmware string `json:"firmwareVersion"`
	// Updatable says that newer firmware waits to be installed. A gateway
	// console's firmware is UniFi OS itself.
	Updatable bool `json:"firmwareUpdatable"`
}

// pending is a device waiting to be adopted, which has no name yet.
type pending struct {
	Model string `json:"model"`
	MAC   string `json:"macAddress"`
}

// stats are an online device's latest statistics.
type stats struct {
	Uptime int64   `json:"uptimeSec"`
	CPU    float64 `json:"cpuUtilizationPct"`
	Mem    float64 `json:"memoryUtilizationPct"`
}

// health is the older API's stat/health, a line for each part of the site,
// of which the wan and www lines are read.
type health struct {
	Data []line `json:"data"`
}

type line struct {
	Subsystem string `json:"subsystem"`
	// The wan line's: how long the gateway has been up, in seconds, and each
	// WAN, by the name the gateway gives its port, such as WAN2.
	Gateway struct {
		Uptime string `json:"uptime"`
	} `json:"gw_system-stats"`
	WANs map[string]wan `json:"uptime_stats"`
	// The www line's: the last speed test, run at a moment in seconds, 0 for
	// none, its speeds in Mbit/s and its ping in milliseconds.
	Speedtest int64   `json:"speedtest_lastrun"`
	Down      float64 `json:"xput_down"`
	Up        float64 `json:"xput_up"`
	Ping      float64 `json:"speedtest_ping"`
}

// wan is how one WAN has been: up for Uptime seconds, or down for Downtime,
// and over the last Period seconds how much of the time it answered its
// monitors, in percent, and how fast, in milliseconds.
type wan struct {
	Uptime       int64    `json:"uptime"`
	Downtime     *int64   `json:"downtime"`
	Availability *float64 `json:"availability"`
	Latency      *float64 `json:"latency_average"`
	Period       int64    `json:"time_period"`
}

// sysinfo is the older API's stat/sysinfo, about the Network application.
type sysinfo struct {
	Data []struct {
		Version string `json:"version"`
		Update  bool   `json:"update_available"`
	} `json:"data"`
}

// read is what Read reads of one site.
type read struct {
	site    site
	devices []device
	// stats are the online devices', by ID.
	stats map[string]stats
	// clients are all the site's clients, the wired and the wireless
	// among them, and the rest on its VPN or Teleport.
	clients, wired, wireless int
	// health is nil when the older API didn't give it.
	health *health
}

// Read reads the console at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var sites list[site]
	var waiting list[pending]
	if err := c.source.JSON(ctx, integration+"/sites", &sites); err != nil {
		return snapshot.Parts{}, err
	}
	if err := c.source.JSON(ctx, integration+"/pending-devices", &waiting); err != nil {
		return snapshot.Parts{}, err
	}
	var all []read
	for _, s := range sites.Data {
		r, err := readSite(ctx, c, s)
		if err != nil {
			return snapshot.Parts{}, err
		}
		all = append(all, r)
	}
	// The application is the console's, whichever site asks.
	var app *sysinfo
	if len(all) > 0 {
		var info sysinfo
		if c.source.JSON(ctx, older+url.PathEscape(all[0].site.Ref)+"/stat/sysinfo", &info) == nil {
			app = &info
		}
	}
	return parts(all, waiting.Data, app, c.Link, now), nil
}

// readSite reads s. The documented API's reads decide whether the console
// answered; the older one's, which an upgrade may move, leave out only their
// own rows when they fail.
func readSite(ctx context.Context, c Config, s site) (read, error) {
	r := read{site: s, stats: map[string]stats{}}
	at := integration + "/sites/" + url.PathEscape(s.ID)
	// ponytail: one page of 200, the most the API gives. A site with more
	// devices will need the next pages.
	var devices list[device]
	if err := c.source.JSON(ctx, at+"/devices?limit=200", &devices); err != nil {
		return r, err
	}
	r.devices = devices.Data
	for _, d := range r.devices {
		if d.State != "ONLINE" {
			continue
		}
		var st stats
		if err := c.source.JSON(ctx, at+"/devices/"+url.PathEscape(d.ID)+"/statistics/latest", &st); err != nil {
			return r, err
		}
		r.stats[d.ID] = st
	}
	// Each count with no client in it, which the limit of 0 leaves out.
	for _, count := range []struct {
		n    *int
		kind string
	}{{&r.clients, ""}, {&r.wired, "WIRED"}, {&r.wireless, "WIRELESS"}} {
		q := url.Values{"limit": {"0"}}
		if count.kind != "" {
			q.Set("filter", "type.eq('"+count.kind+"')")
		}
		var clients list[struct{}]
		if err := c.source.JSON(ctx, at+"/clients?"+q.Encode(), &clients); err != nil {
			return r, err
		}
		*count.n = clients.TotalCount
	}
	var h health
	if c.source.JSON(ctx, older+url.PathEscape(s.Ref)+"/stat/health", &h) == nil {
		r.health = &h
	}
	return r, nil
}

// states are the words and tones of a device that isn't online.
var states = map[string]struct{ key, tone string }{
	"CONNECTION_INTERRUPTED": {"unifi.interrupted", "orange"},
	"ISOLATED":               {"unifi.isolated", "orange"},
	"U5G_INCORRECT_TOPOLOGY": {"unifi.wrong_topology", "orange"},
	"OFFLINE":                {"unifi.offline", "red"},
	"UPDATING":               {"unifi.updating", "blue"},
	"GETTING_READY":          {"unifi.getting_ready", "blue"},
	"ADOPTING":               {"unifi.adopting", "blue"},
	"PENDING_ADOPTION":       {"unifi.pending", "gray"},
	"DELETING":               {"unifi.deleting", "gray"},
}

func parts(all []read, waiting []pending, app *sysinfo, link string, now time.Time) snapshot.Parts {
	link = strings.TrimSuffix(link, "/")
	var sections []snapshot.Section
	var chores []snapshot.Chore
	var clients, wired, wireless, devices, online int
	wrong := false
	for _, r := range all {
		// A site's own sections, named by it only when there are others.
		title := func(key string) snapshot.Text {
			if len(all) > 1 {
				return snapshot.Text{Key: key + "_at", Args: map[string]any{"site": r.site.Name}}
			}
			return snapshot.Text{Key: key}
		}
		if rows := wans(r.health, now); len(rows) > 0 {
			sections = append(sections, snapshot.Section{Area: Area, Title: title("unifi.internet"), Rows: rows})
			wrong = wrong || slices.ContainsFunc(rows, func(r snapshot.Row) bool { return r.Tone == "red" })
		}

		// The devices, a row each, by name: how long each has been up and
		// how busy and full it is, or its state, in red or orange where it
		// went wrong.
		slices.SortFunc(r.devices, func(a, b device) int { return cmp.Compare(a.Name, b.Name) })
		rows := []snapshot.Row{}
		for _, d := range r.devices {
			row := snapshot.Row{
				Title:  snapshot.Text{Name: cmp.Or(d.Name, d.Model)},
				Detail: snapshot.Msg("unifi.device_detail", map[string]any{"model": d.Model, "firmware": d.Firmware}),
			}
			if s, ok := r.stats[d.ID]; ok {
				online++
				row.Value = snapshot.Msg("unifi.up_for", map[string]any{"ms": s.Uptime * 1000})
				row.Meters = []snapshot.Meter{
					{Label: snapshot.Msg("unifi.cpu_meter", nil), Used: s.CPU, Total: 100},
					{Label: snapshot.Msg("unifi.ram_meter", nil), Used: s.Mem, Total: 100},
				}
			} else if st, ok := states[d.State]; ok {
				row.Value, row.Tone = snapshot.Msg(st.key, nil), st.tone
			} else {
				row.Value, row.Tone = &snapshot.Text{Name: d.State}, "gray"
			}
			wrong = wrong || row.Tone == "red" || row.Tone == "orange"
			rows = append(rows, row)
			if d.Updatable {
				chores = append(chores, snapshot.Chore{Name: row.Title.Name, Note: d.Firmware + " →", URL: link + "/network/" + r.site.Ref + "/devices"})
			}
		}
		sections = append(sections, snapshot.Section{Area: Area, Title: title("unifi.devices"), Rows: rows})
		devices += len(r.devices)
		clients, wired, wireless = clients+r.clients, wired+r.wired, wireless+r.wireless
	}

	// The devices waiting to be adopted, and the application's update, each
	// done on the console's page for it.
	if len(all) > 0 {
		ref := all[0].site.Ref
		for _, p := range waiting {
			chores = append(chores, snapshot.Chore{Name: p.Model, Note: p.MAC, URL: link + "/network/" + ref + "/devices"})
		}
		if app != nil && len(app.Data) > 0 && app.Data[0].Update {
			chores = append(chores, snapshot.Chore{Name: "Network", Note: app.Data[0].Version + " →", URL: link + "/network/" + ref + "/settings/updates"})
		}
	}
	slices.SortStableFunc(chores, func(a, b snapshot.Chore) int { return cmp.Compare(a.Name, b.Name) })

	// Today: the clients connected, wired, wireless and the rest, which adds
	// up with another console's.
	f := snapshot.Figure{
		Area: Area, Icon: "network", Label: snapshot.Text{Key: "unifi.clients"}, Value: float64(clients),
		Detail: snapshot.Msg("unifi.devices_online", map[string]any{"online": online, "n": devices}),
		Adds:   true,
	}
	for _, s := range []struct {
		key string
		n   int
	}{{"unifi.wired", wired}, {"unifi.wireless", wireless}, {"unifi.vpn", clients - wired - wireless}} {
		if s.n > 0 {
			f.Shares = append(f.Shares, snapshot.Share{Label: snapshot.Text{Key: s.key}, Value: float64(s.n)})
		}
	}
	if wrong {
		f.Tone = "orange"
	}
	return snapshot.Parts{Sections: sections, Figures: []snapshot.Figure{f}, Chores: chores}
}

// wans are a row for each WAN in h, by its name, and one for the last speed
// test, if any. A WAN that's down is red, but grey while another is up if
// it has been down since the gateway started, as a spare port with nothing
// in it is.
func wans(h *health, now time.Time) []snapshot.Row {
	if h == nil {
		return nil
	}
	var rows []snapshot.Row
	var speed *snapshot.Row
	for _, line := range h.Data {
		switch line.Subsystem {
		case "wan":
			started, _ := strconv.ParseInt(line.Gateway.Uptime, 10, 64)
			up := slices.ContainsFunc(slices.Collect(maps.Values(line.WANs)), func(w wan) bool { return w.Downtime == nil })
			for _, name := range slices.Sorted(maps.Keys(line.WANs)) {
				w := line.WANs[name]
				row := snapshot.Row{Title: snapshot.Text{Name: name}}
				switch {
				case w.Downtime == nil:
					row.Value = snapshot.Msg("unifi.up_for", map[string]any{"ms": w.Uptime * 1000})
					if w.Availability != nil {
						args := map[string]any{"availability": math.Round(*w.Availability*10) / 10, "period": w.Period * 1000}
						key := "unifi.wan_availability"
						if w.Latency != nil {
							key, args["latency"] = "unifi.wan_detail", *w.Latency
						}
						row.Detail = snapshot.Msg(key, args)
					}
				case up && started > 0 && time.Duration(*w.Downtime)*time.Second >= time.Duration(started)*time.Second-boot:
					row.Value, row.Tone, row.Detail = snapshot.Msg("unifi.down", nil), "gray", snapshot.Msg("unifi.down_since_start", nil)
				default:
					since := now.Add(-time.Duration(*w.Downtime) * time.Second)
					row.Value, row.Tone, row.Detail = snapshot.Msg("unifi.down", nil), "red", snapshot.Msg("unifi.down_since", map[string]any{"at": since})
				}
				rows = append(rows, row)
			}
		case "www":
			if line.Speedtest > 0 {
				speed = &snapshot.Row{
					Title:  snapshot.Text{Key: "unifi.speed_test"},
					Value:  snapshot.Msg("unifi.speed", map[string]any{"down": math.Round(line.Down), "up": math.Round(line.Up)}),
					Detail: snapshot.Msg("unifi.speed_detail", map[string]any{"at": time.Unix(line.Speedtest, 0), "ping": line.Ping}),
				}
			}
		}
	}
	// The speed test after the WANs, whichever line comes first.
	if speed != nil {
		rows = append(rows, *speed)
	}
	return rows
}
