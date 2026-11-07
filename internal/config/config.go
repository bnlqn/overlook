// Package config reads overlook.yaml: the sources to read, and the apps and
// tools to link to. A key it doesn't know is an error, so a typo never goes unnoticed, and a ${VAR} comes from
// the environment, or from the .env beside the config.
package config

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/bnlqn/overlook/internal/adapters/adguard"
	"github.com/bnlqn/overlook/internal/adapters/beszel"
	"github.com/bnlqn/overlook/internal/adapters/checkmate"
	"github.com/bnlqn/overlook/internal/adapters/docker"
	"github.com/bnlqn/overlook/internal/adapters/emby"
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
	"github.com/bnlqn/overlook/internal/adapters/prometheus"
	"github.com/bnlqn/overlook/internal/adapters/prowlarr"
	"github.com/bnlqn/overlook/internal/adapters/proxmox"
	"github.com/bnlqn/overlook/internal/adapters/qbittorrent"
	"github.com/bnlqn/overlook/internal/adapters/radarr"
	"github.com/bnlqn/overlook/internal/adapters/seerr"
	"github.com/bnlqn/overlook/internal/adapters/sonarr"
	"github.com/bnlqn/overlook/internal/adapters/tautulli"
	"github.com/bnlqn/overlook/internal/adapters/unifi"
	"github.com/bnlqn/overlook/internal/adapters/uptimekuma"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is overlook.yaml.
type Config struct {
	Sources Sources         `yaml:"sources"`
	Apps    []snapshot.Link `yaml:"apps"`
	Tools   []snapshot.Link `yaml:"tools"`
	// MCP says whether agents may read the page through MCP, at /mcp: off
	// unless the owner turns it on, since it hands the homelab's state to a
	// model (ADR 0015).
	MCP bool `yaml:"mcp"`

	// readers are the sources check let through, each with its read.
	readers []snapshot.Source
	// adapters are every adapter, as check met them.
	adapters []Adapter
}

// Sources are the sources to read, by adapter: each adapter's own config.
// An adapter's key is written twice, in its tag and in its add line, on
// purpose: the strict decoder needs a field for it, and reading the key
// from the tag would take reflection, which no compiler checks. Its icon:
// tag is its logo on the setup page, a file in web/icons that the server's
// test checks; an iCalendar list, served by many programs, has none.
type Sources struct {
	Gatus         []gatus.Config         `yaml:"gatus" icon:"gatus.svg"`
	UptimeKuma    []uptimekuma.Config    `yaml:"uptimekuma" icon:"uptime-kuma.svg"`
	Grafana       []grafana.Config       `yaml:"grafana" icon:"grafana.svg"`
	Prometheus    []prometheus.Config    `yaml:"prometheus" icon:"prometheus.svg"`
	Checkmate     []checkmate.Config     `yaml:"checkmate" icon:"checkmate.svg"`
	GitHub        []github.Config        `yaml:"github" icon:"github.svg"`
	Immich        []immich.Config        `yaml:"immich" icon:"immich.svg"`
	PBS           []pbs.Config           `yaml:"pbs" icon:"proxmox-backup-server.svg"`
	Proxmox       []proxmox.Config       `yaml:"proxmox" icon:"proxmox.svg"`
	Beszel        []beszel.Config        `yaml:"beszel" icon:"beszel.svg"`
	Docker        []docker.Config        `yaml:"docker" icon:"docker.svg"`
	Radarr        []radarr.Config        `yaml:"radarr" icon:"radarr.svg"`
	Sonarr        []sonarr.Config        `yaml:"sonarr" icon:"sonarr.svg"`
	Prowlarr      []prowlarr.Config      `yaml:"prowlarr" icon:"prowlarr.svg"`
	QBittorrent   []qbittorrent.Config   `yaml:"qbittorrent" icon:"qbittorrent.svg"`
	Jellyfin      []jellyfin.Config      `yaml:"jellyfin" icon:"jellyfin.svg"`
	Emby          []emby.Config          `yaml:"emby" icon:"emby.svg"`
	Tautulli      []tautulli.Config      `yaml:"tautulli" icon:"tautulli.svg"`
	Seerr         []seerr.Config         `yaml:"seerr" icon:"seerr.svg"`
	HomeAssistant []homeassistant.Config `yaml:"homeassistant" icon:"home-assistant.svg"`
	Paperless     []paperless.Config     `yaml:"paperless" icon:"paperless-ngx.svg"`
	Nextcloud     []nextcloud.Config     `yaml:"nextcloud" icon:"nextcloud.svg"`
	NUT           []nut.Config           `yaml:"nut" icon:"network-ups-tools.svg"`
	UniFi         []unifi.Config         `yaml:"unifi" icon:"unifi.png"`
	AdGuard       []adguard.Config       `yaml:"adguard" icon:"adguard-home.svg"`
	PiHole        []pihole.Config        `yaml:"pihole" icon:"pi-hole.svg"`
	Ical          []ical.Config          `yaml:"ical"`
}

