// Package snapshot holds Overlook's shapes, which every adapter gives back and
// the page draws, and reads every source into a snapshot when the page asks
// (ADR 0004).
package snapshot

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// fresh is how long a snapshot is served before the next request reads
	// every source again.
	fresh = 30 * time.Second
	// timeout is how long each source has to answer.
	timeout = 10 * time.Second
)

// Snapshot is every source's parts as read at one moment, and the config's
// links. A source that didn't answer is marked so and gives no
// parts.
type Snapshot struct {
	At      time.Time     `json:"at"`
	Sources []SourceState `json:"sources"`
	Parts
	Apps  []Link `json:"apps"`
	Tools []Link `json:"tools"`
}

// SourceState says whether a source answered the last read.
type SourceState struct {
	Name string `json:"name"`
	// Adapter is the adapter that reads it, such as adguard: with another
	// source of the same, the page names each one's parts.
	Adapter  string `json:"adapter"`
	Answered bool   `json:"answered"`
	// Error is short and never holds the source's URL (see fetch.JSON).
	Error string `json:"error,omitempty"`
}

// Source is one source, read by its adapter.
type Source struct {
	Name    string
	Adapter string
	Read    func(context.Context) (Parts, error)
}

// Reader reads every source into a snapshot when asked and the last is
// older than fresh, and remembers nothing else (ADR 0004).
type Reader struct {
	sources     []Source
	apps, tools []Link

	mu   sync.Mutex
	last Snapshot
}

// NewReader returns a Reader of sources whose snapshots carry apps and
// tools.
func NewReader(sources []Source, apps, tools []Link) *Reader {
	return &Reader{sources: sources, apps: apps, tools: tools}
}

// Snapshot returns the last snapshot, or a new one if it's older than fresh.
// Every source is read in parallel, each for at most timeout. A request that
// comes while a read is going waits for it rather than read again.
func (r *Reader) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.last.At) < fresh {
		return r.last
	}
	parts := make([]Parts, len(r.sources))
	errs := make([]error, len(r.sources))
	var wg sync.WaitGroup
	for i, src := range r.sources {
		wg.Go(func() {
			// Not the request's context: a closed tab mustn't spoil the
			// snapshot every other tab is waiting for.
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			parts[i], errs[i] = src.Read(ctx)
		})
	}
	wg.Wait()

	// Empty lists, never null, so the page needn't tell them apart.
	snap := Snapshot{
		At:      time.Now(),
		Sources: make([]SourceState, 0, len(r.sources)),
		Parts:   Parts{Checks: []Check{}, Sections: []Section{}, Charts: []Chart{}, Figures: []Figure{}, Events: []Event{}, Updates: []Update{}, Chores: []Chore{}},
		Apps:    append([]Link{}, r.apps...),
		Tools:   append([]Link{}, r.tools...),
	}
	for i, src := range r.sources {
		state := SourceState{Name: src.Name, Adapter: src.Adapter, Answered: errs[i] == nil}
		if errs[i] != nil {
			state.Error = errs[i].Error()
			slog.Warn("a source didn't answer", "source", src.Name, "error", errs[i])
		} else {
			// Each part is named by its source, so two of one kind can be
			// told apart.
			p := parts[i]
			snap.Checks = append(snap.Checks, p.Checks...)
			for _, s := range p.Sections {
				s.Source = src.Name
				snap.Sections = append(snap.Sections, s)
			}
			for _, c := range p.Charts {
				c.Source = src.Name
				snap.Charts = append(snap.Charts, c)
			}
			for _, f := range p.Figures {
				f.Source = src.Name
				snap.Figures = append(snap.Figures, f)
			}
			for _, e := range p.Events {
				e.Source = src.Name
				snap.Events = append(snap.Events, e)
			}
			snap.Updates = append(snap.Updates, p.Updates...)
			snap.Chores = append(snap.Chores, p.Chores...)
		}
		snap.Sources = append(snap.Sources, state)
	}
	r.last = snap
	return snap
}
