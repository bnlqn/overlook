package config

import (
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
	t.Setenv("PHOTOS_ABOUT", "photos: # all of them")
	c, err := Load(write(t, `
sources:
  gatus:
    - url: https://overlook:${GATUS_PASSWORD}@status.example.com # not ${READ}
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
	if g.URL != "https://overlook:s3cret@status.example.com" || g.Name != "Gatus" || g.Threshold != 3 {
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
		"apps: [ { name: Photos } ]":                                             "apps[0]: url",
		"tools: [ { url: 'https://x.example.com' } ]":                            "tools[0]: name is missing",
		"apps: [ { name: P, url: 'javascript:alert(1)' } ]":                      "http:// or https://",
		"apps: [ { name: P, url: 'https://p.example.com', color: 'red;x' } ]":    "color",
		"apps: [ { name: P, url: 'https://p.example.com', check: P } ]":          "no check engine",
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
	t.Setenv("RADARR_KEY", "0123abcd")
	t.Setenv("SONARR_KEY", "4567efab")
	if _, err := Load("../../overlook.example.yaml"); err != nil {
		t.Fatal(err)
	}
}