// Readers are the sources, each with its adapter's read, as Load checked
// them.
func (c Config) Readers() []snapshot.Source { return c.readers }

// add checks the sources under sources.<key>, which list holds, and keeps
// each with its adapter's read, and its home area and kind, if it has them.
// Each source's name must be its own.
func add[C any, P interface {
	*C
	Check() error
}](c *Config, key, area, kind string, list []C, name func(C) string, read func(context.Context, C, time.Time) (snapshot.Parts, error)) error {
	c.adapters = append(c.adapters, Adapter{Key: key, Area: area, Kind: kind, config: reflect.TypeFor[C]()})
	for i := range list {
		if err := P(&list[i]).Check(); err != nil {
			return fmt.Errorf("sources.%s[%d]: %w", key, i, err)
		}
		src, n := list[i], name(list[i])
		if slices.ContainsFunc(c.readers, func(s snapshot.Source) bool { return s.Name == n }) {
			return fmt.Errorf("sources.%s[%d]: another source is named %s: give each a name", key, i, n)
		}
		c.readers = append(c.readers, snapshot.Source{Name: n, Adapter: key, Area: area, Kind: kind, Read: func(ctx context.Context, now time.Time) (snapshot.Parts, error) { return read(ctx, src, now) }})
	}
	return nil
}

var (
	envVar  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	color   = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)
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
	raw, err := os.ReadFile(path) //nolint:gosec // the owner names their own config
	if err != nil {
		return Config{}, err
	}
	env, err := dotenv(filepath.Join(filepath.Dir(path), ".env"))
	if err != nil {
		return Config{}, err
	}
	// ${VAR} is replaced in values, after parsing, so that a value can't
	// change the file's structure and a comment can't name a var.
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Config{}, err
	}
	if len(doc.Content) == 0 {
		return Config{}, errors.New("empty")
	}
	if err := interpolate(&doc, env); err != nil {
		return Config{}, err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return Config{}, err
	}
	return decode(out)
}

// decode reads a config with every ${VAR} already replaced, and checks it.
func decode(raw []byte) (Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	return c, c.check()
}

// dotenv reads the .env at path, if there is one: a NAME=value a line, the
// value all that follows the first =, so that a key holding a # or a quote
// stays whole. A line that starts with # is a comment.
func dotenv(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // beside the owner's config
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !envName.MatchString(name) {
			return nil, fmt.Errorf(".env, line %d: isn't NAME=value", i+1)
		}
		env[name] = value
	}
	return env, nil
}

// interpolate replaces each ${VAR} in n's values with VAR from the
// environment, or else from env, the .env's.
func interpolate(n *yaml.Node, env map[string]string) error {
	if n.Kind == yaml.ScalarNode {
		var unset error
		n.Value = envVar.ReplaceAllStringFunc(n.Value, func(m string) string {
			name := envVar.FindStringSubmatch(m)[1]
			v, ok := os.LookupEnv(name)
			if !ok {
				v, ok = env[name]
			}
			if !ok && unset == nil {
				unset = fmt.Errorf("%s is not set", name)
			}
			return v
		})
		return unset
	}
	for _, child := range n.Content {
		if err := interpolate(child, env); err != nil {
			return err
		}
	}
	return nil
}

