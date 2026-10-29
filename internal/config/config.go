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
	"github.com/bnlqn/overlook/internal/adapters/beszel"
	"github.com/bnlqn/overlook/internal/adapters/docker"
	"github.com/bnlqn/overlook/internal/adapters/gatus"
	"github.com/bnlqn/overlook/internal/adapters/github"
	"github.com/bnlqn/overlook/internal/adapters/grafana"
	"github.com/bnlqn/overlook/internal/adapters/homeassistant"
	"github.com/bnlqn/overlook/internal/adapters/ical"
	"github.com/bnlqn/overlook/internal/adapters/immich"
	"github.com/bnlqn/overlook/internal/adapters/jellyfin"
	"github.com/bnlqn/overlook/internal/adapters/nextcloud"
	"github.com/bnlqn/overlook/internal/adapters/nut"
	"github.com/bnlqn/overlook/internal/adapters/paperless"
	"github.com/bnlqn/overlook/internal/adapters/pbs"
	"github.com/bnlqn/overlook/internal/adapters/pihole"
	"github.com/bnlqn/overlook/internal/adapters/prowlarr"
	"github.com/bnlqn/overlook/internal/adapters/proxmox"
	"github.com/bnlqn/overlook/internal/adapters/qbittorrent"
	"github.com/bnlqn/overlook/internal/adapters/radarr"
	"github.com/bnlqn/overlook/internal/adapters/sonarr"
	"github.com/bnlqn/overlook/internal/adapters/tautulli"
	"github.com/bnlqn/overlook/internal/adapters/uptimekuma"
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
	Gatus         []gatus.Config         `yaml:"gatus"`
	UptimeKuma    []uptimekuma.Config    `yaml:"uptimekuma"`
	Grafana       []grafana.Config       `yaml:"grafana"`
	GitHub        []github.Config        `yaml:"github"`
	Immich        []immich.Config        `yaml:"immich"`
	PBS           []pbs.Config           `yaml:"pbs"`
	Proxmox       []proxmox.Config       `yaml:"proxmox"`
	Beszel        []beszel.Config        `yaml:"beszel"`
	Docker        []docker.Config        `yaml:"docker"`
	Radarr        []radarr.Config        `yaml:"radarr"`
	Sonarr        []sonarr.Config        `yaml:"sonarr"`
	Prowlarr      []prowlarr.Config      `yaml:"prowlarr"`
	QBittorrent   []qbittorrent.Config   `yaml:"qbittorrent"`
	Jellyfin      []jellyfin.Config      `yaml:"jellyfin"`
	Tautulli      []tautulli.Config      `yaml:"tautulli"`
	HomeAssistant []homeassistant.Config `yaml:"homeassistant"`
	Paperless     []paperless.Config     `yaml:"paperless"`
	Nextcloud     []nextcloud.Config     `yaml:"nextcloud"`
	NUT           []nut.Config           `yaml:"nut"`
	AdGuard       []adguard.Config       `yaml:"adguard"`
	PiHole        []pihole.Config        `yaml:"pihole"`
	Ical          []ical.Config          `yaml:"ical"`
}

// Readers are the sources, each with its adapter's read, as Load checked
// them.
func (c Config) Readers() []snapshot.Source { return c.readers }

// add checks the sources under sources.<key>, which list holds, and keeps
// each with its adapter's read and home area, if it has one. Each source's
// name must be its own.
func add[C any, P interface {
	*C
	Check() error
}](c *Config, key, area string, list []C, name func(C) string, read func(context.Context, C) (snapshot.Parts, error)) error {
	for i := range list {
		if err := P(&list[i]).Check(); err != nil {
			return fmt.Errorf("sources.%s[%d]: %w", key, i, err)
		}
		src, n := list[i], name(list[i])
		if slices.ContainsFunc(c.readers, func(s snapshot.Source) bool { return s.Name == n }) {
			return fmt.Errorf("sources.%s[%d]: another source is named %s: give each a name", key, i, n)
		}
		c.readers = append(c.readers, snapshot.Source{Name: n, Adapter: key, Area: area, Read: func(ctx context.Context) (snapshot.Parts, error) { return read(ctx, src) }})
	}
	return nil
}

