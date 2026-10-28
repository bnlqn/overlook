package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func writeConfig(t *testing.T, yaml string) string {
	path := filepath.Join(t.TempDir(), "overlook.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfig(t *testing.T) {
	t.Setenv("GATUS_PASSWORD", "s3cret: #not a comment")
	c, err := loadConfig(writeConfig(t, `
sources:
  gatus:
    - url: https://overlook:${GATUS_PASSWORD}@status.example.com # read with ${NOT_READ}
      areas: { DNS: network }
apps:
  - { name: Photos, url: "https://photos.example.com", check: "8080" }
`))
	if err != nil {
		t.Fatal(err)
	}
	g := c.Sources.Gatus[0]
	if g.URL != "https://overlook:s3cret: #not a comment@status.example.com" || g.Name != "Gatus" || g.Threshold != 3 || g.Link != g.URL {
		t.Errorf("read %+v", g)
	}
	if c.Apps[0].Check != "8080" {
		t.Errorf("a number in a name stays a name: %q", c.Apps[0].Check)
	}

	for yaml, want := range map[string]string{
		"sources: { gatus: [ { url: x, treshold: 2 } ] }":           "treshold",
		"sources: { gatus: [ { url: '${NO_SUCH_VAR}' } ] }":         "NO_SUCH_VAR is not set",
		"sources: { gatus: [ { url: x, areas: { DNS: attic } } ] }": "attic",
		"apps: [ { name: Photos } ]":                                "a name and a url",
		"":                                                          "empty",
	} {
		if _, err := loadConfig(writeConfig(t, yaml)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want an error naming %q, got %v", yaml, want, err)
		}
	}
}

func TestExampleConfig(t *testing.T) {
	t.Setenv("GATUS_URL", "http://gatus:8080")
	if _, err := loadConfig("overlook.example.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestClientOnlyReads(t *testing.T) {
	var reached atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer s.Close()
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "HEAD"} {
		req, _ := http.NewRequest(method, s.URL, nil)
		if _, err := client.Do(req); err == nil {
			t.Errorf("%s went through", method)
		}
	}
	if reached.Load() != 0 {
		t.Errorf("%d requests reached the source", reached.Load())
	}
	var v any
	if err := getJSON(context.Background(), s.URL, &v); err == nil || reached.Load() != 1 {
		t.Errorf("a GET should reach it (and its empty answer fail as JSON): %v", err)
	}
}

// A source that doesn't answer is marked so and its parts left out, and a
// snapshot is read again only once it's 30 seconds old.
func TestSnapshot(t *testing.T) {
	gatus := replay(t, "testdata/gatus")
	var reads atomic.Int32
	counted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		gatus.Config.Handler.ServeHTTP(w, r)
	}))
	defer counted.Close()
	gone := httptest.NewServer(nil)
	gone.Close()
	var c Config
	c.Sources.Gatus = []GatusConfig{{Name: "Gatus", URL: counted.URL, Link: counted.URL, Threshold: 3}, {Name: "Spare", URL: gone.URL, Link: gone.URL, Threshold: 3}}

	rec := httptest.NewRecorder()
	h := handler(c, t.TempDir())
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/snapshot", nil))
	var snap Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Sources) != 2 || !snap.Sources[0].Answered || snap.Sources[1].Answered || snap.Sources[1].Error == "" {
		t.Errorf("sources: %+v", snap.Sources)
	}
	if strings.Contains(snap.Sources[1].Error, gone.URL) {
		t.Errorf("an error shouldn't hold the source's url: %s", snap.Sources[1].Error)
	}
	if len(snap.Checks) != 12 {
		t.Errorf("want Gatus's 12 checks, got %d", len(snap.Checks))
	}
	n := reads.Load()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/snapshot", nil))
	if reads.Load() != n {
		t.Errorf("a fresh snapshot was read again")
	}
}

func TestPageRoutes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "custom.css"), []byte("body{}"), 0o600)
	h := handler(Config{}, dir)
	for path, want := range map[string]string{
		"/":                "<main",
		"/network":         "<main",
		"/custom.css":      "body{}",
		"/icons/gatus.svg": "<svg",
		"/i18n/en.json":    `"title"`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d, want %q", path, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/overlook.yaml", nil))
	if rec.Code != 404 {
		t.Errorf("the config's folder is served beyond icons/ and custom.css")
	}
}
