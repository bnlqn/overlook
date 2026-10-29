// Package beszel reads Beszel: each system its agents report on, as a
// host, whether it's up, how busy its processor and how full its memory and
// disk are, and its processor and memory by the hour over the week.
// Everything it gives goes in Servers.
package beszel

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
const Area = "servers"

// Config is one Beszel hub, under sources.beszel in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads the hub, such as http://beszel:8090.
	URL string `yaml:"url"`
	// User and Password are a Beszel user's email and password: one with
	// the role readonly, given the systems to show. Beszel lets Overlook in
	// only with a login, one POST (ADR 0009).
	User     string `yaml:"user"`
	Password string `yaml:"password"`

	session *fetch.Session
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.User == "" || c.Password == "" {
		return errors.New("user and password are missing: Beszel lets Overlook in only with a login")
	}
	c.Name = cmp.Or(c.Name, "Beszel")
	s, err := fetch.Client{}.Session(c.api("collections/users/auth-with-password"), map[string]string{"identity": c.User, "password": c.Password}, token)
	if err != nil {
		return err
	}
	c.session = s
	return nil
}

func (c Config) api(path string) string { return strings.TrimSuffix(c.URL, "/") + "/api/" + path }

// token takes the session from a login's answer: the token Beszel gives,
// sent back as it is.
func token(_ http.Header, body []byte) (http.Header, error) {
	var a struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.Token == "" {
		return nil, errors.New("no token in the answer")
	}
	return http.Header{"Authorization": {a.Token}}, nil
}

// system is one of Beszel's systems, with what its agent last said of it.
type system struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Status is up, down, paused or pending.
	Status  string `json:"status"`
	Updated string `json:"updated"`
	Info    struct {
		CPU     float64   `json:"cpu"`
		Mem     float64   `json:"mp"`
		Disk    float64   `json:"dp"`
		Threads int       `json:"t"`
		Uptime  int64     `json:"u"`
		Load    []float64 `json:"la"`
	} `json:"info"`
}

// record is a system's means over two hours, up to when it was made.
type record struct {
	System  string `json:"system"`
	Created string `json:"created"`
	Stats   struct {
		CPU float64 `json:"cpu"`
		Mem float64 `json:"mp"`
		// GB is how much memory the system has, in binary gigabytes.
		GB float64 `json:"m"`
	} `json:"stats"`
}

// pbTime is how Beszel writes a time, in UTC.
const pbTime = "2006-01-02 15:04:05.999Z07:00"

// Read reads the hub at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	// A lapsed session doesn't fail a list: Beszel answers it as a guest's,
	// with nothing in it. This read fails one with a 401 instead, so the
	// session logs in again before the lists.
	if err := c.session.JSON(ctx, c.api("beszel/getkey"), &struct{}{}); err != nil {
		return snapshot.Parts{}, err
	}
	systems, err := list[system](ctx, c, "systems", url.Values{"fields": {"id,name,status,info,updated"}})
	if err != nil {
		return snapshot.Parts{}, err
	}
	since := now.UTC().AddDate(0, 0, -7).Format(time.DateTime)
	week, err := list[record](ctx, c, "system_stats", url.Values{
		"filter": {fmt.Sprintf("type='120m' && created>='%s'", since)},
		"sort":   {"created"},
		"fields": {"system,created,stats.cpu,stats.mp,stats.m"},
	})
	if err != nil {
		return snapshot.Parts{}, err
	}
	return parts(systems, week, now), nil
}

// perPage is the most records Beszel gives at once.
const perPage = 1000

