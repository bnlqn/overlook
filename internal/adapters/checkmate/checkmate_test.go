package checkmate

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// at is a time of the recording, on 31 October 2026, in UTC.
func at(t *testing.T, clock string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, "2026-10-31T"+clock+"Z")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// read replays testdata, with edits, to Overlook's user, and returns its
// checks by name, and what it was sent.
func read(t *testing.T, edits ...string) (map[string]snapshot.Check, []adaptertest.Request) {
	t.Helper()
	src := adaptertest.Replay(t, "testdata", edits...)
	c := Config{URL: src.URL, User: "overlook@example.com", Password: "Overlook-password-1", Link: "https://checkmate.example.com/", Areas: map[string]string{"Media": "apps"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, at(t, "10:10:21"))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, "", p)
	by := map[string]snapshot.Check{}
	for _, ch := range p.Checks {
		by[ch.Name] = ch
	}
	return by, src.Sent()
}

// testdata holds answers recorded from a scratch Checkmate 3.13.1 (ADR 0008),
// which testdata/fill.sh fills: eleven monitors checked every 20 seconds
// from 09:59:51, in groups named Servers, Network, Storage and Media, one in
// none, and Old wiki, paused. Standby DNS has been down since its first
// check; Offsite copy failed from 10:02:52 to 10:04:35, and Checkmate
// called it down; Films has failed its last three checks, of the five its
// status window needs; NAS agent, a hardware monitor, has been breached
// since 10:02:36, its memory over 1%. record.sh read them at 10:10:21.
func TestRead(t *testing.T) {
	t.Parallel()
	by, sent := read(t)

	login := sent[0]
	if login.Method != http.MethodPost || login.URL.Path != "/api/v1/auth/login" || login.Body != `{"email":"overlook@example.com","password":"Overlook-password-1"}` {
		t.Errorf("first, the login: %+v", login)
	}
	var answer struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "api", "v1", "auth", "login.json"))
	if err != nil || json.Unmarshal(raw, &answer) != nil {
		t.Fatal("the login's answer", err)
	}
	for _, r := range sent[1:] {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+answer.Data.Token {
			t.Errorf("%s %s: read without the token", r.Method, r.URL)
		}
	}
	// The monitors, the incidents open and ended, and each hardware
	// monitor's checks.
	if len(sent) != 6 {
		t.Errorf("a login and five reads, sent %d", len(sent))
	}

	if len(by) != 10 {
		t.Errorf("ten monitors, Old wiki's pause leaving it out: %d", len(by))
	}
	if _, ok := by["Old wiki"]; ok {
		t.Error("a paused monitor is left out")
	}
	for name, area := range map[string]string{"Hypervisor": "servers", "Router": "network", "Backup target": "storage", "Films": "apps", "Test page": ""} {
		if by[name].Area != area {
			t.Errorf("%s: in %q, want %q", name, by[name].Area, area)
		}
	}

	dns := by["Standby DNS"]
	if !dns.Problem || dns.Run != snapshot.Recent || dns.Threshold != 3 || dns.Why != "connect ECONNREFUSED 127.0.0.1:53" ||
		!dns.Since.Equal(at(t, "09:59:51.132")) || len(dns.Outages) != 1 || dns.Outages[0].To != nil {
		t.Errorf("Standby DNS, down since its first check: %+v", dns)
	}
	if dns.URL != "https://checkmate.example.com/uptime/6ae5bc179fc2e0de2a484ac8" {
		t.Errorf("a monitor links to its page in Checkmate: %s", dns.URL)
	}

	off := by["Offsite copy"]
	if off.Problem || off.Run != 0 || len(off.Outages) != 1 || !off.Outages[0].From.Equal(at(t, "10:02:52.550")) || !off.Outages[0].To.Equal(at(t, "10:04:35.539")) {
		t.Errorf("Offsite copy, a problem from its first failure to its next pass: %+v", off.Outages)
	}

	films := by["Films"]
	if films.Problem || films.Run != 3 || films.Threshold != 5 || len(films.Outages) != 0 ||
		films.Why != "Request failed with status code 404 (Not Found): GET http://capture:59232/films" {
		t.Errorf("Films, failing 3 of the 5 its window of 10 at 50%% needs: %+v", films)
	}

	nas := by["NAS agent"]
	if !nas.Problem || nas.Run != 23 || !nas.Since.Equal(at(t, "10:02:52.510")) || nas.Why != "MEMORY: 10.9% (threshold: 1%)" {
		t.Errorf("NAS agent, breached since its first check after the breach: %+v", nas)
	}
	if len(nas.Results) != snapshot.Recent || nas.Results[0].MS == nil || !nas.Results[snapshot.Recent-1].At.Equal(at(t, "10:10:14.430")) ||
		nas.URL != "https://checkmate.example.com/infrastructure/6ae5bc179fc2e0de2a484ab6" {
		t.Errorf("a hardware monitor's last 30 results, from its own read: %+v", nas)
	}
	if h := by["Hypervisor agent"]; h.Problem || h.Run != 0 || len(h.Results) != snapshot.Recent {
		t.Errorf("Hypervisor agent: %+v", h)
	}

	h := by["Hypervisor"]
	if h.Problem || h.Run != 0 || h.Threshold != 3 || len(h.Results) != snapshot.Recent || h.Results[0].MS == nil || !h.Watched.Equal(at(t, "09:59:51.020")) {
		t.Errorf("Hypervisor, watched since it was made: %+v", h)
	}
}

