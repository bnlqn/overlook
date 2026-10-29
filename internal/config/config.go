// Package config reads overlook.yaml: the sources to read, and the apps and
// tools to link to. A key it doesn't know is an error, so a typo never goes unnoticed, and a ${VAR} comes from
// the environment.
package config

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/bnlqn/overlook/internal/adapters/adguard"
	"github.com/bnlqn/overlook/internal/adapters/gatus"
	"github.com/bnlqn/overlook/internal/adapters/github"
	"github.com/bnlqn/overlook/internal/adapters/ical"
	"github.com/bnlqn/overlook/internal/adapters/immich"
	"github.com/bnlqn/overlook/internal/adapters/nut"
	"github.com/bnlqn/overlook/internal/adapters/pbs"
	"github.com/bnlqn/overlook/internal/adapters/proxmox"
	"github.com/bnlqn/overlook/internal/adapters/radarr"
	"github.com/bnlqn/overlook/internal/adapters/sonarr"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is overlook.yaml.
type Config struct {
	Sources Sources         `yaml:"sources"`
	Apps    []snapshot.Link `yaml:"apps"`
	Tools   []snapshot.Link `yaml:"tools"`

	// readers are the sources check let through, each with its read.
	readers []snapshot.Source
}

// Sources are the sources to read, by adapter: each adapter's own config.
type Sources struct {
	Gatus   []gatus.Config   `yaml:"gatus"`
	GitHub  []github.Config  `yaml:"github"`
	Immich  []immich.Config  `yaml:"immich"`
	PBS     []pbs.Config     `yaml:"pbs"`
	Proxmox []proxmox.Config `yaml:"proxmox"`
	Radarr  []radarr.Config  `yaml:"radarr"`
	Sonarr  []sonarr.Config  `yaml:"sonarr"`
	NUT     []nut.Config     `yaml:"nut"`
	AdGuard []adguard.Config `yaml:"adguard"`
	Ical    []ical.Config    `yaml:"ical"`
}

// Readers are the sources, each with its adapter's read, as Load checked
// them.
func (c Config) Readers() []snapshot.Source { return c.readers }

// add checks the sources under sources.<key>, which list holds, and keeps
// each with its adapter's read. Each source's name must be its own.
func add[C any, P interface {
	*C
	Check() error
}](c *Config, key string, list []C, name func(C) string, read func(context.Context, C) (snapshot.Parts, error)) error {
	for i := range list {
		if err := P(&list[i]).Check(); err != nil {
			return fmt.Errorf("sources.%s[%d]: %w", key, i, err)
		}
		src, n := list[i], name(list[i])
		if slices.ContainsFunc(c.readers, func(s snapshot.Source) bool { return s.Name == n }) {
			return fmt.Errorf("sources.%s[%d]: another source is named %s: give each a name", key, i, n)
		}
		c.readers = append(c.readers, snapshot.Source{Name: n, Adapter: key, Read: func(ctx context.Context) (snapshot.Parts, error) { return read(ctx, src) }})
	}
	return nil
}

var (
	envVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	color  = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)
)

// Load reads the config at path.
func Load(path string) (Config, error) {
	c, err := load(path)
	if err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func load(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path) //nolint:gosec // the owner names their own config
	if err != nil {
		return c, err
	}
	// ${VAR} is replaced in values, after parsing, so that a value can't
	// change the file's structure and a comment can't name a var.
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return c, err
	}
	if len(doc.Content) == 0 {
		return c, errors.New("empty")
	}
	if err := interpolate(&doc); err != nil {
		return c, err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(out))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	return c, c.check()
}

func interpolate(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var unset error
		n.Value = envVar.ReplaceAllStringFunc(n.Value, func(m string) string {
			name := envVar.FindStringSubmatch(m)[1]
			v, ok := os.LookupEnv(name)
			if !ok && unset == nil {
				unset = fmt.Errorf("%s is not set", name)
			}
			return v
		})
		return unset
	}
	for _, child := range n.Content {
		if err := interpolate(child); err != nil {
			return err
		}
	}
	return nil
}

// check fills the defaults and refuses what can't work.
func (c *Config) check() error {
	// The first refusal, in the order of Sources.
	if err := cmp.Or(
		add(c, "gatus", c.Sources.Gatus, func(g gatus.Config) string { return g.Name }, gatus.Read),
		add(c, "github", c.Sources.GitHub, func(g github.Config) string { return g.Name }, github.Read),
		add(c, "immich", c.Sources.Immich, func(im immich.Config) string { return im.Name }, immich.Read),
		add(c, "pbs", c.Sources.PBS, func(b pbs.Config) string { return b.Name }, pbs.Read),
		add(c, "proxmox", c.Sources.Proxmox, func(p proxmox.Config) string { return p.Name }, proxmox.Read),
		add(c, "radarr", c.Sources.Radarr, func(r radarr.Config) string { return r.Name }, radarr.Read),
		add(c, "sonarr", c.Sources.Sonarr, func(s sonarr.Config) string { return s.Name }, sonarr.Read),
		add(c, "nut", c.Sources.NUT, func(n nut.Config) string { return n.Name }, nut.Read),
		add(c, "adguard", c.Sources.AdGuard, func(a adguard.Config) string { return a.Name }, adguard.Read),
		add(c, "ical", c.Sources.Ical, func(i ical.Config) string { return i.Name }, ical.Read),
	); err != nil {
		return err
	}
	for _, list := range []struct {
		name  string
		links []snapshot.Link
	}{{"apps", c.Apps}, {"tools", c.Tools}} {
		for i, l := range list.links {
			if l.Name == "" {
				return fmt.Errorf("%s[%d]: name is missing", list.name, i)
			}
			if err := fetch.CheckURL(l.URL); err != nil {
				return fmt.Errorf("%s[%d]: url %w", list.name, i, err)
			}
			if l.Color != "" && !color.MatchString(l.Color) {
				return fmt.Errorf("%s[%d]: color %q isn't #rgb or #rrggbb", list.name, i, l.Color)
			}
			if l.Check != "" && len(c.Sources.Gatus) == 0 {
				return fmt.Errorf("%s[%d]: check %q, but no check engine is configured", list.name, i, l.Check)
			}
			// Engine/Check names the check engine, when Engine is one: a
			// check's own name may hold a slash.
			engine, name, ok := strings.Cut(l.Check, "/")
			if ok && slices.ContainsFunc(c.Sources.Gatus, func(g gatus.Config) bool { return g.Name == engine }) {
				list.links[i].Engine, list.links[i].Check = engine, name
			}
		}
	}
	return nil
}
