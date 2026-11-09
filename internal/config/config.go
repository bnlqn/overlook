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
	"github.com/bnlqn/overlook/internal/adapters/audiobookshelf"
	"github.com/bnlqn/overlook/internal/adapters/beszel"
	"github.com/bnlqn/overlook/internal/adapters/checkmate"
	"github.com/bnlqn/overlook/internal/adapters/docker"
	"github.com/bnlqn/overlook/internal/adapters/emby"
	"github.com/bnlqn/overlook/internal/adapters/forgejo"
	"github.com/bnlqn/overlook/internal/adapters/gatus"
	"github.com/bnlqn/overlook/internal/adapters/gitea"
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
	"github.com/bnlqn/overlook/internal/adapters/portainer"
	"github.com/bnlqn/overlook/internal/adapters/prometheus"
	"github.com/bnlqn/overlook/internal/adapters/prowlarr"
	"github.com/bnlqn/overlook/internal/adapters/proxmox"
	"github.com/bnlqn/overlook/internal/adapters/qbittorrent"
	"github.com/bnlqn/overlook/internal/adapters/radarr"
	"github.com/bnlqn/overlook/internal/adapters/seerr"
	"github.com/bnlqn/overlook/internal/adapters/sonarr"
	"github.com/bnlqn/overlook/internal/adapters/syncthing"
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
// An adapter's key is written twice, in its tag and in its package's
// Adapter, on purpose: the strict decoder needs a field for it, whose key
// is its tag, and decoding sources: by the adapters' keys instead would
// take reflection, or lose yaml's own errors, since a yaml node doesn't
// decode strictly. TestSources, in setup_test.go, holds each field's tag,
// type and place to its adapter's. Its icon:
// tag is its logo on the setup page, a file in web/icons that the server's
// test checks; an iCalendar list, served by many programs, has none.
type Sources struct {
	Gatus          []gatus.Config          `yaml:"gatus" icon:"gatus.svg"`
	UptimeKuma     []uptimekuma.Config     `yaml:"uptimekuma" icon:"uptime-kuma.svg"`
	Grafana        []grafana.Config        `yaml:"grafana" icon:"grafana.svg"`
	Prometheus     []prometheus.Config     `yaml:"prometheus" icon:"prometheus.svg"`
	Checkmate      []checkmate.Config      `yaml:"checkmate" icon:"checkmate.svg"`
	GitHub         []github.Config         `yaml:"github" icon:"github.svg"`
	Immich         []immich.Config         `yaml:"immich" icon:"immich.svg"`
	PBS            []pbs.Config            `yaml:"pbs" icon:"proxmox-backup-server.svg"`
	Syncthing      []syncthing.Config      `yaml:"syncthing" icon:"syncthing.svg"`
	Proxmox        []proxmox.Config        `yaml:"proxmox" icon:"proxmox.svg"`
	Beszel         []beszel.Config         `yaml:"beszel" icon:"beszel.svg"`
	Docker         []docker.Config         `yaml:"docker" icon:"docker.svg"`
	Portainer      []portainer.Config      `yaml:"portainer" icon:"portainer.svg"`
	Radarr         []radarr.Config         `yaml:"radarr" icon:"radarr.svg"`
	Sonarr         []sonarr.Config         `yaml:"sonarr" icon:"sonarr.svg"`
	Prowlarr       []prowlarr.Config       `yaml:"prowlarr" icon:"prowlarr.svg"`
	QBittorrent    []qbittorrent.Config    `yaml:"qbittorrent" icon:"qbittorrent.svg"`
	Jellyfin       []jellyfin.Config       `yaml:"jellyfin" icon:"jellyfin.svg"`
	Emby           []emby.Config           `yaml:"emby" icon:"emby.svg"`
	Tautulli       []tautulli.Config       `yaml:"tautulli" icon:"tautulli.svg"`
	Audiobookshelf []audiobookshelf.Config `yaml:"audiobookshelf" icon:"audiobookshelf.svg"`
	Seerr          []seerr.Config          `yaml:"seerr" icon:"seerr.svg"`
	HomeAssistant  []homeassistant.Config  `yaml:"homeassistant" icon:"home-assistant.svg"`
	Paperless      []paperless.Config      `yaml:"paperless" icon:"paperless-ngx.svg"`
	Nextcloud      []nextcloud.Config      `yaml:"nextcloud" icon:"nextcloud.svg"`
	Gitea          []gitea.Config          `yaml:"gitea" icon:"gitea.svg"`
	Forgejo        []forgejo.Config        `yaml:"forgejo" icon:"forgejo.svg"`
	NUT            []nut.Config            `yaml:"nut" icon:"network-ups-tools.svg"`
	UniFi          []unifi.Config          `yaml:"unifi" icon:"unifi.png"`
	AdGuard        []adguard.Config        `yaml:"adguard" icon:"adguard-home.svg"`
	PiHole         []pihole.Config         `yaml:"pihole" icon:"pi-hole.svg"`
	Ical           []ical.Config           `yaml:"ical"`
}

