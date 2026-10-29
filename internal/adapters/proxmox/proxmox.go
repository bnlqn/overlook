// Package proxmox reads Proxmox VE: each host's processor, memory and
// versions, the guests, the storage and ZFS pools, and the backup runs.
// Everything it gives goes in Servers.
package proxmox

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

const area = "servers"

// week is how far back the backup runs are read, for the page's week.
const week = 7 * 24 * time.Hour

// Config is one Proxmox VE host or cluster, under sources.proxmox in
// overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Proxmox, such as https://pve.example.com:8006.
	URL string `yaml:"url"`
	// Token is an API token's ID, as user@realm!name, and Secret its secret.
	Token  string `yaml:"token"`
	Secret string `yaml:"secret"`
	// CA is a PEM file of the CA that signed Proxmox's certificate, such as
	// the host's /etc/pve/pve-root-ca.pem, when the system doesn't trust it.
	CA string `yaml:"ca"`

	client fetch.Client
}

var tokenID = regexp.MustCompile(`^[^@!\s]+@[^@!\s]+![^@!\s]+$`)

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if !tokenID.MatchString(c.Token) {
		return errors.New("token needs to be an API token's ID, such as overlook@pve!overlook")
	}
	if c.Secret == "" {
		return errors.New("secret is missing")
	}
	if c.Name == "" {
		c.Name = "Proxmox"
	}
	if c.CA != "" {
		client, err := fetch.Trusting(c.CA, "")
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		c.client = client
	}
	return nil
}

// resource is one line of /cluster/resources: a node, a guest or a storage.
type resource struct {
	Type     string  `json:"type"`
	Node     string  `json:"node"`
	Status   string  `json:"status"`
	Uptime   int64   `json:"uptime"`
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Template int     `json:"template"`
	MaxCPU   float64 `json:"maxcpu"`
	MaxMem   int64   `json:"maxmem"`
	Storage  string  `json:"storage"`
	Shared   int     `json:"shared"`
	Disk     int64   `json:"disk"`
	MaxDisk  int64   `json:"maxdisk"`
}

// host is what Proxmox says of one node, from /nodes/{node}/...
type host struct {
	name   string
	status struct {
		CPU     float64 `json:"cpu"`
		CPUInfo struct {
			Model string `json:"model"`
			CPUs  int    `json:"cpus"`
		} `json:"cpuinfo"`
		LoadAvg []string `json:"loadavg"`
		Memory  struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		} `json:"memory"`
		Uptime     int64  `json:"uptime"`
		PVEVersion string `json:"pveversion"`
		KVersion   string `json:"kversion"`
	}
	pools []struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Alloc  int64  `json:"alloc"`
		Health string `json:"health"`
	}
	runs []task
}

// task is one backup run, from /nodes/{node}/tasks. A run still going has
// no end and no status.
type task struct {
	Start  int64  `json:"starttime"`
	End    int64  `json:"endtime"`
	Status string `json:"status"`
}

// Read reads the Proxmox at c: its nodes, then each online node's status,
// pools and the week's backup runs.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	var resources []resource
	if err := get(ctx, c, "cluster/resources", &resources); err != nil {
		return snapshot.Parts{}, err
	}
	var hosts []*host
	nodes := 0
	for _, r := range resources {
		if r.Type == "node" {
			nodes++
			if r.Status == "online" {
				hosts = append(hosts, &host{name: r.Node})
			}
		}
	}
	slices.SortFunc(hosts, func(a, b *host) int { return strings.Compare(a.name, b.name) })

	since := time.Now().Add(-week).Unix()
	errs := make([]error, 3*len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		node := "nodes/" + url.PathEscape(h.name) + "/"
		wg.Go(func() { errs[3*i] = get(ctx, c, node+"status", &h.status) })
		wg.Go(func() { errs[3*i+1] = get(ctx, c, node+"disks/zfs", &h.pools) })
		// source=all lists the run still going too.
		wg.Go(func() {
			errs[3*i+2] = get(ctx, c, fmt.Sprintf("%stasks?typefilter=vzdump&source=all&since=%d&limit=100", node, since), &h.runs)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return snapshot.Parts{}, err
		}
	}
	return parts(resources, hosts, nodes), nil
}

// get reads the answer at path into v: Proxmox wraps each in {"data": …}.
func get(ctx context.Context, c Config, path string, v any) error {
	header := http.Header{"Authorization": {"PVEAPIToken=" + c.Token + "=" + c.Secret}}
	return c.client.JSON(ctx, strings.TrimSuffix(c.URL, "/")+"/api2/json/"+path, header, &struct {
		Data any `json:"data"`
	}{v})
}

