package proxmox

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is the morning after testdata's last night of backups, which
// ends its week of runs.
var recorded = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

// testdata holds answers recorded from a Proxmox VE 9 host with its names
// and numbers invented: one node, nova, with eight guests and a template,
// and a week of backup runs to 28 October 2026, one with warnings and one
// failed.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "overlook@pve!overlook", Secret: "00000000-0000-0000-0000-000000000000"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	sections := map[string]snapshot.Section{}
	for _, s := range p.Sections {
		sections[s.Title.Key] = s
	}
	if len(p.Sections) != 5 || len(sections) != 5 {
		t.Fatalf("the hosts, the guests, storage, pools and backups: %+v", p.Sections)
	}
	hosts := sections[snapshot.HostsSection.Key].Rows
	if len(hosts) != 1 {
		t.Fatalf("one host: %+v", hosts)
	}
	host := hosts[0]
	if d := host.Detail.Args; host.Title.Name != "nova" || host.Value.Key != "proxmox.up_for" || d["model"] != "i5-13500H" || d["load"] != 0.64 || d["version"] != "9.2.21" {
		t.Errorf("the host, up, on what, with which Proxmox: %+v %+v", host.Value, host.Detail)
	}
	if m := host.Meters; len(m) != 2 || m[0].Used/m[0].Total < 0.015 || m[0].Used/m[0].Total > 0.025 || m[1].Tone != "blue" {
		t.Errorf("2%% of the processor, and memory, never red: %+v", m)
	}

	// The recorded week has its first two days without readings, so the
	// lines start where the readings do, and end with this hour, 10:00,
	// which has none yet and carries 9:00's: 117 hours.
	if c := p.Charts; len(c) != 2 || !c[0].Measures || c[0].Title.Key != snapshot.ProcessorChart.Key || c[1].Title.Key != snapshot.MemoryChart.Key {
		t.Fatalf("the processor and memory by the hour, as levels: %+v", c)
	}
	cpu, mem := p.Charts[0].Series, p.Charts[1].Series
	if len(cpu) != 1 || cpu[0].Label.Name != "nova" || len(cpu[0].Hours) != 117 || len(mem[0].Hours) != 117 {
		t.Fatalf("a line for nova, from its readings to this hour: %+v", cpu)
	}
	for _, last := range []int{115, 116} {
		if cpu[0].Hours[last] != 1.7 || mem[0].Hours[last] != 79.1 {
			t.Errorf("9:00's mean of its half hours, in percent, carried into 10:00: %v %v", cpu[0].Hours[last], mem[0].Hours[last])
		}
	}

	guests := sections["proxmox.guests"].Rows
	if len(guests) != 8 {
		t.Fatalf("eight guests, the template left out: %d", len(guests))
	}
	if g := guests[4]; g.Title.Name != "dns-standby" || g.Detail.Key != "proxmox.container" || g.Detail.Args["gb"] != 1.0 || g.Value.Key != "proxmox.up_for" {
		t.Errorf("a container, up: %+v", g)
	}
	if g := guests[7]; g.Title.Name != "sandbox" || g.Value.Key != "proxmox.stopped" || g.Tone != "gray" {
		t.Errorf("a VM, stopped: %+v", g)
	}

	storage := sections["proxmox.storage"].Rows
	if len(storage) != 4 || storage[3].Title.Name != "backups" || storage[3].Meter == nil {
		t.Errorf("every storage, with how full: %+v", storage)
	}
	pools := sections["proxmox.pools"].Rows
	if pools[0].Detail.Key != "proxmox.healthy" || pools[1].Value.Name != "DEGRADED" || pools[1].Tone != "red" || pools[1].Detail.Key != "proxmox.bytes_of" {
		t.Errorf("rpool healthy, tank degraded: %+v", pools)
	}
	if run := sections["proxmox.backups"].Rows; len(run) != 1 || run[0].Value.Key != "proxmox.done" || run[0].Detail.Args["ms"] != int64(499_000) {
		t.Errorf("the last run took 8 min 19 s: %+v", run)
	}

	if len(p.Events) != 9 {
		t.Fatalf("each run is an event: %d", len(p.Events))
	}
	tones := map[string]string{}
	for _, e := range p.Events {
		tones[e.Text.Key] = e.Tone
	}
	if tones["proxmox.backup_warnings"] != "orange" || tones["proxmox.backup_failed"] != "red" || p.Events[0].At != time.Date(2026, 10, 28, 0, 30, 0, 0, time.UTC) {
		t.Errorf("the runs, newest first, in their tones: %+v", p.Events)
	}

	if f := p.Figures[0]; f.Value != 7 || f.Unit.Args["n"] != 8 || f.Tone != "orange" || !f.Adds || f.Detail.Args["online"] != 1 || f.Detail.Args["n"] != 1 {
		t.Errorf("7 of 8 guests up, on 1 of 1 host, adding up with another Proxmox's: %+v", f)
	}
	if m := p.Figures[0].Meters; len(m) != 2 || m[0].Total != 16 || m[1].Tone != "blue" {
		t.Errorf("the host's processor and memory under the guests: %+v", m)
	}
}

