package beszel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// hub answers as Beszel's does: a login for overlook@example.com and s3cret
// with a token, getkey with a 401 without it, and a list with testdata's
// answer, or with nothing in it, as a guest's, without it.
type hub struct {
	*httptest.Server
	logins atomic.Int32

	mu    sync.Mutex
	token string
}

func newHub(t *testing.T) *hub {
	t.Helper()
	h := &hub{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		in := h.token != "" && r.Header.Get("Authorization") == h.token
		h.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collections/users/auth-with-password":
			n := h.logins.Add(1)
			var login struct{ Identity, Password string }
			if json.NewDecoder(r.Body).Decode(&login) != nil || login.Identity != "overlook@example.com" || login.Password != "s3cret" {
				http.Error(w, `{"status": 400, "message": "Failed to authenticate."}`, http.StatusBadRequest)
				return
			}
			h.mu.Lock()
			h.token = fmt.Sprintf("token-%d", n)
			_, _ = fmt.Fprintf(w, `{"token": %q, "record": {"role": "readonly"}}`, h.token)
			h.mu.Unlock()
		case r.Method != http.MethodGet:
			t.Errorf("%s %s: a source is only read", r.Method, r.URL.Path)
		case in:
			http.ServeFile(w, r, filepath.Join("testdata", r.URL.Path+".json"))
		case r.URL.Path == "/api/beszel/getkey":
			http.Error(w, `{"status": 401}`, http.StatusUnauthorized)
		default:
			_, _ = w.Write([]byte(`{"items": [], "page": 1, "perPage": 1000}`))
		}
	}))
	t.Cleanup(h.Close)
	return h
}

// testdata holds answers recorded from Beszel 0.21.1 with its systems
// renamed, their IDs and the hub's key replaced, every number nudged, and
// the times moved to end at 28 October 2026, 09:30 UTC: nova, a Proxmox
// host with a week of records, and three VMs with a day or two, of which
// vega has been down since 06:12.
func TestRead(t *testing.T) {
	t.Parallel()
	h := newHub(t)
	c := Config{URL: h.URL, User: "overlook@example.com", Password: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != snapshot.HostsSection.Key {
		t.Fatalf("one section, the hosts: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 4 || rows[0].Title.Name != "atlas" || rows[3].Title.Name != "vega" {
		t.Fatalf("four hosts, by name: %+v", rows)
	}
	nova := rows[2]
	if nova.Title.Name != "nova" || nova.Value.Key != "beszel.up_for" || nova.Value.Args["ms"] != int64(442_885_000) || nova.Detail.Args["n"] != 16 || nova.Detail.Args["load"] != 0.32 {
		t.Errorf("nova, up 5 days, with 16 threads at a load of 0.32: %+v %+v", nova.Value, nova.Detail)
	}
	if m := nova.Meters; len(m) != 3 || m[0].Used != 2.31 || m[1].Used != 69.07 || m[2].Used != 6.78 || m[2].Total != 100 {
		t.Errorf("its processor, memory and disk, in percent: %+v", m)
	}
	if vega := rows[3]; vega.Value.Key != "beszel.down" || vega.Tone != "red" || vega.Meters != nil ||
		vega.Detail.Args["at"] != time.Date(2026, 10, 28, 6, 12, 41, 208_000_000, time.UTC) {
		t.Errorf("vega, down since 06:12, without what it said last: %+v %+v", vega, vega.Detail)
	}

	// The hosts up, a line each, from when the last of them has a record,
	// lyra's at 22:24 for the two hours before, to the agents' last, at 09:29.
	if c := p.Charts; len(c) != 2 || c[0].Title.Key != snapshot.ProcessorChart.Key || c[1].Title.Key != snapshot.MemoryChart.Key {
		t.Fatalf("the processor and memory by the hour, shared with any host's: %+v", c)
	}
	cpu := p.Charts[0].Series
	if len(cpu) != 3 || cpu[0].Label.Name != "atlas" || len(cpu[0].Hours) != 37 {
		t.Fatalf("three lines, vega's left out, over 37 hours: %d lines, %d hours", len(cpu), len(cpu[0].Hours))
	}
	if last := cpu[2].Hours[36]; cpu[2].Label.Name != "nova" || last != 2.3 {
		t.Errorf("nova's last hour, the agent's last reading: %v", last)
	}

	f := p.Figures[0]
	if f.Value != 3 || f.Unit.Args["n"] != 4 || f.Tone != "orange" || !f.Adds || len(f.Meters) != 2 {
		t.Errorf("3 of 4 hosts up, with their processor and memory: %+v", f)
	}
	if m := f.Meters[0]; m.Total != 22 {
		t.Errorf("the processor over the hosts' 22 threads: %+v", m)
	}
}

// A session Beszel forgot reads as a guest's, with nothing in it, so a read
// logs in again first, rather than show no hosts.
func TestLapsedSession(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC)
	h := newHub(t)
	c := Config{URL: h.URL, User: "overlook@example.com", Password: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, now); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.token = "forgotten"
	h.mu.Unlock()
	p, err := Read(context.Background(), c, now)
	if err != nil {
		t.Fatal(err)
	}
	if n := h.logins.Load(); n != 2 || len(p.Sections[0].Rows) != 4 {
		t.Errorf("logged in %d times, read %d hosts: want twice and 4", n, len(p.Sections[0].Rows))
	}

	c = Config{URL: h.URL, User: "overlook@example.com", Password: "wrong"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, now); err == nil || !strings.Contains(err.Error(), "login: HTTP 400") {
		t.Errorf("a login refused: %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://beszel:8090", User: "overlook@example.com", Password: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "Beszel" || c.source.Login == nil {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "beszel:8090"}, "url needs"},
		{Config{URL: "http://beszel:8090", User: "overlook@example.com"}, "user and password"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
