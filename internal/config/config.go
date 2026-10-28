// Package config reads overlook.yaml: the sources to read, and the apps and
// tools to link to. A key it doesn't know is an error, so a typo never goes
// unnoticed, and a ${VAR} comes from the environment.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"go.yaml.in/yaml/v3"

	"github.com/bnlqn/overlook/internal/adapters/adguard"
	"github.com/bnlqn/overlook/internal/adapters/gatus"
	"github.com/bnlqn/overlook/internal/adapters/github"
	"github.com/bnlqn/overlook/internal/adapters/immich"
	"github.com/bnlqn/overlook/internal/adapters/nut"
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
}

// Sources are the sources to read, by adapter: each adapter's own config.
type Sources struct {
	Gatus   []gatus.Config   `yaml:"gatus"`
	GitHub  []github.Config  `yaml:"github"`
	Immich  []immich.Config  `yaml:"immich"`
	Radarr  []radarr.Config  `yaml:"radarr"`
	Sonarr  []sonarr.Config  `yaml:"sonarr"`
	NUT     []nut.Config     `yaml:"nut"`
	AdGuard []adguard.Config `yaml:"adguard"`
}

// Readers are the sources, each with its adapter's read.
func (c Config) Readers() []snapshot.Source {
	var all []snapshot.Source
	for _, g := range c.Sources.Gatus {
		all = append(all, snapshot.Source{Name: g.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return gatus.Read(ctx, g) }})
	}
	for _, g := range c.Sources.GitHub {
		all = append(all, snapshot.Source{Name: g.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return github.Read(ctx, g) }})
	}
	for _, im := range c.Sources.Immich {
		all = append(all, snapshot.Source{Name: im.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return immich.Read(ctx, im) }})
	}
	for _, r := range c.Sources.Radarr {
		all = append(all, snapshot.Source{Name: r.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return radarr.Read(ctx, r) }})
	}
	for _, s := range c.Sources.Sonarr {
		all = append(all, snapshot.Source{Name: s.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return sonarr.Read(ctx, s) }})
	}
	for _, n := range c.Sources.NUT {
		all = append(all, snapshot.Source{Name: n.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return nut.Read(ctx, n) }})
	}
	for _, a := range c.Sources.AdGuard {
		all = append(all, snapshot.Source{Name: a.Name, Read: func(ctx context.Context) (snapshot.Parts, error) { return adguard.Read(ctx, a) }})
	}
	return all
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
	names := map[string]bool{}
	for i := range c.Sources.Gatus {
		g := &c.Sources.Gatus[i]
		if err := g.Check(); err != nil {
			return fmt.Errorf("sources.gatus[%d]: %w", i, err)
		}
		if names[g.Name] {
			return fmt.Errorf("sources.gatus[%d]: another source is named %s: give each a name", i, g.Name)
		}
		names[g.Name] = true
	}
	for i := range c.Sources.GitHub {
		g := &c.Sources.GitHub[i]
		if err := g.Check(); err != nil {
			return fmt.Errorf("sources.github[%d]: %w", i, err)
		}
		if names[g.Name] {
			return fmt.Errorf("sources.github[%d]: another source is named %s: give each a name", i, g.Name)
		}
		names[g.Name] = true
	}
	for i := range c.Sources.Immich {
		im := &c.Sources.Immich[i]
		if err := im.Check(); err != nil {
			return fmt.Errorf("sources.immich[%d]: %w", i, err)
		}
		if names[im.Name] {
			return fmt.Errorf("sources.immich[%d]: another source is named %s: give each a name", i, im.Name)
		}
		names[im.Name] = true
	}
	for i := range c.Sources.Radarr {
		r := &c.Sources.Radarr[i]
		if err := r.Check(); err != nil {
			return fmt.Errorf("sources.radarr[%d]: %w", i, err)
		}
		if names[r.Name] {
			return fmt.Errorf("sources.radarr[%d]: another source is named %s: give each a name", i, r.Name)
		}
		names[r.Name] = true
	}
	for i := range c.Sources.Sonarr {
		s := &c.Sources.Sonarr[i]
		if err := s.Check(); err != nil {
			return fmt.Errorf("sources.sonarr[%d]: %w", i, err)
		}
		if names[s.Name] {
			return fmt.Errorf("sources.sonarr[%d]: another source is named %s: give each a name", i, s.Name)
		}
		names[s.Name] = true
	}
	for i := range c.Sources.NUT {
		n := &c.Sources.NUT[i]
		if err := n.Check(); err != nil {
			return fmt.Errorf("sources.nut[%d]: %w", i, err)
		}
		if names[n.Name] {
			return fmt.Errorf("sources.nut[%d]: another source is named %s: give each a name", i, n.Name)
		}
		names[n.Name] = true
	}
	for i := range c.Sources.AdGuard {
		a := &c.Sources.AdGuard[i]
		if err := a.Check(); err != nil {
			return fmt.Errorf("sources.adguard[%d]: %w", i, err)
		}
		if names[a.Name] {
			return fmt.Errorf("sources.adguard[%d]: another source is named %s: give each a name", i, a.Name)
		}
		names[a.Name] = true
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
		}
	}
	return nil
}
