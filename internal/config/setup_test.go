package config

import (
	"slices"
	"strings"
	"testing"
)

func TestAdapters(t *testing.T) {
	t.Parallel()
	adapters, err := Adapters()
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) != 29 || adapters[0].Key != "gatus" || adapters[0].Kind != "checkengine" {
		t.Fatalf("every adapter, in the order of Sources: %+v", adapters)
	}
	for _, a := range adapters {
		if !slices.ContainsFunc(a.Asks, func(k Ask) bool { return k.Field == "name" && k.Optional }) {
			t.Errorf("%s doesn't ask for a name", a.Key)
		}
	}
	at := func(key string) Adapter {
		return adapters[slices.IndexFunc(adapters, func(a Adapter) bool { return a.Key == key })]
	}
	if p := at("proxmox"); p.Area != "servers" || len(p.Asks) != 5 || p.Asks[3] != (Ask{Field: "secret", Key: true}) || p.Asks[4].Field != "ca" {
		t.Errorf("proxmox asks %+v", p)
	}
	if r := at("radarr"); r.Asks[1] != (Ask{Field: "url", Example: "http://radarr.example.com:7878"}) {
		t.Errorf("radarr's url, from the field that embeds servarr's config: %+v", r.Asks)
	}
	if g := at("github"); g.Asks[1] != (Ask{Field: "repos", List: true, Example: "example/homelab example/dotfiles"}) {
		t.Errorf("github asks %+v", g.Asks)
	}
}

func TestTried(t *testing.T) {
	t.Parallel()
	c, err := Tried("immich", map[string]any{"url": "http://immich.example.com:2283", "key": "k3y"})
	if err != nil || len(c.Readers()) != 1 || c.Readers()[0].Name != "Immich" {
		t.Errorf("one source to read: %v %+v", err, c.Readers())
	}
	for _, tc := range []struct{ adapter, url, want string }{
		{"immich", "http://immich.example.com:2283", "key is missing"},
		{"gatus", "ftp://gatus.example.com", "url needs to be"},
		{"attic", "http://attic.example.com", "field attic not found"},
	} {
		_, err := Tried(tc.adapter, map[string]any{"url": tc.url})
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.HasPrefix(err.Error(), "sources.") {
			t.Errorf("%s: want its own error with %q, got %v", tc.adapter, tc.want, err)
		}
	}
}