// In a cluster, what belongs to one node says which, and an offline node
// is counted but not read.
func TestCluster(t *testing.T) {
	t.Parallel()
	h := &host{name: "nova", runs: []task{{Start: 1_793_150_000}}}
	h.status.Memory.Total, h.status.PVEVersion, h.status.KVersion = 100, "pve-manager/9.2.21/0c4f1e2a", "Linux 7.0.14-25-pve"
	h.pools = append(h.pools, struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Alloc  int64  `json:"alloc"`
		Health string `json:"health"`
	}{"rpool", 100, 10, "ONLINE"})
	p := parts([]resource{
		{Type: "node", Node: "nova", Status: "online"},
		{Type: "node", Node: "vega", Status: "offline"},
		{Type: "qemu", Node: "nova", VMID: 100, Name: "house", Status: "running"},
		{Type: "storage", Node: "nova", Storage: "local", Status: "available", MaxDisk: 100},
		{Type: "storage", Node: "nova", Storage: "backups", Shared: 1, Status: "available", MaxDisk: 100},
		{Type: "storage", Node: "vega", Storage: "backups", Shared: 1, Status: "unknown"},
	}, []*host{h}, 2, recorded)
	adaptertest.Validate(t, p)

	if h := p.Sections[0].Rows; len(h) != 2 || h[0].Title.Name != "nova" || h[1].Title.Name != "vega" || h[1].Value.Key != "proxmox.offline" || h[1].Tone != "red" {
		t.Errorf("each host by its name, the one offline in red: %+v", h)
	}
	if p.Sections[1].Rows[0].Title.Args["node"] != "nova" {
		t.Errorf("a guest by its node: %+v", p.Sections[1])
	}
	if s := p.Sections[2].Rows; len(s) != 2 || s[0].Title.Key != "proxmox.on_node" || s[1].Title.Name != "backups" {
		t.Errorf("a shared storage once, without its node: %+v", s)
	}
	if run := p.Sections[4].Rows[0]; run.Title.Name != "nova" || run.Value.Key != "proxmox.running" {
		t.Errorf("a run still going: %+v", run)
	}
	if f := p.Figures[0]; f.Detail.Key != "proxmox.hosts_online" || f.Detail.Args["online"] != 1 {
		t.Errorf("1 of 2 hosts online: %+v", f.Detail)
	}
}

// Proxmox signs its own certificate: Overlook reads it trusting its CA, and
// sends its token as Proxmox expects.
func TestCA(t *testing.T) {
	t.Parallel()
	var auth string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	t.Cleanup(s.Close)
	ca := filepath.Join(t.TempDir(), "pve-root-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Config{URL: s.URL, Token: "overlook@pve!overlook", Secret: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("without its CA, Proxmox's certificate is refused: %v", err)
	}
	c.CA = ca
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err != nil {
		t.Fatal(err)
	}
	if auth != "PVEAPIToken=overlook@pve!overlook=s3cret" {
		t.Errorf("the token's header: %q", auth)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "https://pve.example.com:8006", Token: "overlook@pve!overlook", Secret: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "Proxmox" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "pve.example.com"}, "url needs"},
		{Config{URL: "https://pve.example.com", Token: "overlook@pve"}, "token needs"},
		{Config{URL: "https://pve.example.com", Token: "overlook@pve!overlook"}, "secret is missing"},
		{Config{URL: "https://pve.example.com", Token: "overlook@pve!overlook", Secret: "s", CA: "no-such.pem"}, "ca: open no-such.pem"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
