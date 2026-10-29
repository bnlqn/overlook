// Package docker reads a Docker engine through a socket proxy: each
// container, whether it runs, and those that exited, restart or fail their
// health check. Everything it gives goes in Servers.
package docker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "servers"

// api is the version of Docker's API read, which Docker 25 and later speak:
// asking for it keeps the answer's shape whatever the engine's version.
const api = "/v1.44"

// Config is one Docker engine, under sources.docker in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Docker's API: a socket proxy that lets
	// through only the containers' reads, such as http://socket-proxy:2375.
	// Overlook reads over HTTP only, never from the socket itself.
	URL string `yaml:"url"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Name == "" {
		c.Name = "Docker"
	}
	return nil
}

// container is one line of /containers/json. Only the list is read, never a
// container's own answer, which holds its environment and so its keys.
type container struct {
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	// State is running, exited, restarting, paused, created, dead or
	// removing.
	State string `json:"State"`
	// Status is Docker's line for it, such as "Up 2 hours (unhealthy)" or
	// "Exited (1) 3 hours ago", which alone gives its health and exit code.
	Status string `json:"Status"`
}

// Read reads the Docker engine at c: every container, the stopped ones too.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	var all []container
	if err := fetch.JSON(ctx, strings.TrimSuffix(c.URL, "/")+api+"/containers/json?all=1", nil, &all); err != nil {
		return snapshot.Parts{}, err
	}
	return parts(all), nil
}

// exited is the exit code in a stopped container's status.
var exited = regexp.MustCompile(`^Exited \((-?\d+)\)`)

func parts(all []container) snapshot.Parts {
	name := func(c container) string {
		if len(c.Names) == 0 {
			return ""
		}
		return strings.TrimPrefix(c.Names[0], "/")
	}
	slices.SortFunc(all, func(a, b container) int { return cmp.Compare(name(a), name(b)) })

	// A row each, by name: its image without its digest, and whether it
	// runs, in red where it went wrong. One that exited cleanly, as a job
	// that has done its work does, is only grey.
	rows := []snapshot.Row{}
	running, unhealthy, wrong := 0, 0, false
	for _, c := range all {
		image, _, _ := strings.Cut(c.Image, "@")
		row := snapshot.Row{Title: snapshot.Text{Name: name(c)}, Detail: &snapshot.Text{Name: image}}
		value := func(key string, args map[string]any, tone string) {
			row.Value, row.Tone = snapshot.Msg(key, args), tone
			wrong = wrong || tone == "red" || tone == "orange"
		}
		if c.State == "running" {
			running++
		}
		switch {
		case c.State == "running" && strings.HasSuffix(c.Status, "(unhealthy)"):
			unhealthy++
			value("docker.unhealthy", nil, "red")
		case c.State == "running" && strings.HasSuffix(c.Status, "(health: starting)"):
			value("docker.starting", nil, "blue")
		case c.State == "running" && strings.HasSuffix(c.Status, "(healthy)"):
			value("docker.healthy", nil, "")
		case c.State == "running":
			value("docker.running", nil, "")
		case c.State == "exited":
			code, tone := 0, "red"
			if m := exited.FindStringSubmatch(c.Status); m != nil {
				code, _ = strconv.Atoi(m[1])
			}
			if code == 0 {
				tone = "gray"
			}
			value("docker.exited", map[string]any{"code": code}, tone)
		case c.State == "restarting":
			value("docker.restarting", nil, "orange")
		case c.State == "dead":
			value("docker.dead", nil, "red")
		case c.State == "paused":
			value("docker.paused", nil, "gray")
		case c.State == "created":
			value("docker.created", nil, "gray")
		default:
			row.Value, row.Tone = &snapshot.Text{Name: c.State}, "gray"
		}
		rows = append(rows, row)
	}

	// Today: the containers running, which adds up with another engine's.
	f := snapshot.Figure{
		Area: Area, Icon: "box", Label: snapshot.Text{Key: "docker.containers"}, Value: float64(running),
		Unit:   snapshot.Msg("docker.of_running", map[string]any{"n": len(all)}),
		Detail: snapshot.Msg("docker.unhealthy_n", map[string]any{"n": unhealthy}),
		Adds:   true,
	}
	if wrong {
		f.Tone = "orange"
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "docker.containers"}, Rows: rows}},
		Figures:  []snapshot.Figure{f},
	}
}
