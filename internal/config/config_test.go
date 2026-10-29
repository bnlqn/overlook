package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overlook.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	t.Setenv("GATUS_PASSWORD", "s3cret")
	t.Setenv("IMMICH_KEY", "k3y")
	t.Setenv("ADGUARD_PASSWORD", "s3cret")
	t.Setenv("PHOTOS_ABOUT", "photos: # all of them")
	c, err := Load(write(t, `
sources:
  gatus:
    - url: https://status.example.com # not ${READ}
      user: overlook
      password: ${GATUS_PASSWORD}
      areas: { DNS: network }
  immich:
    - url: http://immich:2283
      key: ${IMMICH_KEY}
apps:
  - name: Photos
    url: https://photos.example.com
    check: "8080"
    color: "#ED79B5"
    description: ${PHOTOS_ABOUT}
`))
	if err != nil {
		t.Fatal(err)
	}
	g := c.Sources.Gatus[0]
	if g.URL != "https://status.example.com" || g.Password != "s3cret" || g.Name != "Gatus" || g.Threshold != 3 {
		t.Errorf("read %+v", g)
	}
	if c.Apps[0].Description != "photos: # all of them" {
		t.Errorf("a value can't change the file's structure: %q", c.Apps[0].Description)
	}
	if c.Apps[0].Check != "8080" {
		t.Errorf("a number in a name stays a name: %q", c.Apps[0].Check)
	}
	if im := c.Sources.Immich[0]; im.Key != "k3y" || im.Name != "Immich" {
		t.Errorf("read %+v", im)
	}
	if r := c.Readers(); len(r) != 2 || r[0].Name != "Gatus" || r[1].Name != "Immich" {
		t.Errorf("two sources to read: %+v", r)
	}
}

// With two check engines, a link names the one its check is in as
// Engine/Check; a slash in a check's own name stays in its name.
func TestLoadEngine(t *testing.T) {
	c, err := Load(write(t, `
sources:
  gatus:
    - url: http://gatus:8080
    - url: https://status.example.com
      name: Gatus Outside
apps:
  - name: Photos
    url: https://photos.example.com
    check: Gatus Outside/Photos
  - name: Files
    url: https://files.example.com
    check: NAS/SMB
`))
	if err != nil {
		t.Fatal(err)
	}
	if a := c.Apps[0]; a.Engine != "Gatus Outside" || a.Check != "Photos" {
		t.Errorf("the engine named: %+v", a)
	}
	if a := c.Apps[1]; a.Engine != "" || a.Check != "NAS/SMB" {
		t.Errorf("a check's own slash: %+v", a)
	}
}

// A password is whatever the source allows, so one with a URL's own
// characters in it still reaches the source as it is.
func TestLoadAnyPassword(t *testing.T) {
	const password = "p@ss/w#rd?%:"
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "overlook" || pass != password {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		// An empty answer of each kind: Gatus's lists, AdGuard's objects.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fmt.Fprint(w, "[]")
		} else {
			fmt.Fprint(w, "{}")
		}
	}))
	t.Cleanup(src.Close)
	t.Setenv("PASSWORD", password)
	for _, adapter := range []string{"adguard", "gatus"} {
		c, err := Load(write(t, "sources: { "+adapter+": [ { url: '"+src.URL+"', user: overlook, password: '${PASSWORD}' } ] }"))
		if err != nil {
			t.Errorf("%s: %v", adapter, err)
			continue
		}
		if _, err := c.Readers()[0].Read(context.Background()); err != nil {
			t.Errorf("%s: %v", adapter, err)
		}
	}
}

func TestLoadRefuses(t *testing.T) {
	t.Parallel()
	for yaml, want := range map[string]string{
		"sources: { gatus: [ { url: 'http://gatus', treshold: 2 } ] }":           "treshold",
		"sources: { gatus: [ { url: '${OVERLOOK_NO_SUCH_VAR}' } ] }":             "OVERLOOK_NO_SUCH_VAR is not set",
		"sources: { gatus: [ { url: 'http://a' }, { url: 'http://b' } ] }":       "another source is named Gatus",
		"sources: { gatus: [ { url: 'http://gatus', areas: { DNS: attic } } ] }": "sources.gatus[0]: areas",
		"sources: { github: [ { repos: [homelab] } ] }":                          "sources.github[0]: repos",
		"sources: { immich: [ { url: 'http://immich:2283' } ] }":                 "sources.immich[0]: key is missing",
		"sources: {immich: [{url: http://a, key: k}, {url: http://b, key: k}]}":  "another source is named Immich",
		"sources: { radarr: [ { url: 'http://radarr:7878' } ] }":                 "sources.radarr[0]: key is missing",
		"sources: { sonarr: [ { key: k } ] }":                                    "sources.sonarr[0]: url is missing",
		"sources: { nut: [ { host: nut.example.com } ] }":                        "sources.nut[0]: ups is missing",
		"sources: { adguard: [ { url: 'http://a' }, { url: 'http://b' } ] }":     "sources.adguard[1]: another source is named AdGuard Home",
		"apps: [ { name: Photos } ]":                                             "apps[0]: url",
		"tools: [ { url: 'https://x.example.com' } ]":                            "tools[0]: name is missing",
		"apps: [ { name: P, url: 'javascript:alert(1)' } ]":                      "http:// or https://",
		"apps: [ { name: P, url: 'https://p.example.com', color: 'red;x' } ]":    "color",
		"apps: [ { name: P, url: 'https://p.example.com', check: P } ]":          "no check engine",
		"sources: { ical: [ { name: Chores } ] }":                                "sources.ical[0]: url is missing",
		"chores: [ { name: Dust, from: 2026-11-02 } ]":                           "field chores not found",
		"":           "empty",
		"sources: [": "did not find expected",
	} {
		if _, err := Load(write(t, yaml)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want an error with %q, got %v", yaml, want, err)
		}
	}
}

func TestExample(t *testing.T) {
	t.Setenv("GATUS_URL", "http://gatus:8080")
	t.Setenv("IMMICH_URL", "http://immich:2283")
	t.Setenv("IMMICH_KEY", "k3y")
	t.Setenv("PBS_SECRET", "00000000-0000-0000-0000-000000000000")
	t.Setenv("PROXMOX_SECRET", "00000000-0000-0000-0000-000000000000")
	t.Setenv("BESZEL_PASSWORD", "s3cret")
	t.Setenv("RADARR_KEY", "0123abcd")
	t.Setenv("SONARR_KEY", "4567efab")
	t.Setenv("PROWLARR_KEY", "89abcdef")
	t.Setenv("QBITTORRENT_KEY", "qbt_0123abcd")
	t.Setenv("JELLYFIN_KEY", "0123abcd")
	t.Setenv("HOMEASSISTANT_TOKEN", "0123abcd")
	t.Setenv("NUT_HOST", "nut.example.com")
	t.Setenv("ADGUARD_PASSWORD", "s3cret")
	t.Setenv("DAV_PASSWORD", "s3cret")
	if _, err := Load("../../overlook.example.yaml"); err != nil {
		t.Fatal(err)
	}
}