// Readers are the sources, each with its adapter's read, as Load checked
// them.
func (c Config) Readers() []snapshot.Source { return c.readers }

// add checks the sources under sources.<a.Key>, which list holds, and keeps
// each with a's read and what a says it is. Each source's name must be its
// own.
func add[C any, P interface {
	*C
	Check() error
}](c *Config, a snapshot.Adapter[C], list []C) error {
	c.adapters = append(c.adapters, Adapter{Key: a.Key, Area: a.Area, Kind: a.Kind, Storage: a.Storage, config: reflect.TypeFor[C]()})
	for i := range list {
		if err := P(&list[i]).Check(); err != nil {
			return fmt.Errorf("sources.%s[%d]: %w", a.Key, i, err)
		}
		src, n := list[i], name(reflect.ValueOf(list[i]))
		if slices.ContainsFunc(c.readers, func(s snapshot.Source) bool { return s.Name == n }) {
			return fmt.Errorf("sources.%s[%d]: another source is named %s: give each a name", a.Key, i, n)
		}
		c.readers = append(c.readers, snapshot.Source{Name: n, Adapter: a.Key, Area: a.Area, Kind: a.Kind, Storage: a.Storage, Read: func(ctx context.Context, now time.Time) (snapshot.Parts, error) { return a.Read(ctx, src, now) }})
	}
	return nil
}

// name is the name of the source configured as v, once its Check has filled
// the default: its field tagged yaml:"name", or that of the inline config
// that holds it, as servarr.Config does for Radarr. It is found by its tag,
// as the setup page finds every field it asks for, because it is the same
// field in every adapter's config, where TestName, in setup_test.go, reads
// it.
func name(v reflect.Value) string {
	for i := range v.NumField() {
		f := v.Type().Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if n := name(v.Field(i)); n != "" {
				return n
			}
			continue
		}
		if strings.Split(f.Tag.Get("yaml"), ",")[0] == "name" {
			return v.Field(i).String()
		}
	}
	return ""
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
	// The first refusal, in the order of Sources, which is the page's: each
	// area's sections, figures and events come in it, and the setup page
	// offers the adapters in it.
	if err := cmp.Or(
		add(c, gatus.Adapter, c.Sources.Gatus),
		add(c, uptimekuma.Adapter, c.Sources.UptimeKuma),
		add(c, grafana.Adapter, c.Sources.Grafana),
		add(c, prometheus.Adapter, c.Sources.Prometheus),
		add(c, checkmate.Adapter, c.Sources.Checkmate),
		add(c, github.Adapter, c.Sources.GitHub),
		add(c, immich.Adapter, c.Sources.Immich),
		add(c, pbs.Adapter, c.Sources.PBS),
		add(c, syncthing.Adapter, c.Sources.Syncthing),
		add(c, proxmox.Adapter, c.Sources.Proxmox),
		add(c, beszel.Adapter, c.Sources.Beszel),
		add(c, docker.Adapter, c.Sources.Docker),
		add(c, portainer.Adapter, c.Sources.Portainer),
		add(c, radarr.Adapter, c.Sources.Radarr),
		add(c, sonarr.Adapter, c.Sources.Sonarr),
		add(c, prowlarr.Adapter, c.Sources.Prowlarr),
		add(c, qbittorrent.Adapter, c.Sources.QBittorrent),
		add(c, jellyfin.Adapter, c.Sources.Jellyfin),
		add(c, emby.Adapter, c.Sources.Emby),
		add(c, tautulli.Adapter, c.Sources.Tautulli),
		add(c, audiobookshelf.Adapter, c.Sources.Audiobookshelf),
		add(c, seerr.Adapter, c.Sources.Seerr),
		add(c, homeassistant.Adapter, c.Sources.HomeAssistant),
		add(c, paperless.Adapter, c.Sources.Paperless),
		add(c, nextcloud.Adapter, c.Sources.Nextcloud),
		add(c, gitea.Adapter, c.Sources.Gitea),
		add(c, forgejo.Adapter, c.Sources.Forgejo),
		add(c, nut.Adapter, c.Sources.NUT),
		add(c, unifi.Adapter, c.Sources.UniFi),
		add(c, adguard.Adapter, c.Sources.AdGuard),
		add(c, pihole.Adapter, c.Sources.PiHole),
		add(c, ical.Adapter, c.Sources.Ical),
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