// An incident older than the results is a stretch of its own, at
// Checkmate's times, and one still open is the problem going on, since it
// opened. A run of failures no incident overlaps alerted nothing.
func TestOlderIncidents(t *testing.T) {
	t.Parallel()
	by, _ := read(t,
		`"startTime": "2026-10-31T09:59:54.071Z"`, `"startTime": "2026-10-29T22:10:00.000Z"`,
		`"startTime": "2026-10-31T10:03:34.958Z"`, `"startTime": "2026-10-30T14:00:00.000Z"`,
		`"endTime": "2026-10-31T10:05:13.933Z"`, `"endTime": "2026-10-30T14:20:00.000Z"`,
	)
	if dns := by["Standby DNS"]; !dns.Problem || !dns.Since.Equal(time.Date(2026, 10, 29, 22, 10, 0, 0, time.UTC)) || len(dns.Outages) != 1 {
		t.Errorf("Standby DNS, down for two days: %+v", dns)
	}
	off := by["Offsite copy"]
	if len(off.Outages) != 1 || !off.Outages[0].From.Equal(time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)) ||
		!off.Outages[0].To.Equal(time.Date(2026, 10, 30, 14, 20, 0, 0, time.UTC)) {
		t.Errorf("Offsite copy, a problem yesterday and not this morning: %+v", off.Outages)
	}
}

// A monitor put in a maintenance window while it was down keeps its last
// results, failing, as Checkmate checks nothing in the window, but isn't a
// problem, as Checkmate alerts on none there. Its incident stays open.
func TestMaintenance(t *testing.T) {
	t.Parallel()
	by, _ := read(t, `"status": "down"`, `"status": "maintenance"`)
	if dns := by["Standby DNS"]; dns.Problem || dns.Run != snapshot.Recent || dns.Threshold != 0 || len(dns.Outages) != 0 {
		t.Errorf("Standby DNS, failing in maintenance: %+v", dns)
	}
}

// An incident Checkmate left open when a maintenance window or a pause took
// its monitor out of breached is over: the monitor's results pass.
func TestIncidentLeftOpen(t *testing.T) {
	t.Parallel()
	by, _ := read(t, `"status": "breached"`, `"status": "up"`)
	if nas := by["NAS agent"]; nas.Problem || nas.Run != 0 || nas.Why != "" || len(nas.Outages) != 0 {
		t.Errorf("NAS agent, up, its breach left open: %+v", nas)
	}
}

// A breach someone resolved by hand goes on while Checkmate calls the
// monitor breached: a problem is never dismissed.
func TestBreachResolvedByHand(t *testing.T) {
	t.Parallel()
	by, _ := read(t,
		"\"startTime\": \"2026-10-31T10:02:36.540Z\",\n        \"endTime\": null,",
		"\"startTime\": \"2026-10-31T10:02:36.540Z\",\n        \"endTime\": \"2026-10-31T10:05:00.000Z\",",
	)
	if nas := by["NAS agent"]; !nas.Problem || nas.Run != 23 || !nas.Since.Equal(at(t, "10:02:52.510")) {
		t.Errorf("NAS agent, still breached: %+v", nas)
	}
}

// A login Checkmate refuses is an error that says so.
func TestLoginRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	login := filepath.Join(dir, "api", "v1", "auth", "login.http")
	if err := os.MkdirAll(filepath.Dir(login), 0o750); err != nil {
		t.Fatal(err)
	}
	answer := "HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json; charset=utf-8\r\n\r\n" + `{"status":401,"msg":"Invalid credentials"}`
	if err := os.WriteFile(login, []byte(answer), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Config{URL: adaptertest.Replay(t, dir).URL, User: "overlook@example.com", Password: "wrong"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, time.Time{}); err == nil || !strings.Contains(err.Error(), "login: HTTP 401") {
		t.Errorf("a login refused: %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://checkmate:52345", User: "overlook@example.com", Password: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "Checkmate" || c.Link != c.URL || c.source.Login == nil {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "checkmate:52345"}, "url needs"},
		{Config{URL: "http://checkmate:52345", User: "overlook@example.com"}, "user and password"},
		{Config{URL: "http://checkmate:52345", User: "overlook@example.com", Password: "s3cret", Link: "https://overlook:s3cret@checkmate.example.com"}, "link can't hold a login"},
		{Config{URL: "http://checkmate:52345", User: "overlook@example.com", Password: "s3cret", Areas: map[string]string{"DNS": "attic"}}, "attic"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