// check fills the defaults and refuses what can't work.
func (c *Config) check() error {
	// The first refusal, in the order of Sources.
	if err := cmp.Or(
		add(c, "gatus", "", "checkengine", c.Sources.Gatus, func(g gatus.Config) string { return g.Name }, gatus.Read),
		add(c, "uptimekuma", "", "checkengine", c.Sources.UptimeKuma, func(u uptimekuma.Config) string { return u.Name }, uptimekuma.Read),
		add(c, "grafana", "", "checkengine", c.Sources.Grafana, func(g grafana.Config) string { return g.Name }, grafana.Read),
		add(c, "prometheus", "", "checkengine", c.Sources.Prometheus, func(p prometheus.Config) string { return p.Name }, prometheus.Read),
		add(c, "checkmate", "", "checkengine", c.Sources.Checkmate, func(m checkmate.Config) string { return m.Name }, checkmate.Read),
		add(c, "github", "", "", c.Sources.GitHub, func(g github.Config) string { return g.Name }, github.Read),
		add(c, "immich", immich.Area, "", c.Sources.Immich, func(im immich.Config) string { return im.Name }, immich.Read),
		add(c, "pbs", pbs.Area, "", c.Sources.PBS, func(b pbs.Config) string { return b.Name }, pbs.Read),
		add(c, "proxmox", proxmox.Area, "", c.Sources.Proxmox, func(p proxmox.Config) string { return p.Name }, proxmox.Read),
		add(c, "beszel", beszel.Area, "", c.Sources.Beszel, func(b beszel.Config) string { return b.Name }, beszel.Read),
		add(c, "docker", docker.Area, "", c.Sources.Docker, func(d docker.Config) string { return d.Name }, docker.Read),
		add(c, "radarr", radarr.Area, "", c.Sources.Radarr, func(r radarr.Config) string { return r.Name }, radarr.Read),
		add(c, "sonarr", sonarr.Area, "", c.Sources.Sonarr, func(s sonarr.Config) string { return s.Name }, sonarr.Read),
		add(c, "prowlarr", prowlarr.Area, "", c.Sources.Prowlarr, func(p prowlarr.Config) string { return p.Name }, prowlarr.Read),
		add(c, "qbittorrent", qbittorrent.Area, "", c.Sources.QBittorrent, func(q qbittorrent.Config) string { return q.Name }, qbittorrent.Read),
		add(c, "jellyfin", jellyfin.Area, "mediaserver", c.Sources.Jellyfin, func(j jellyfin.Config) string { return j.Name }, jellyfin.Read),
		add(c, "emby", emby.Area, "mediaserver", c.Sources.Emby, func(e emby.Config) string { return e.Name }, emby.Read),
		add(c, "tautulli", tautulli.Area, "mediaserver", c.Sources.Tautulli, func(t tautulli.Config) string { return t.Name }, tautulli.Read),
		add(c, "seerr", seerr.Area, "", c.Sources.Seerr, func(s seerr.Config) string { return s.Name }, seerr.Read),
		add(c, "homeassistant", homeassistant.Area, "", c.Sources.HomeAssistant, func(h homeassistant.Config) string { return h.Name }, homeassistant.Read),
		add(c, "paperless", paperless.Area, "", c.Sources.Paperless, func(p paperless.Config) string { return p.Name }, paperless.Read),
		add(c, "nextcloud", nextcloud.Area, "", c.Sources.Nextcloud, func(n nextcloud.Config) string { return n.Name }, nextcloud.Read),
		add(c, "nut", nut.Area, "", c.Sources.NUT, func(n nut.Config) string { return n.Name }, nut.Read),
		add(c, "unifi", unifi.Area, "", c.Sources.UniFi, func(u unifi.Config) string { return u.Name }, unifi.Read),
		add(c, "adguard", adguard.Area, "resolver", c.Sources.AdGuard, func(a adguard.Config) string { return a.Name }, adguard.Read),
		add(c, "pihole", pihole.Area, "resolver", c.Sources.PiHole, func(p pihole.Config) string { return p.Name }, pihole.Read),
		add(c, "ical", "", "todolist", c.Sources.Ical, func(i ical.Config) string { return i.Name }, ical.Read),
	); err != nil {
		return err
	}
	// The check engines, which a link's check names.
	var engines []string
	for _, r := range c.readers {
		if r.Kind == "checkengine" {
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
