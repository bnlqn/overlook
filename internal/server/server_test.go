package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnlqn/overlook/internal/adapters/gatus"
	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/snapshot"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestRoutes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "overlook.yaml"), []byte("apps: []"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := New(config.Config{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/":                "<main",
		"/network":         `src="js/area.js"`,
		"/js/text.js":      "export function t",
		"/custom.css":      "body{}",
		"/icons/gatus.svg": "<svg",
		"/i18n/en.json":    `"title":"Homelab"`,
		"/i18n/fr.json":    `{}`,
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d, want %q", path, rec.Code, want)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self';") {
			t.Errorf("%s: no policy: %q", path, csp)
		}
	}
	for _, path := range []string{"/overlook.yaml", "/i18n/en", "/i18n/..%2fweb.go", "/icons/..%2f..%2fgo.mod", "/icons/..%2fstyle.css"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want nothing", path, rec.Code)
		}
	}
}

func TestSnapshot(t *testing.T) {
	t.Parallel()
	gone := httptest.NewServer(nil)
	gone.Close()
	c := config.Config{Sources: config.Sources{Gatus: []gatus.Config{{Name: "Gatus", URL: gone.URL, Link: gone.URL, Threshold: 3}}}}
	h, err := New(c, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, h, "/snapshot")
	var snap snapshot.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Sources) != 1 || snap.Sources[0].Answered || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Gatus didn't answer: %+v", snap.Sources)
	}
}
