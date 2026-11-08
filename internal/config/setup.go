package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Adapter is one adapter as the setup page offers it (ADR 0014): what its
// package's snapshot.Adapter says it is, and what it asks of each source.
type Adapter struct {
	Key  string `json:"key"`
	Area string `json:"area,omitempty"`
	Kind string `json:"kind,omitempty"`
	// Storage says it gives stores or backup runs (ADR 0016), which the
	// setup page doesn't ask by, but the demo's test does.
	Storage bool `json:"storage,omitempty"`
	// Icon is its logo in web/icons, as its field in Sources names it.
	Icon string `json:"icon,omitempty"`
	Asks []Ask  `json:"asks"`

	// config is the type of its config, whose tags say what it asks.
	config reflect.Type
}

// Ask is one field of a source's config that the setup page asks for, as
// the field's setup: tag says: text, key or list, then optional if a
// source can be read without it. Its example: tag is an invented value,
// such as the source's usual address.
type Ask struct {
	Field string `json:"field"`
	// Key is a key or a password, which the page hides and writes to .env.
	Key bool `json:"key,omitempty"`
	// List is a list, such as GitHub's repos.
	List     bool   `json:"list,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Example  string `json:"example,omitempty"`
}

// Adapters are every adapter, in the order of Sources, with what each asks.
func Adapters() ([]Adapter, error) {
	// An empty config's check refuses nothing, and add meets every adapter.
	var c Config
	if err := c.check(); err != nil {
		return nil, err
	}
	icons := map[string]string{}
	sources := reflect.TypeFor[Sources]()
	for i := range sources.NumField() {
		f := sources.Field(i)
		icons[f.Tag.Get("yaml")] = f.Tag.Get("icon")
	}
	for i, a := range c.adapters {
		asks, err := asks(a.config, "")
		if err != nil {
			return nil, err
		}
		c.adapters[i].Asks, c.adapters[i].Icon = asks, icons[a.Key]
	}
	return c.adapters, nil
}

// asks are the fields of the config t that the setup page asks for. An
// inline config, which several adapters share, as Radarr and Sonarr do,
// takes the example of its url from the field that embeds it: example.
func asks(t reflect.Type, example string) ([]Ask, error) {
	var out []Ask
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			inner, err := asks(f.Type, f.Tag.Get("example"))
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		tag, ok := f.Tag.Lookup("setup")
		if !ok {
			continue
		}
		a := Ask{Field: strings.Split(f.Tag.Get("yaml"), ",")[0], Example: f.Tag.Get("example")}
		if a.Field == "url" && example != "" {
			a.Example = example
		}
		as, optional, _ := strings.Cut(tag, ",")
		a.Key, a.List, a.Optional = as == "key", as == "list", optional == "optional"
		if (as != "text" && !a.Key && !a.List) || (optional != "" && !a.Optional) {
			return nil, fmt.Errorf("%s.%s: setup:%q isn't text, key or list, then optional", t, f.Name, tag)
		}
		out = append(out, a)
	}
	return out, nil
}

// Tried is the config of one source as the setup page gives it to be tried:
// its adapter's key, and its fields as typed, with no ${VAR}. A source it
// refuses is refused with its adapter's own error, as "url is missing".
func Tried(adapter string, fields map[string]any) (Config, error) {
	raw, err := yaml.Marshal(map[string]any{"sources": map[string]any{adapter: []any{fields}}})
	if err != nil {
		return Config{}, err
	}
	c, err := decode(raw)
	// Without add's sources.<key>[0], since the page shows it beside the
	// source.
	if inner := errors.Unwrap(err); inner != nil {
		err = inner
	}
	return c, err
}
