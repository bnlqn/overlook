// Package tdarr reads Tdarr, a transcoding server: the files its nodes are
// working on now, each with how far along it is, and how many files wait. It
// gives a section only while a node works on something, since an idle Tdarr
// has nothing to say that a check engine's answer doesn't.
package tdarr

import (
	"cmp"
	"context"
	"math"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "tdarr", Area: Area, Read: Read}

// Config is one Tdarr, under sources.tdarr in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Tdarr's server, such as
	// http://tdarr:8265.
	URL string `yaml:"url" setup:"text" example:"http://tdarr.example.com:8265"`
	// Key is the API key Tdarr asks for when its server has one set, under
	// Tools, Options. With none set, Tdarr answers anyone who reaches it.
	Key string `yaml:"key" setup:"key,optional"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL}
	if c.Key != "" {
		c.source.Header = http.Header{"x-api-key": {c.Key}}
	}
	if err := c.source.Check(); err != nil {
		return err
	}
	c.Name = cmp.Or(c.Name, "Tdarr")
	return nil
}

// node is one of Tdarr's nodes, a machine that transcodes for the server.
type node struct {
	Name         string            `json:"nodeName"`
	Paused       bool              `json:"nodePaused"`
	Workers      map[string]worker `json:"workers"`
	QueueLengths map[string]int    `json:"queueLengths"`
}

// worker is one of a node's workers, which a node has only while it has
// something to do. Its type says what it does: transcodecpu, transcodegpu,
// healthcheckcpu or healthcheckgpu.
type worker struct {
	Type       string  `json:"workerType"`
	Idle       bool    `json:"idle"`
	File       string  `json:"file"`
	Percentage float64 `json:"percentage"`
	FPS        float64 `json:"fps"`
	ETA        string  `json:"ETA"`
}

// Read reads the Tdarr at c, which Check has filled. Tdarr's server answers
// a node's workers and queues to a GET, as its page asks them; the totals it
// keeps of what it has done answer only to a POST, which Overlook doesn't
// send (ADR 0001), so there are none here.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	var nodes map[string]node
	if err := c.source.JSON(ctx, "/api/v2/get-nodes", &nodes); err != nil {
		return snapshot.Parts{}, err
	}

	type job struct {
		node string
		worker
	}
	var jobs []job
	waiting := 0
	for _, n := range nodes {
		for _, w := range n.Workers {
			if !w.Idle && w.File != "" {
				jobs = append(jobs, job{n.Name, w})
			}
		}
		// What waits for this node's workers, whichever of them it is for.
		for _, q := range n.QueueLengths {
			waiting += q
		}
	}
	// Nothing is being worked on: no card.
	if len(jobs) == 0 {
		return snapshot.Parts{}, nil
	}
	slices.SortFunc(jobs, func(a, b job) int {
		return cmp.Or(cmp.Compare(a.node, b.node), cmp.Compare(a.File, b.File))
	})

	rows := make([]snapshot.Row, 0, len(jobs)+1)
	var done float64
	for _, j := range jobs {
		key := "tdarr.transcoding"
		if strings.HasPrefix(j.Type, "healthcheck") {
			key = "tdarr.checking"
		}
		percent := math.Round(min(max(j.Percentage, 0), 100))
		done += percent
		args := map[string]any{"node": j.node, "fps": int(math.Round(j.FPS)), "eta": j.ETA}
		detail := key + "_detail"
		if j.FPS <= 0 || j.ETA == "" {
			detail = key + "_start"
		}
		rows = append(rows, snapshot.Row{
			Title:  snapshot.Text{Name: path.Base(strings.ReplaceAll(j.File, `\`, "/"))},
			Value:  snapshot.Percent(percent),
			Detail: snapshot.Msg(detail, args),
			Meter:  &snapshot.Meter{Used: percent, Total: 100, Tone: "blue"},
		})
	}
	if waiting > 0 {
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Key: "tdarr.waiting"}, Value: snapshot.Msg("tdarr.waiting_n", map[string]any{"n": waiting}),
		})
	}
	// Today: the files in work, with how far along they are on average. Like
	// the section, it is there only while there is work.
	f := snapshot.Figure{
		Area: Area, Icon: "refresh", Label: snapshot.Text{Key: "tdarr.title"}, Value: float64(len(jobs)),
		Unit:   snapshot.Msg("tdarr.unit", nil),
		Detail: snapshot.Msg("tdarr.figure_detail", map[string]any{"n": waiting}),
		Meters: []snapshot.Meter{{Label: &snapshot.Text{Key: "tdarr.progress"}, Used: math.Round(done / float64(len(jobs))), Total: 100, Tone: "blue"}},
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "tdarr.title"}, Rows: rows}},
		Figures:  []snapshot.Figure{f},
	}, nil
}
