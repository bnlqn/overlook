// Package tdarr reads Tdarr, a transcoding server: the files its nodes are
// working on now, each with how far along it is, and how many files wait. It
// gives a section only while a node works on something or files wait, since
// an idle Tdarr has nothing to say that a check engine's answer doesn't.
package tdarr

import (
	"cmp"
	"context"
	"math"
	"net/http"
	"path"
	"slices"
	"strconv"
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
		c.source.Header = http.Header{"X-Api-Key": {c.Key}}
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
	var (
		jobs   []job
		paused []string
		// queued is the longest queue of each kind of work among the
		// nodes. A node's queue is the files its workers could take, which
		// the server holds for all of them, so adding the nodes' queues
		// would count a file once for each node.
		queued = map[string]int{}
	)
	for _, n := range nodes {
		if n.Paused {
			paused = append(paused, n.Name)
		}
		for _, w := range n.Workers {
			if !w.Idle && w.File != "" {
				jobs = append(jobs, job{n.Name, w})
			}
		}
		for kind, q := range n.QueueLengths {
			queued[kind] = max(queued[kind], q)
		}
	}
	waiting := 0
	for _, q := range queued {
		waiting += q
	}
	// With nothing in work and nothing waiting there is nothing to show.
	if len(jobs) == 0 && waiting == 0 {
		return snapshot.Parts{}, nil
	}
	slices.SortFunc(jobs, func(a, b job) int {
		return cmp.Or(cmp.Compare(a.node, b.node), cmp.Compare(a.File, b.File))
	})
	slices.Sort(paused)

	rows := make([]snapshot.Row, 0, len(jobs)+len(paused)+1)
	for _, j := range jobs {
		kind := "tdarr.transcoding"
		if strings.HasPrefix(j.Type, "healthcheck") {
			kind = "tdarr.checking"
		}
		percent := math.Round(min(max(j.Percentage, 0), 100))
		// What Tdarr reports of the file: it says how fast and how long
		// only once the file has begun, and a quick check may never say.
		key, args := kind+"_bare", map[string]any{"node": j.node}
		eta, ok := span(j.ETA)
		switch {
		case percent == 0:
			key = kind + "_start"
		case ok && j.FPS > 0:
			key, args["fps"], args["eta"] = kind+"_full", int(math.Round(j.FPS)), eta
		case ok:
			key, args["eta"] = kind+"_eta", eta
		}
		rows = append(rows, snapshot.Row{
			Title:  snapshot.Text{Name: path.Base(strings.ReplaceAll(j.File, `\`, "/"))},
			Value:  snapshot.Percent(percent),
			Detail: snapshot.Msg(key, args),
			Meter:  &snapshot.Meter{Used: percent, Total: 100, Tone: "blue"},
		})
	}
	if waiting > 0 {
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Key: "tdarr.waiting"}, Value: snapshot.Msg("tdarr.waiting_n", map[string]any{"n": waiting}),
		})
	}
	// A node that is paused takes no work, which is the one thing that
	// keeps files waiting for good.
	if len(jobs) == 0 {
		for _, name := range paused {
			rows = append(rows, snapshot.Row{Title: snapshot.Text{Name: name}, Value: snapshot.Msg("tdarr.paused", nil), Tone: "orange"})
		}
	}
	parts := snapshot.Parts{Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "tdarr.title"}, Rows: rows}}}
	// Today: the files in work. Like the section, it is there only while
	// there is work.
	if len(jobs) > 0 {
		parts.Figures = []snapshot.Figure{{
			Area: Area, Icon: "refresh", Label: snapshot.Text{Key: "tdarr.title"}, Value: float64(len(jobs)),
			Unit:   snapshot.Msg("tdarr.unit", nil),
			Detail: snapshot.Msg("tdarr.figure_detail", map[string]any{"n": waiting}),
		}}
	}
	return parts, nil
}

// span is Tdarr's time left, written H:MM:SS, in milliseconds, or false when
// it isn't written so, as with none left to tell.
func span(eta string) (int64, bool) {
	parts := strings.Split(eta, ":")
	if len(parts) != 3 {
		return 0, false
	}
	var seconds int64
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return 0, false
		}
		seconds = seconds*60 + n
	}
	return seconds * 1000, true
}
