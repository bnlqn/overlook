package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/bnlqn/overlook/internal/snapshot"
)

func TestAdapters(t *testing.T) {
	t.Parallel()
	adapters, err := Adapters()
	if err != nil {
		t.Fatal(err)
	}
	if adapters[0].Key != "gatus" || adapters[0].Kind != "checkengine" {
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

// Each field of Sources is an adapter's, in its place on check's list: its
// tag is the adapter's key and its type a list of the adapter's config, so
// that the key, written twice, can't drift.
func TestSources(t *testing.T) {
	t.Parallel()
	adapters, err := Adapters()
	if err != nil {
		t.Fatal(err)
	}
	sources := reflect.TypeFor[Sources]()
	if sources.NumField() != len(adapters) {
		t.Fatalf("%d fields in Sources, %d adapters on check's list", sources.NumField(), len(adapters))
	}
	for i, a := range adapters {
		f := sources.Field(i)
		if f.Tag.Get("yaml") != a.Key || f.Type != reflect.SliceOf(a.config) {
			t.Errorf("Sources.%s, %s under %q, is in the place of %s, whose config is %s", f.Name, f.Type, f.Tag.Get("yaml"), a.Key, a.config)
		}
	}
}

// What each adapter says it is holds together: a key of its own, a home
// area of the page's or none, a kind of the four or none, no home area for
// a check engine, whose checks go in every area, and one for a resolver or
// a media server, whose parts go in it (ADR 0012).
func TestWhatAdaptersSay(t *testing.T) {
	t.Parallel()
	adapters, err := Adapters()
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range adapters {
		if slices.ContainsFunc(adapters[:i], func(b Adapter) bool { return b.Key == a.Key }) {
			t.Errorf("%s: another adapter has its key", a.Key)
		}
		if a.Area != "" && !slices.Contains(snapshot.Areas, a.Area) {
			t.Errorf("%s: home area %q isn't one of %v", a.Key, a.Area, snapshot.Areas)
		}
		switch a.Kind {
		case "", "todolist":
		case "checkengine":
			if a.Area != "" {
				t.Errorf("%s: a check engine, with a home area, %s", a.Key, a.Area)
			}
		case "resolver", "mediaserver":
			if a.Area == "" {
				t.Errorf("%s: a %s, with no home area", a.Key, a.Kind)
			}
		default:
			t.Errorf("%s: kind %q isn't checkengine, todolist, resolver or mediaserver", a.Key, a.Kind)
		}
	}
}

// Every adapter's config names its source in its field tagged yaml:"name",
// where add reads it.
func TestName(t *testing.T) {
	t.Parallel()
	adapters, err := Adapters()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range adapters {
		v := reflect.New(a.config)
		if err := yaml.Unmarshal([]byte("name: Attic"), v.Interface()); err != nil {
			t.Fatal(err)
		}
		if n := name(v.Elem()); n != "Attic" {
			t.Errorf("%s: name %q, want Attic", a.Key, n)
		}
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
