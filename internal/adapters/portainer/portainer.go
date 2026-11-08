// Package portainer reads Portainer: each environment it manages, up or
// down, and the containers running, stopped and unhealthy and the stacks on
// each, from the snapshot Portainer keeps of it, and whether a newer
// Portainer is out. Everything it gives goes in Servers, where its containers
// add up with the Docker adapter's.
package portainer

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/docker"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "servers"

// Config is one Portainer, under sources.portainer in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Portainer, such as https://portainer:9443.
	URL string `yaml:"url" setup:"text" example:"https://portainer.example.com:9443"`
	// Key is an access token of a user of Overlook's own, from My account.
	Key string `yaml:"key" setup:"key"`
	// CA is a PEM file of Portainer's certificate, which signs itself, when
	// the system doesn't trust it, and ServerName the name it is for,
	// localhost for Portainer's own, when URL reaches it by another.
	CA         string `yaml:"ca" setup:"text,optional" example:"/etc/overlook/portainer.pem"`
	ServerName string `yaml:"server_name" setup:"text,optional" example:"localhost"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, CA: c.CA, ServerName: c.ServerName, Header: http.Header{"X-Api-Key": {c.Key}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Key == "" {
		return errors.New("key is missing")
	}
	c.Name = cmp.Or(c.Name, "Portainer")
	return nil
}

// environment is one of /api/endpoints, which lists only those the key's
// user may see. Only this list is read, never a container through
// Portainer's way to Docker, whose answer holds its environment and so its
// keys, as the Docker adapter never reads one.
type environment struct {
	Name string `json:"Name"`
	// Type is Portainer's: 1 Docker's API, 2 an agent and 4 an Edge agent on
	// Docker; the others, Kubernetes and Azure, count no containers.
	Type int `json:"Type"`
	// Status is 1 up and 2 down, as Portainer last found it, but for an Edge
	// agent, which calls Portainer, and is up while Heartbeat says it called
	// lately.
	Status    int  `json:"Status"`
	Heartbeat bool `json:"Heartbeat"`
	// Snapshots hold the last look Portainer took at a Docker environment.
	Snapshots []look `json:"Snapshots"`
}

// look is Portainer's snapshot of a Docker environment, taken every five
// minutes unless the owner changed it, which a down one keeps from when it
// was last up. Its stacks are those of Docker's compose and swarm, whoever
// started them, not only Portainer's.
type look struct {
	// Time is when it was taken, in seconds.
	Time       int64 `json:"Time"`
	Containers int   `json:"ContainerCount"`
	Running    int   `json:"RunningContainerCount"`
	Unhealthy  int   `json:"UnhealthyContainerCount"`
	Stacks     int   `json:"StackCount"`
}

// version is /api/system/version: Portainer's own, and the newer one out, if
// any.
type version struct {
	Version string `json:"ServerVersion"`
	Newer   bool   `json:"UpdateAvailable"`
	Latest  string `json:"LatestVersion"`
}

// Read reads the Portainer at c: its environments and its version.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	var envs []environment
	if err := c.source.JSON(ctx, "/api/endpoints", &envs); err != nil {
		return snapshot.Parts{}, err
	}
	var v version
	if err := c.source.JSON(ctx, "/api/system/version", &v); err != nil {
		return snapshot.Parts{}, err
	}
	return parts(envs, v), nil
}

func parts(envs []environment, v version) snapshot.Parts {
	slices.SortFunc(envs, func(a, b environment) int { return cmp.Compare(a.Name, b.Name) })

	// A row each, by name: down in red, with when Portainer last saw it, or
	// its containers running, of all, orange while one is unhealthy, and
	// its stacks. A down one's last snapshot is from before it went down, so
	// its containers don't count.
	rows := []snapshot.Row{}
	dockers, running, all, unhealthy, wrong := 0, 0, 0, 0, false
	for _, e := range envs {
		if e.Name == "" {
			continue
		}
		isDocker := e.Type == 1 || e.Type == 2 || e.Type == 4
		up := e.Status == 1
		if e.Type == 4 || e.Type == 7 {
			up = e.Heartbeat
		}
		if isDocker {
			dockers++
		}
		row := snapshot.Row{Title: snapshot.Text{Name: e.Name}}
		var last *time.Time
		if len(e.Snapshots) > 0 && e.Snapshots[0].Time > 0 {
			at := time.Unix(e.Snapshots[0].Time, 0)
			last = &at
		}
		switch {
		case !up:
			row.Value, row.Tone = snapshot.Msg("portainer.down", nil), "red"
			if last != nil {
				row.Detail = snapshot.Msg("portainer.last_seen", map[string]any{"at": *last})
			}
			wrong = wrong || isDocker
		case last == nil:
			row.Value = snapshot.Msg("portainer.up", nil)
		default:
			s := e.Snapshots[0]
			row.Value = snapshot.Msg("portainer.running", map[string]any{"running": s.Running, "n": s.Containers})
			row.Detail = snapshot.Msg("portainer.stacks", map[string]any{"n": s.Stacks})
			if s.Unhealthy > 0 {
				row.Tone = "orange"
				row.Detail = snapshot.Msg("portainer.unhealthy_stacks", map[string]any{"unhealthy": s.Unhealthy, "n": s.Stacks})
			}
			running, all, unhealthy = running+s.Running, all+s.Containers, unhealthy+s.Unhealthy
		}
		rows = append(rows, row)
	}

	// Its version, or the newer one out in orange.
	if v.Version != "" {
		newer := ""
		if v.Newer {
			newer = v.Latest
		}
		rows = append(rows, snapshot.VersionRow(v.Version, newer))
	}

	p := snapshot.Parts{Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "portainer.environments"}, Rows: rows}}}
	// Today: the containers running, which add up with every Docker
	// engine's, orange while one is unhealthy or an environment is down.
	if dockers > 0 {
		p.Figures = []snapshot.Figure{docker.Containers(running, all, unhealthy, wrong || unhealthy > 0)}
	}
	return p
}
