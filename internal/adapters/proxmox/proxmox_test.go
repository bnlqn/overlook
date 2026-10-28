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
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	sections := map[string]snapshot.Section{}
	for _, s := range p.Sections {
		sections[s.Title.Key] = s
	}
	if len(p.Sections) != 5 || len(sections) != 5 {
		t.Fatalf("a host, its guests, storage, pools and backups: %+v", p.Sections)
	}
	host := sections["proxmox.host"].Rows
	if v := host[0].Value.Args["n"]; v != 2.0 || host[0].Detail.Args["model"] != "i5-13500H" || host[0].Detail.Args["load"] != 0.64 {
		t.Errorf("the processor: %+v %+v", host[0].Value, host[0].Detail)
	}
	if m := host[1]; m.Value.Args["used"] != 24.0 || m.Value.Args["total"] != 31.0 || m.Meter.Tone != "blue" {
		t.Errorf("memory, in binary gigabytes, never red: %+v %+v", m.Value, m.Meter)
	}
	if v := host[3]; v.Value.Name != "9.2.21" || v.Detail.Name != "Linux 7.0.14-25-pve" {
		t.Errorf("the versions: %+v %+v", v.Value, v.Detail)
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
	}, []*host{h}, 2, time.Now())
	adaptertest.Validate(t, p)

	if p.Sections[0].Title.Name != "nova" || p.Sections[1].Rows[0].Title.Args["node"] != "nova" {
		t.Errorf("a host by its name, a guest by its node: %+v", p.Sections[:2])
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
	if _, err := Read(context.Background(), c); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("without its CA, Proxmox's certificate is refused: %v", err)
	}
	c.CA = ca
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c); err != nil {
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