// checkEngines are the adapters whose sources run the homelab's checks,
// which a link's check names.
var checkEngines = []string{"gatus", "uptimekuma", "grafana"}

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
		add(c, "gatus", "", c.Sources.Gatus, func(g gatus.Config) string { return g.Name }, gatus.Read),
		add(c, "uptimekuma", "", c.Sources.UptimeKuma, func(u uptimekuma.Config) string { return u.Name }, uptimekuma.Read),
		add(c, "grafana", "", c.Sources.Grafana, func(g grafana.Config) string { return g.Name }, grafana.Read),
		add(c, "github", "", c.Sources.GitHub, func(g github.Config) string { return g.Name }, github.Read),
		add(c, "immich", immich.Area, c.Sources.Immich, func(im immich.Config) string { return im.Name }, immich.Read),
		add(c, "pbs", pbs.Area, c.Sources.PBS, func(b pbs.Config) string { return b.Name }, pbs.Read),
		add(c, "proxmox", proxmox.Area, c.Sources.Proxmox, func(p proxmox.Config) string { return p.Name }, proxmox.Read),
		add(c, "beszel", beszel.Area, c.Sources.Beszel, func(b beszel.Config) string { return b.Name }, beszel.Read),
		add(c, "docker", docker.Area, c.Sources.Docker, func(d docker.Config) string { return d.Name }, docker.Read),
		add(c, "radarr", radarr.Area, c.Sources.Radarr, func(r radarr.Config) string { return r.Name }, radarr.Read),
		add(c, "sonarr", sonarr.Area, c.Sources.Sonarr, func(s sonarr.Config) string { return s.Name }, sonarr.Read),
		add(c, "prowlarr", prowlarr.Area, c.Sources.Prowlarr, func(p prowlarr.Config) string { return p.Name }, prowlarr.Read),
		add(c, "qbittorrent", qbittorrent.Area, c.Sources.QBittorrent, func(q qbittorrent.Config) string { return q.Name }, qbittorrent.Read),
		add(c, "jellyfin", jellyfin.Area, c.Sources.Jellyfin, func(j jellyfin.Config) string { return j.Name }, jellyfin.Read),
		add(c, "tautulli", tautulli.Area, c.Sources.Tautulli, func(t tautulli.Config) string { return t.Name }, tautulli.Read),
		add(c, "homeassistant", homeassistant.Area, c.Sources.HomeAssistant, func(h homeassistant.Config) string { return h.Name }, homeassistant.Read),
		add(c, "paperless", paperless.Area, c.Sources.Paperless, func(p paperless.Config) string { return p.Name }, paperless.Read),
		add(c, "nextcloud", nextcloud.Area, c.Sources.Nextcloud, func(n nextcloud.Config) string { return n.Name }, nextcloud.Read),
		add(c, "nut", nut.Area, c.Sources.NUT, func(n nut.Config) string { return n.Name }, nut.Read),
		add(c, "adguard", adguard.Area, c.Sources.AdGuard, func(a adguard.Config) string { return a.Name }, adguard.Read),
		add(c, "pihole", pihole.Area, c.Sources.PiHole, func(p pihole.Config) string { return p.Name }, pihole.Read),
		add(c, "ical", "", c.Sources.Ical, func(i ical.Config) string { return i.Name }, ical.Read),
	); err != nil {
		return err
	}
	var engines []string
	for _, r := range c.readers {
		if slices.Contains(checkEngines, r.Adapter) {
			engines = append(engines, r.Name)
		}
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
			if l.Check != "" && len(engines) == 0 {
				return fmt.Errorf("%s[%d]: check %q, but no check engine is configured", list.name, i, l.Check)
			}
			// Engine/Check names the check engine, when Engine is one: a
			// check's own name may hold a slash.
			engine, name, ok := strings.Cut(l.Check, "/")
			if ok && slices.Contains(engines, engine) {
				list.links[i].Engine, list.links[i].Check = engine, name
			}
		}
	}
	return nil
}