// list reads every record of the collection asked for, a page at a time.
func list[T any](ctx context.Context, c Config, collection string, q url.Values) ([]T, error) {
	q.Set("perPage", strconv.Itoa(perPage))
	q.Set("skipTotal", "1")
	var all []T
	for page := 1; ; page++ {
		q.Set("page", strconv.Itoa(page))
		var p struct {
			Items []T `json:"items"`
		}
		if err := c.session.JSON(ctx, c.api("collections/"+collection+"/records?"+q.Encode()), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if len(p.Items) < perPage {
			return all, nil
		}
	}
}

func parts(systems []system, week []record, now time.Time) snapshot.Parts {
	slices.SortFunc(systems, func(a, b system) int { return cmp.Compare(a.Name, b.Name) })

	// The hosts, a row each, by name: how long each has been up and how
	// busy and full it is, or since when it's down.
	rows := []snapshot.Row{}
	up, threads := 0, 0
	var cpu float64
	for _, s := range systems {
		row := snapshot.Row{Title: snapshot.Text{Name: s.Name}}
		switch s.Status {
		case "up":
			up++
			i := s.Info
			load := 0.0
			if len(i.Load) > 0 {
				load = i.Load[0]
			}
			row.Value = snapshot.Msg("beszel.up_for", map[string]any{"ms": i.Uptime * 1000})
			row.Detail = snapshot.Msg("beszel.host_detail", map[string]any{"n": i.Threads, "load": load})
			row.Meters = []snapshot.Meter{
				{Label: snapshot.Msg("beszel.cpu_meter", nil), Used: i.CPU, Total: 100},
				// Beszel's memory leaves out the page cache, so a full one
				// is short of memory and warns, where a Proxmox host's
				// counts what its guests keep and never does.
				{Label: snapshot.Msg("beszel.ram_meter", nil), Used: i.Mem, Total: 100},
				{Label: snapshot.Msg("beszel.disk_meter", nil), Used: i.Disk, Total: 100},
			}
			cpu += i.CPU / 100 * float64(i.Threads)
			threads += i.Threads
		case "down":
			row.Value, row.Tone = snapshot.Msg("beszel.down", nil), "red"
			if at, err := time.Parse(pbTime, s.Updated); err == nil {
				row.Detail = snapshot.Msg("beszel.down_since", map[string]any{"at": at})
			}
		case "paused":
			row.Value, row.Tone = snapshot.Msg("beszel.paused", nil), "gray"
		case "pending":
			row.Value, row.Tone = snapshot.Msg("beszel.pending", nil), "gray"
		default:
			row.Value, row.Tone = &snapshot.Text{Name: s.Status}, "gray"
		}
		rows = append(rows, row)
	}

	// The week, a reading for each of the two hours a record is the mean of,
	// and the agents' last, so that the lines end with this hour.
	names := make([]string, 0, len(systems))
	readings := make([][]snapshot.Reading, 0, len(systems))
	var used, total float64
	for _, s := range systems {
		if s.Status != "up" {
			continue
		}
		var r []snapshot.Reading
		gb := 0.0
		for _, w := range week {
			at, err := time.Parse(pbTime, w.Created)
			if w.System != s.ID || err != nil {
				continue
			}
			r = append(r, snapshot.Reading{At: at.Add(-time.Hour), CPU: w.Stats.CPU, Mem: w.Stats.Mem},
				snapshot.Reading{At: at, CPU: w.Stats.CPU, Mem: w.Stats.Mem})
			gb = w.Stats.GB
		}
		if at, err := time.Parse(pbTime, s.Updated); err == nil {
			r = append(r, snapshot.Reading{At: at, CPU: s.Info.CPU, Mem: s.Info.Mem})
		}
		names, readings = append(names, s.Name), append(readings, r)
		used += s.Info.Mem / 100 * gb
		total += gb
	}

	// Today: the hosts up, and how busy and how full they are, which adds up
	// with another hub's.
	f := snapshot.Figure{
		Area: Area, Icon: "server", Label: snapshot.Text{Key: "beszel.hosts"}, Value: float64(up),
		Unit: snapshot.Msg("beszel.of_up", map[string]any{"n": len(systems)}),
		Adds: true,
	}
	if threads > 0 && total > 0 {
		f.Meters = []snapshot.Meter{
			{Label: snapshot.Msg("beszel.cpu_meter", nil), Used: cpu, Total: float64(threads)},
			{Label: snapshot.Msg("beszel.ram_meter", nil), Used: used, Total: total},
		}
	}
	if up < len(systems) {
		f.Tone = "orange"
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.HostsSection, Rows: rows}},
		Charts:   snapshot.HostCharts(Area, now, names, readings),
		Figures:  []snapshot.Figure{f},
	}
}
