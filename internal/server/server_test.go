package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
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
		"/i18n/fr.json":    `"title": "`,
		"/i18n/nl.json":    `{}`, // a language with no translation yet
		// What installs the page on a phone, from Overlook itself, at an
		// address relative to the page's own.
		"/manifest.json":        `"start_url": "./"`,
		"/apple-touch-icon.png": "PNG",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d, want %q", path, rec.Code, want)
		}
		// Its own scripts only, and framed only from its own address, as Home
		// Assistant's sidebar frames it (ADR 0019).
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self';") || !strings.HasSuffix(csp, "frame-ancestors 'self'") {
			t.Errorf("%s: no policy: %q", path, csp)
		}
	}
	// /setup too, which a setup page still open asks for until Overlook is
	// back with its config.
	for _, path := range []string{"/overlook.yaml", "/setup", "/i18n/en", "/i18n/..%2fweb.go", "/icons/..%2f..%2fgo.mod", "/icons/..%2fstyle.css"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want nothing", path, rec.Code)
		}
	}
}

func TestSnapshot(t *testing.T) {
	t.Parallel()
	gone := httptest.NewServer(nil)
	gone.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "overlook.yaml")
	if err := os.WriteFile(path, []byte("sources: { gatus: [ { url: '"+gone.URL+"' } ] }"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(c, dir)
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

func TestSetup(t *testing.T) {
	t.Parallel()
	const code = "K7QM-X3RD-NW5P"
	h, err := Setup(code)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/":             `src="js/setup.js"`,
		"/setup":        `"key":"gatus"`,
		"/js/setup.js":  "tryOne",
		"/i18n/en.json": `"setup.title"`,
	} {
		if rec := get(t, h, path); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d, want %q", path, rec.Code, want)
		}
	}
	for _, path := range []string{"/snapshot", "/servers", "/custom.css"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want nothing while there is no config", path, rec.Code)
		}
	}

	gatus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "[]") }))
	t.Cleanup(gatus.Close)
	try := func(origin, body string) (*httptest.ResponseRecorder, tried) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/setup", strings.NewReader(body))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var got tried
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return rec, got
	}
	const own = "http://example.com"
	source := `"adapter":"gatus","fields":{"url":"` + gatus.URL + `"}`
	body := `{"code":"` + code + `",` + source + `}`
	// Another Origin is refused, even with the code.
	for _, origin := range []string{"", "https://elsewhere.example.com", "null"} {
		if rec, _ := try(origin, body); rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q: %d, want refused", origin, rec.Code)
		}
	}
	// So is a try without the code, or with another, before anything is
	// read: a website that points its own name at Overlook passes the
	// Origin check, but can't read the log.
	for _, b := range []string{
		`{` + source + `}`, `{"code":"",` + source + `}`, `{"code":"K7QM-X3RD-NW5Q",` + source + `}`,
		`{"code":"K7QM-X3RD",` + source + `}`, `{"code":"K7QM-X3RD-NW5PA",` + source + `}`,
	} {
		if rec, got := try(own, b); rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Setup-Code" || got.Name != "" {
			t.Errorf("%s: %d %q %+v, want refused", b, rec.Code, rec.Header().Get("WWW-Authenticate"), got)
		}
	}
	// The code alone is how the page asks whether it's right.
	if rec, _ := try(own, `{"code":"`+code+`"}`); rec.Code != http.StatusNoContent {
		t.Errorf("the code alone: %d, want taken", rec.Code)
	}
	// The code is taken however it's typed: in either case, with or without
	// its dashes, and with spaces.
	for _, typed := range []string{code, "k7qmx3rdnw5p", " K7QM x3rd NW5P "} {
		if rec, got := try(own, `{"code":"`+typed+`",`+source+`}`); rec.Code != http.StatusOK || got != (tried{Name: "Gatus"}) {
			t.Errorf("Gatus with %q answered with nothing: %d %+v", typed, rec.Code, got)
		}
	}
	if _, got := try(own, `{"code":"`+code+`","adapter":"immich","fields":{"url":"http://immich.example.com"}}`); got.Error != "key is missing" || got.Why != nil {
		t.Errorf("Immich without its key: %+v", got)
	}
	gone := httptest.NewServer(nil)
	gone.Close()
	// httptest listens on 127.0.0.1: in a container, the container itself.
	if _, got := try(own, `{"code":"`+code+`","adapter":"gatus","fields":{"url":"`+gone.URL+`"}}`); got.Name != "Gatus" ||
		got.Why == nil || got.Why.Key != "setup.why.localhost" || got.Why.Args["name"] != "gatus" || got.Why.Args["host"] != "127.0.0.1" {
		t.Errorf("a Gatus that's gone: %+v %+v", got, got.Why)
	}
}

