package pbs

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds an answer recorded from a Proxmox Backup Server 4 with its
// names and numbers invented: four VMs backed up each night at 00:30 UTC to
// 28 October 2026, two of them again by hand on the 27th, one on the 26th,
// and a container last on the 19th. Only two of the four made it on the
// 24th. Verification is invented in its documented shape: the recorded
// backups hadn't been checked yet.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "overlook@pbs!overlook", Secret: "00000000-0000-0000-0000-000000000000", Datastore: "archive"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	rows := p.Sections[0].Rows
	if len(p.Sections) != 1 || len(rows) != 5 {
		t.Fatalf("each guest's newest backup: %+v", p.Sections)
	}
	names := []string{}
	for _, r := range rows {
		names = append(names, r.Title.Name)
	}
	if strings.Join(names, " ") != "house workbench gate ct/140 family" {
		t.Errorf("by number, named by their notes, or by their group without: %v", names)
	}
	if r := rows[0]; r.Detail.Key != "pbs.verified" || r.Detail.Args["size"] != int64(68_719_477_785) || r.Value.Args["at"] != time.Date(2026, 10, 28, 0, 30, 0, 0, time.UTC) {
		t.Errorf("house, verified, last night: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Value.Key != "pbs.failed" || r.Tone != "red" || r.Detail.Key != "pbs.failed_detail" {
		t.Errorf("workbench failed its check: %+v", r)
	}
	if r := rows[2]; r.Detail.Key != "pbs.unverified" {
		t.Errorf("gate isn't verified yet: %+v", r.Detail)
	}

	f := p.Figures[0]
	if f.Value != 4 || f.Unit.Args["n"] != 5 || f.Detail.Args["at"] != time.Date(2026, 10, 28, 0, 30, 0, 0, time.UTC) {
		t.Errorf("last night backed up 4 of 5 guests: %+v", f)
	}

	// The week to 10:00 on the 28th: the nights from the 22nd, a run by
	// hand on the 27th and one guest's on the 26th; the 21st's is older.
	raw, err := os.ReadFile("testdata/api2/json/admin/datastore/archive/snapshots.json")
	if err != nil {
		t.Fatal(err)
	}
	var all struct {
		Data []backup `json:"data"`
	}
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	var runs []any
	for _, e := range parts(all.Data, time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)).Events {
		runs = append(runs, e.Text.Args["n"])
	}
	if got := fmt.Sprint(runs); got != "[4 2 4 1 4 4 2 4 4]" {
		t.Errorf("the week's runs, newest first, by the guests each backed up: %s", got)
	}
}

// A guest backed up by hand after the night is a run of its own. Today shows
// the night all the same, while it's less than a day older than the newest
// backup.
func TestTodayIsTheNight(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 10, 27, 1, 30, 0, 0, time.UTC)
	var backups []backup
	for i, id := range []string{"100", "110", "130", "150"} {
		backups = append(backups, backup{Type: "vm", ID: id, Time: start.Add(time.Duration(i) * 3 * time.Minute).Unix()})
	}
	backups = append(backups, backup{Type: "vm", ID: "130", Time: time.Date(2026, 10, 27, 17, 16, 0, 0, time.UTC).Unix()})
	if f := parts(backups, now).Figures[0]; f.Value != 4 || f.Unit.Args["n"] != 4 || f.Detail.Args["at"] != start {
		t.Errorf("the night backed up 4 of 4 guests, before gate by hand: %+v", f)
	}

	// The next night never came: the day to the newest backup holds only
	// backups by hand, and Today shows the newest.
	byHand := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	backups = append(backups, backup{Type: "vm", ID: "100", Time: byHand.Unix()})
	if f := parts(backups, now).Figures[0]; f.Value != 1 || f.Detail.Args["at"] != byHand {
		t.Errorf("house by hand, a day after the night: %+v", f)
	}
}

// PBS signs its own certificate for its own name: Overlook reads it at
// another address, trusting that certificate and checking that name, and
// sends its token as PBS expects.
func TestCertificate(t *testing.T) {
	t.Parallel()
	var auth string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data": []}`))
	}))
	t.Cleanup(s.Close)
	ca := filepath.Join(t.TempDir(), "proxy.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	// The test certificate is for example.com and 127.0.0.1; the URL names
	// localhost, as an address the certificate doesn't carry would.
	url := strings.Replace(s.URL, "127.0.0.1", "localhost", 1)
	c := Config{URL: url, Token: "overlook@pbs!overlook", Secret: "s3cret", Datastore: "archive", CA: ca}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("a certificate for another name is refused: %v", err)
	}
	c.ServerName = "example.com"
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if auth != "PBSAPIToken=overlook@pbs!overlook:s3cret" {
		t.Errorf("the token's header: %q", auth)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "https://pbs.example.com:8007", Token: "overlook@pbs!overlook", Secret: "s3cret", Datastore: "archive"}
	if err := c.Check(); err != nil || c.Name != "Backup Server" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	base := func(change func(*Config)) Config {
		c := Config{URL: "https://pbs.example.com:8007", Token: "overlook@pbs!overlook", Secret: "s", Datastore: "archive"}
		change(&c)
		return c
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{base(func(c *Config) { c.URL = "pbs.example.com" }), "url needs"},
		{base(func(c *Config) { c.Token = "overlook@pbs" }), "token needs"},
		{base(func(c *Config) { c.Secret = "" }), "secret is missing"},
		{base(func(c *Config) { c.Datastore = "" }), "datastore is missing"},
		{base(func(c *Config) { c.ServerName = "pbs.example.com" }), "server_name needs the ca"},
		{base(func(c *Config) { c.CA = "no-such.pem" }), "ca: open no-such.pem"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
