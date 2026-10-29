package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds the containers of a Docker 29 engine, recorded through
// linuxserver's socket proxy, with their names, hashes and addresses
// invented and three of them changed to show what goes wrong: bazarr exited
// with an error, recyclarr exited cleanly after its job, and gluetun fails
// its health check.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "docker.containers" {
		t.Fatalf("one section, the containers: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 9 || rows[0].Title.Name != "media-bazarr-1" || rows[8].Title.Name != "media-sonarr-1" {
		t.Fatalf("nine containers, by name: %+v", rows)
	}
	want := map[string]struct{ key, tone string }{
		"media-bazarr-1":       {"docker.exited", "red"},
		"media-gluetun-1":      {"docker.unhealthy", "red"},
		"media-jellyfin-1":     {"docker.starting", "blue"},
		"media-recyclarr-1":    {"docker.exited", "gray"},
		"media-socket-proxy-1": {"docker.running", ""},
	}
	for _, r := range rows {
		w, ok := want[r.Title.Name]
		if ok && (r.Value.Key != w.key || r.Tone != w.tone) {
			t.Errorf("%s: want %s in %q, got %+v", r.Title.Name, w.key, w.tone, r)
		}
	}
	if code := rows[0].Value.Args["code"]; code != 1 {
		t.Errorf("bazarr exited with 1: %v", code)
	}
	if d := rows[7].Detail.Name; d != "lscr.io/linuxserver/socket-proxy:3.4.7" {
		t.Errorf("the image, without its digest: %q", d)
	}

	f := p.Figures[0]
	if f.Value != 7 || f.Unit.Args["n"] != 9 || f.Detail.Args["n"] != 1 || f.Tone != "orange" || !f.Adds {
		t.Errorf("7 of 9 running, 1 unhealthy, adding up with another engine's: %+v", f)
	}
}

// A clean engine's tile isn't orange, and one that restarts or died is.
func TestTones(t *testing.T) {
	t.Parallel()
	p := parts([]container{
		{Names: []string{"/web"}, Image: "nginx", State: "running", Status: "Up 2 hours (healthy)"},
		{Names: []string{"/job"}, Image: "busybox", State: "exited", Status: "Exited (0) 3 hours ago"},
	})
	adaptertest.Validate(t, Area, p)
	if f := p.Figures[0]; f.Tone != "" || f.Value != 1 {
		t.Errorf("a job done and a healthy container: %+v", f)
	}
	p = parts([]container{
		{Names: []string{"/loop"}, Image: "busybox", State: "restarting", Status: "Restarting (1) 5 seconds ago"},
		{Names: []string{"/gone"}, Image: "busybox", State: "dead"},
	})
	adaptertest.Validate(t, Area, p)
	if r := p.Sections[0].Rows; r[0].Tone != "red" || r[1].Tone != "orange" || p.Figures[0].Tone != "orange" {
		t.Errorf("dead in red, restarting in orange: %+v", r)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://socket-proxy:2375"}
	if err := c.Check(); err != nil || c.Name != "Docker" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "unix:///var/run/docker.sock"}, "url needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
