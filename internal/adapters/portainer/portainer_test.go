package portainer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds answers recorded from a scratch Portainer 2.45.3 on 7
// November 2026 (compose.yaml, fill.sh and record.sh beside them): media, a
// Docker engine running five of its seven containers, one unhealthy, in a
// stack and on their own, and backups, an engine that stopped after
// Portainer last saw it running two.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "ptr_example"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	for _, r := range src.Sent() {
		if r.Header.Get("X-Api-Key") != "ptr_example" {
			t.Errorf("%s: read without the key", r.URL)
		}
	}

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "portainer.environments" {
		t.Fatalf("one section, the environments: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("two environments and the version: %+v", rows)
	}
	backups, media, version := rows[0], rows[1], rows[2]
	if backups.Title.Name != "backups" || backups.Value.Key != "portainer.down" || backups.Tone != "red" ||
		!backups.Detail.Args["at"].(time.Time).Equal(time.Date(2026, 11, 7, 23, 13, 50, 0, time.UTC)) {
		t.Errorf("backups is down, last seen when Portainer last looked: %+v", backups)
	}
	if media.Title.Name != "media" || media.Value.Args["running"] != 5 || media.Value.Args["n"] != 7 ||
		media.Tone != "orange" || media.Detail.Key != "portainer.unhealthy_stacks" ||
		media.Detail.Args["unhealthy"] != 1 || media.Detail.Args["n"] != 1 {
		t.Errorf("media runs 5 of 7, 1 unhealthy, in orange, with 1 stack: %+v", media)
	}
	if version.Value.Name != "2.45.3" || version.Tone != "" {
		t.Errorf("Portainer's version, the newest: %+v", version)
	}

	if len(p.Figures) != 1 {
		t.Fatalf("one figure, the containers: %+v", p.Figures)
	}
	f := p.Figures[0]
	if f.Label.Key != "docker.containers" || !f.Adds || f.Value != 5 || f.Unit.Args["n"] != 7 ||
		f.Detail.Args["n"] != 1 || f.Tone != "orange" {
		t.Errorf("5 of 7 running, backups' not counted, adding up with Docker's: %+v", f)
	}
}

// A newer Portainer is out.
func TestReadNewer(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata",
		`"UpdateAvailable": false`, `"UpdateAvailable": true`,
		`"LatestVersion": ""`, `"LatestVersion": "2.46.0"`)
	c := Config{URL: src.URL, Key: "ptr_example"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	rows := p.Sections[0].Rows
	if v := rows[len(rows)-1]; v.Value.Key != "version.newer" || v.Value.Args["version"] != "2.46.0" ||
		v.Tone != "orange" || v.Detail.Name != "2.45.3" {
		t.Errorf("2.46.0 is out, in orange, over 2.45.3: %+v", v)
	}
}

// An Edge agent is up while it calls Portainer, whatever its status says; a
// Kubernetes environment counts no containers; and a clean one isn't orange.
func TestParts(t *testing.T) {
	t.Parallel()
	edge := environment{Name: "garage", Type: 4, Status: 1, Snapshots: []look{{Time: 1794093000, Containers: 3, Running: 3, Stacks: 2}}}
	k8s := environment{Name: "cluster", Type: 6, Status: 2}

	p := parts([]environment{edge, k8s}, version{Version: "2.45.3"})
	adaptertest.Validate(t, Adapter, p)
	rows := p.Sections[0].Rows
	if rows[1].Title.Name != "garage" || rows[1].Value.Key != "portainer.down" {
		t.Errorf("an Edge agent that stopped calling is down: %+v", rows[1])
	}
	if rows[0].Title.Name != "cluster" || rows[0].Value.Key != "portainer.down" || rows[0].Detail != nil {
		t.Errorf("a Kubernetes environment down, never seen up: %+v", rows[0])
	}
	if f := p.Figures[0]; f.Value != 0 || f.Unit.Args["n"] != 0 || f.Tone != "orange" {
		t.Errorf("the Edge agent's containers don't count while it's down: %+v", f)
	}

	edge.Heartbeat = true
	k8s.Status = 1
	p = parts([]environment{edge, k8s}, version{Version: "2.45.3"})
	adaptertest.Validate(t, Adapter, p)
	rows = p.Sections[0].Rows
	if rows[1].Value.Args["running"] != 3 || rows[1].Detail.Key != "portainer.stacks" || rows[1].Tone != "" {
		t.Errorf("an Edge agent that calls is up, 3 running, 2 stacks: %+v", rows[1])
	}
	if rows[0].Value.Key != "portainer.up" {
		t.Errorf("a Kubernetes environment up: %+v", rows[0])
	}
	if f := p.Figures[0]; f.Value != 3 || f.Tone != "" {
		t.Errorf("3 running, none wrong: %+v", f)
	}

	if p := parts([]environment{k8s}, version{}); len(p.Figures) != 0 || len(p.Sections[0].Rows) != 1 {
		t.Errorf("no Docker environment, no containers figure: %+v", p)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "https://portainer:9443", Key: "ptr_example"}
	if err := c.Check(); err != nil || c.Name != "Portainer" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "ptr_example"}, "url is missing"},
		{Config{URL: "https://portainer:9443"}, "key is missing"},
		{Config{URL: "https://portainer:9443", Key: "ptr_example", ServerName: "localhost"}, "server_name needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{URL: url, Key: "ptr_example"}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, time.Time{})
	})
}