func parts(resources []resource, hosts []*host, nodes int) snapshot.Parts {
	// In a cluster, what belongs to one node says which.
	cluster := nodes > 1
	onNode := func(name, node string) snapshot.Text {
		if cluster {
			return snapshot.Text{Key: "proxmox.on_node", Args: map[string]any{"name": name, "node": node}}
		}
		return snapshot.Text{Name: name}
	}
	var p snapshot.Parts

	// The hosts, a row each, by name: how long each has been up, or that
	// it's offline, what it runs on, and how busy and how full it is.
	online := map[string]*host{}
	for _, h := range hosts {
		online[h.name] = h
	}
	var names []string
	for _, r := range resources {
		if r.Type == "node" {
			names = append(names, r.Node)
		}
	}
	slices.Sort(names)
	hostRows := []snapshot.Row{}
	for _, name := range names {
		h := online[name]
		if h == nil {
			hostRows = append(hostRows, snapshot.Row{Title: snapshot.Text{Name: name}, Value: snapshot.Msg("proxmox.offline", nil), Tone: "red"})
			continue
		}
		s := h.status
		var load float64
		if len(s.LoadAvg) > 0 {
			load, _ = strconv.ParseFloat(s.LoadAvg[0], 64)
		}
		// "pve-manager/9.2.21/…".
		_, version, _ := strings.Cut(s.PVEVersion, "/")
		version, _, _ = strings.Cut(version, "/")
		row := snapshot.Row{
			Title:  snapshot.Text{Name: name},
			Value:  snapshot.Msg("proxmox.up_for", map[string]any{"ms": s.Uptime * 1000}),
			Detail: snapshot.Msg("proxmox.host_detail", map[string]any{"model": model(s.CPUInfo.Model), "n": s.CPUInfo.CPUs, "load": load, "version": version}),
		}
		if s.CPUInfo.CPUs > 0 && s.Memory.Total > 0 {
			row.Meters = []snapshot.Meter{
				{Label: snapshot.Msg("proxmox.cpu_meter", nil), Used: s.CPU * float64(s.CPUInfo.CPUs), Total: float64(s.CPUInfo.CPUs)},
				// Mostly the guests', which keep what they're given: full is
				// normal, so the bar never turns red.
				{Label: snapshot.Msg("proxmox.ram_meter", nil), Used: float64(s.Memory.Used), Total: float64(s.Memory.Total), Tone: "blue"},
			}
		}
		hostRows = append(hostRows, row)
	}
	p.Sections = append(p.Sections, snapshot.Section{Area: area, Title: snapshot.Text{Key: "proxmox.hosts"}, Rows: hostRows})

	// The guests, by their IDs, leaving out templates, which never run.
	var guests []resource
	for _, r := range resources {
		if (r.Type == "qemu" || r.Type == "lxc") && r.Template == 0 {
			guests = append(guests, r)
		}
	}
	slices.SortFunc(guests, func(a, b resource) int { return cmp.Compare(a.VMID, b.VMID) })
	rows, running := []snapshot.Row{}, 0
	for _, g := range guests {
		kind := "proxmox.vm"
		if g.Type == "lxc" {
			kind = "proxmox.container"
		}
		row := snapshot.Row{
			Title:  onNode(g.Name, g.Node),
			Detail: snapshot.Msg(kind, map[string]any{"id": g.VMID, "n": g.MaxCPU, "gb": gb(g.MaxMem)}),
		}
		switch g.Status {
		case "running":
			running++
			row.Value = snapshot.Msg("proxmox.up_for", map[string]any{"ms": g.Uptime * 1000})
		case "stopped":
			row.Value, row.Tone = snapshot.Msg("proxmox.stopped", nil), "gray"
		default:
			row.Value, row.Tone = &snapshot.Text{Name: g.Status}, "gray"
		}
		rows = append(rows, row)
	}
	if len(guests) > 0 {
		p.Sections = append(p.Sections, snapshot.Section{Area: area, Title: snapshot.Text{Key: "proxmox.guests"}, Rows: rows})
	}

	// The storage, a shared one once.
	rows = []snapshot.Row{}
	shared := map[string]bool{}
	for _, r := range resources {
		if r.Type != "storage" || shared[r.Storage] {
			continue
		}
		row := snapshot.Row{Title: onNode(r.Storage, r.Node)}
		if r.Shared == 1 {
			shared[r.Storage] = true
			row.Title = snapshot.Text{Name: r.Storage}
		}
		if r.Status == "available" && r.MaxDisk > 0 {
			row.Value = snapshot.Msg("proxmox.bytes_of", map[string]any{"used": r.Disk, "total": r.MaxDisk})
			row.Meter = &snapshot.Meter{Used: float64(r.Disk), Total: float64(r.MaxDisk)}
		} else {
			row.Value, row.Tone = &snapshot.Text{Name: r.Status}, "gray"
		}
		rows = append(rows, row)
	}
	if len(rows) > 0 {
		p.Sections = append(p.Sections, snapshot.Section{Area: area, Title: snapshot.Text{Key: "proxmox.storage"}, Rows: rows})
	}

	// The ZFS pools: how full each is, and its health.
	rows = []snapshot.Row{}
	for _, h := range hosts {
		for _, z := range h.pools {
			space := snapshot.Text{Key: "proxmox.bytes_of", Args: map[string]any{"used": z.Alloc, "total": z.Size}}
			row := snapshot.Row{Title: onNode(z.Name, h.name), Value: &space, Detail: snapshot.Msg("proxmox.healthy", nil)}
			if z.Health != "ONLINE" {
				row.Value, row.Tone, row.Detail = &snapshot.Text{Name: z.Health}, "red", &space
			}
			if z.Size > 0 {
				row.Meter = &snapshot.Meter{Used: float64(z.Alloc), Total: float64(z.Size)}
			}
			rows = append(rows, row)
		}
	}
	if len(rows) > 0 {
		p.Sections = append(p.Sections, snapshot.Section{Area: area, Title: snapshot.Text{Key: "proxmox.pools"}, Rows: rows})
	}

	// The backup runs: each node's newest in Backups, and the week's as
	// events.
	rows = []snapshot.Row{}
	for _, h := range hosts {
		slices.SortFunc(h.runs, func(a, b task) int { return cmp.Compare(b.Start, a.Start) })
		for i, t := range h.runs {
			start := time.Unix(t.Start, 0).UTC()
			took := map[string]any{"at": start, "ms": (t.End - t.Start) * 1000, "status": t.Status}
			e := snapshot.Event{At: start, Area: area, Icon: "archive"}
			row := snapshot.Row{Title: snapshot.Text{Key: "proxmox.last_run"}, Detail: snapshot.Msg("proxmox.run_took", took)}
			if cluster {
				row.Title = snapshot.Text{Name: h.name}
			}
			switch {
			case t.End == 0:
				e.Text = snapshot.Text{Key: "proxmox.backup_running"}
				row.Value, row.Tone, row.Detail = snapshot.Msg("proxmox.running", nil), "blue", snapshot.Msg("proxmox.run_since", took)
			case t.Status == "OK":
				e.Text = snapshot.Text{Key: "proxmox.backup_done", Args: took}
				row.Value, row.Tone = snapshot.Msg("proxmox.done", nil), "green"
			case strings.HasPrefix(t.Status, "WARNINGS"):
				e.Text, e.Tone = snapshot.Text{Key: "proxmox.backup_warnings", Args: took}, "orange"
				row.Value, row.Tone, row.Detail = snapshot.Msg("proxmox.warnings", nil), "orange", snapshot.Msg("proxmox.run_why", took)
			default:
				e.Text, e.Tone = snapshot.Text{Key: "proxmox.backup_failed", Args: took}, "red"
				row.Value, row.Tone, row.Detail = snapshot.Msg("proxmox.failed", nil), "red", snapshot.Msg("proxmox.run_why", took)
			}
			p.Events = append(p.Events, e)
			if i == 0 {
				rows = append(rows, row)
			}
		}
	}
	if len(rows) > 0 {
		p.Sections = append(p.Sections, snapshot.Section{Area: area, Title: snapshot.Text{Key: "proxmox.backups"}, Rows: rows})
	}

	// Today: the guests up, the hosts online, and how busy and how full the
	// hosts are. It adds up with another Proxmox's, guests on two hosts being
	// different guests, so its numbers are all counts.
	var cpu, threads, used, total float64
	for _, h := range hosts {
		cpu += h.status.CPU * float64(h.status.CPUInfo.CPUs)
		threads += float64(h.status.CPUInfo.CPUs)
		used += float64(h.status.Memory.Used)
		total += float64(h.status.Memory.Total)
	}
	f := snapshot.Figure{
		Area: area, Icon: "stack", Label: snapshot.Text{Key: "proxmox.guests"}, Value: float64(running),
		Unit:   snapshot.Msg("proxmox.of_up", map[string]any{"n": len(guests)}),
		Detail: snapshot.Msg("proxmox.hosts_online", map[string]any{"online": len(hosts), "n": nodes}),
		Adds:   true,
	}
	if threads > 0 && total > 0 {
		f.Meters = []snapshot.Meter{
			{Label: snapshot.Msg("proxmox.cpu_meter", nil), Used: cpu, Total: threads},
			{Label: snapshot.Msg("proxmox.ram_meter", nil), Used: used, Total: total, Tone: "blue"},
		}
	}
	if running < len(guests) {
		f.Tone = "orange"
	}
	p.Figures = append(p.Figures, f)
	return p
}

// cpuBrand is what goes before a processor's model, such as "12th Gen
// Intel(R) Core(TM) ".
var cpuBrand = regexp.MustCompile(`^.*\(TM\)\s*`)

// model is a processor's model without its brand: "i5-12600H".
func model(s string) string { return cpuBrand.ReplaceAllString(s, "") }

// gb is bytes of memory in the binary gigabytes it's sold in: an "8 GB"
// guest, to a tenth below 10.
func gb(bytes int64) float64 {
	g := float64(bytes) / (1 << 30)
	if g >= 10 {
		return math.Round(g)
	}
	return math.Round(10*g) / 10
}