// A setup code is twelve characters of base32 in three groups of four, and
// a new one each time.
func TestCode(t *testing.T) {
	t.Parallel()
	shape := regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}-[A-Z2-7]{4}$`)
	a, b := Code(), Code()
	if !shape.MatchString(a) || !shape.MatchString(b) || a == b {
		t.Errorf("codes %q and %q", a, b)
	}
}

// A try that failed says what to do about it, by its kind, the fields its
// adapter asks for and the host it was given, in words the page has.
func TestWhy(t *testing.T) {
	t.Parallel()
	english, err := web.English()
	if err != nil {
		t.Fatal(err)
	}
	plain := config.Adapter{Key: "gatus", Asks: []config.Ask{{Field: "url"}}}
	asks := config.Adapter{Key: "proxmox", Asks: []config.Ask{{Field: "url"}, {Field: "ca"}, {Field: "server_name"}}}
	nut := config.Adapter{Key: "nut", Asks: []config.Ask{{Field: "host"}}}
	at := func(url string) map[string]any { return map[string]any{"url": url} }
	for _, tc := range []struct {
		kind      string
		a         config.Adapter
		fields    map[string]any
		key, host string
	}{
		{"certificate", asks, at("https://pve.example.com:8006"), "certificate", "pve.example.com"},
		{"certificate", plain, at("https://gatus.example.com"), "certificate_none", "gatus.example.com"},
		{"certificate_name", asks, at("https://pve.example.com:8006"), "certificate_name", "pve.example.com"},
		{"certificate_name", plain, at("https://gatus.example.com"), "certificate_name_none", "gatus.example.com"},
		{"plain", plain, at("https://gatus.example.com"), "plain", "gatus.example.com"},
		{"refused", plain, at("http://localhost:8080"), "localhost", "localhost"},
		{"refused", plain, at("http://127.0.0.2:8080"), "localhost", "127.0.0.2"},
		{"refused", plain, at("http://[::1]:8080"), "localhost", "::1"},
		{"refused", nut, map[string]any{"host": "[::1]:3493"}, "localhost", "::1"},
		{"refused", nut, map[string]any{"host": "localhost"}, "localhost", "localhost"},
		{"refused", nut, map[string]any{"host": "upsd.example.com:3493"}, "refused", "upsd.example.com"},
		{"refused", plain, at("http://gatus.example.com"), "refused", "gatus.example.com"},
		{"no_host", plain, at("http://nas.local"), "local", "nas.local"},
		{"no_host", plain, at("http://nas.example.com"), "no_host", "nas.example.com"},
		{"timeout", plain, at("http://nas.local:5000"), "local", "nas.local"},
		{"timeout", plain, at("http://nas.example.com:5000"), "timeout", "nas.example.com"},
		{"key", plain, at("https://gatus.example.com"), "key", "gatus.example.com"},
		{"not_found", plain, at("https://gatus.example.com"), "not_found", "gatus.example.com"},
	} {
		got := why(tc.kind, tc.a, tc.fields)
		if got == nil || got.Key != "setup.why."+tc.key || got.Args["host"] != tc.host || got.Args["name"] != tc.a.Key {
			t.Errorf("%s at %v: %+v, want setup.why.%s at %s", tc.kind, tc.fields, got, tc.key, tc.host)
			continue
		}
		if _, ok := english[got.Key]; !ok {
			t.Errorf("%s is missing", got.Key)
		}
	}
	// Every kind fetch tells apart has its words, and no kind has nothing to say.
	for _, kind := range fetch.Kinds {
		if why(kind, plain, at("https://gatus.example.com")) == nil {
			t.Errorf("%s: nothing to say", kind)
		}
	}
	if got := why("", plain, at("https://gatus.example.com")); got != nil {
		t.Errorf("no kind: %+v", got)
	}
}

// Every adapter the setup page offers, and every field it asks for, has its
// words, each adapter's name is its README section's heading, which the
// page links to, and its logo is one Overlook ships.
func TestSetupWords(t *testing.T) {
	t.Parallel()
	english, err := web.English()
	if err != nil {
		t.Fatal(err)
	}
	adapters, err := config.Adapters()
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range adapters {
		name, ok := english["setup.adapter."+a.Key].(string)
		if !ok {
			t.Errorf("setup.adapter.%s is missing", a.Key)
		} else if !strings.Contains(string(readme), "\n### "+name+"\n") {
			t.Errorf("%s: no README section is named %q", a.Key, name)
		}
		for _, ask := range a.Asks {
			if _, ok := english["setup.field."+ask.Field]; !ok {
				t.Errorf("%s: setup.field.%s is missing", a.Key, ask.Field)
			}
		}
		if _, err := fs.Stat(web.FS, "icons/"+a.Icon); a.Icon != "" && err != nil {
			t.Errorf("%s: %v", a.Key, err)
		}
	}
}

// The page names to a homelab with no check engine every check engine the
// setup page offers, in its order, from a list of its own in page.js.
func TestPageNamesCheckEngines(t *testing.T) {
	t.Parallel()
	adapters, err := config.Adapters()
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, a := range adapters {
		if a.Kind == "checkengine" {
			want = append(want, a.Key)
		}
	}
	src, err := fs.ReadFile(web.FS, "js/page.js")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	list := regexp.MustCompile(`CHECK_ENGINES = (\[[^\]]*\])`).FindSubmatch(src)
	if list == nil || json.Unmarshal(list[1], &got) != nil {
		t.Fatal("page.js has no CHECK_ENGINES list of keys")
	}
	if !slices.Equal(got, want) {
		t.Errorf("page.js names %v, the setup page offers %v", got, want)
	}
}

// On Unraid, which says so in HOST_OS, as "Unraid" or once "unRAID", the
// setup page says where to save the files there.
func TestSetupOnUnraid(t *testing.T) {
	for host, want := range map[string]bool{"unRAID": true, "Unraid": true, "": false} {
		t.Setenv("HOST_OS", host)
		h, err := Setup(Code())
		if err != nil {
			t.Fatal(err)
		}
		var got struct{ Unraid bool }
		if err := json.Unmarshal(get(t, h, "/setup").Body.Bytes(), &got); err != nil || got.Unraid != want {
			t.Errorf("HOST_OS %q: unraid %v, want %v (%v)", host, got.Unraid, want, err)
		}
	}
}

// In Home Assistant, which gives every app a SUPERVISOR_TOKEN, the setup
// page says where to save the files there.
func TestSetupInHomeAssistant(t *testing.T) {
	for token, want := range map[string]bool{"0123456789abcdef": true, "": false} {
		t.Setenv("SUPERVISOR_TOKEN", token)
		h, err := Setup(Code())
		if err != nil {
			t.Fatal(err)
		}
		var got struct{ HomeAssistant bool }
		if err := json.Unmarshal(get(t, h, "/setup").Body.Bytes(), &got); err != nil || got.HomeAssistant != want {
			t.Errorf("SUPERVISOR_TOKEN %q: homeassistant %v, want %v (%v)", token, got.HomeAssistant, want, err)
		}
	}
}
