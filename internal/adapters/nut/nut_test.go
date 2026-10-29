package nut

import (
	"bufio"
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// replay starts a upsd that replays the transcript at path to each
// connection: its "> " lines are what Overlook must send, in order, and the
// "< " lines after each are upsd's answer. Anything else Overlook sends fails
// tb.
func replay(tb testing.TB, path string) string {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	type turn struct{ ask, answer string }
	var turns []turn
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if ask, ok := strings.CutPrefix(line, "> "); ok {
			turns = append(turns, turn{ask: ask + "\n"})
		} else if answer, ok := strings.CutPrefix(line, "< "); ok && len(turns) > 0 {
			turns[len(turns)-1].answer += answer + "\n"
		} else {
			tb.Fatalf("%s: %q is neither > nor <", path, line)
		}
	}
	ln, err := (&net.ListenConfig{}).Listen(tb.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for _, tn := range turns {
					ask, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if ask != tn.ask {
						tb.Errorf("upsd was sent %q, not %q", ask, tn.ask)
						return
					}
					_, _ = conn.Write([]byte(tn.answer))
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// read reads the UPS named ups from the transcript at path: its section's
// rows by title, and its figure.
func read(t *testing.T, path string) (map[string]snapshot.Row, snapshot.Figure) {
	t.Helper()
	c := Config{Host: replay(t, path), UPS: "ups"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	if len(p.Sections) != 1 || len(p.Figures) != 1 {
		t.Fatalf("want one section and one figure: %+v", p)
	}
	s, f := p.Sections[0], p.Figures[0]
	if s.Area != "servers" || f.Area != "servers" || f.Icon != "plug" || len(f.Meters) != 1 {
		t.Errorf("the UPS is a section and a figure in servers, with its battery: %+v, %+v", s, f)
	}
	rows := map[string]snapshot.Row{}
	for _, r := range s.Rows {
		rows[r.Title.Key] = r
	}
	return rows, f
}

// testdata holds what upsd said of an Eaton UPS through the exporter Overlook
// replaces, as upsd's own transcript, with its serial and every reading
// invented; on-battery.txt is the same UPS twenty minutes into a power cut.
func TestRead(t *testing.T) {
	t.Parallel()
	rows, f := read(t, "testdata/on-mains.txt")
	if r := rows["nut.power"]; r.Value.Key != "nut.on_mains" || r.Tone != "green" || r.Detail.Args["model"] != "Ellipse PRO 650" || r.Detail.Args["v"] != 231.0 {
		t.Errorf("on mains, its model trimmed, fed 231 V: %+v", r)
	}
	if r := rows["nut.battery"]; r.Value.Args["n"] != 100.0 || r.Meter.Tone != "green" || r.Detail.Key != "nut.runtime_low" || r.Detail.Args["ms"] != 1380e3 || r.Detail.Args["low"] != 40.0 {
		t.Errorf("full, 23 minutes at this load, low at 40%%: %+v", r)
	}
	if r := rows["nut.drawing"]; r.Value.Args["n"] != 96.0 || r.Detail.Args["n"] != 27.0 {
		t.Errorf("drawing 96 W, 27%% of what it can carry: %+v", r)
	}
	if f.Value != 96 || f.Detail.Key != "nut.on_mains" || f.Tone != "" || f.Meters[0].Used != 100 {
		t.Errorf("the figure is the watts, on mains, with a full battery: %+v", f)
	}

	rows, f = read(t, "testdata/on-battery.txt")
	if r := rows["nut.power"]; r.Value.Key != "nut.on_battery" || r.Tone != "red" {
		t.Errorf("on battery: %+v", r)
	}
	if r := rows["nut.battery"]; r.Value.Args["n"] != 45.0 || r.Meter.Tone != "red" {
		t.Errorf("under half full: %+v", r)
	}
	if f.Tone != "red" || f.Detail.Key != "nut.on_battery" || f.Meters[0].Tone != "red" {
		t.Errorf("the figure turns red: %+v", f)
	}
}

// A UPS that says less than an Eaton: only its load, its capacity, or its
// status.
func TestParts(t *testing.T) {
	t.Parallel()
	p := parts(map[string]string{"ups.status": "OL CHRG", "ups.load": "40", "ups.realpower.nominal": "900"})
	adaptertest.Validate(t, p)
	if len(p.Figures) != 1 || p.Figures[0].Value != 360 || len(p.Figures[0].Meters) != 0 {
		t.Errorf("draws 40%% of 900 W, and has no battery to show: %+v", p.Figures)
	}
	p = parts(map[string]string{"ups.status": "OL", "ups.load": "40"})
	if len(p.Figures) != 0 || len(p.Sections[0].Rows) != 2 || p.Sections[0].Rows[1].Value.Key != "percent" {
		t.Errorf("with no watts, no figure, and the load as a share: %+v", p)
	}
	p = parts(map[string]string{"ups.status": "OFF"})
	adaptertest.Validate(t, p)
	if r := p.Sections[0].Rows; len(r) != 1 || r[0].Value.Name != "OFF" || r[0].Tone != "gray" || r[0].Detail != nil {
		t.Errorf("a status it doesn't know, as upsd words it: %+v", r)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{Host: "nut.example.com", UPS: "ups"}
	if err := c.Check(); err != nil || c.Name != "NUT" || c.Host != "nut.example.com:3493" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{UPS: "ups"}, "host is missing"},
		{Config{Host: "http://nut.example.com", UPS: "ups"}, "host:"},
		{Config{Host: "nut.example.com"}, "ups is missing"},
		{Config{Host: "nut.example.com", UPS: "ups\nFSD ups"}, "ups:"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
