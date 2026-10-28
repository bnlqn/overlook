package main

import (
	"context"
	"sync"
	"time"
)

// Text is a message key with its values (ADR 0006), which the page turns
// into words from its table, or, with no key, a name shown as written.
type Text struct {
	Key  string         `json:"key,omitempty"`
	Args map[string]any `json:"args,omitempty"`
	Name string         `json:"name,omitempty"`
}

// Check is one check of the check engine and its recent results.
type Check struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Area      string `json:"area,omitempty"`
	Source    string `json:"source"`
	URL       string `json:"url"`
	Threshold int    `json:"threshold"`
	// Failures in a row, up to now; a problem from Threshold of them.
	Run     int  `json:"run"`
	Problem bool `json:"problem"`
	// The newest result's failure, as the check engine words it.
	Why string `json:"why,omitempty"`
	// When the run of failures started, if the check engine remembers.
	Since   *time.Time `json:"since,omitempty"`
	Results []Result   `json:"results"`
	// The problems the check engine remembers, the one still going with no end.
	Outages []Outage `json:"outages,omitempty"`
	// When the check engine started the check, if it remembers.
	Watched *time.Time `json:"watched,omitempty"`
}

type Result struct {
	OK  bool      `json:"ok"`
	At  time.Time `json:"at"`
	MS  *float64  `json:"ms"`
	Why string    `json:"why,omitempty"`
}

type Outage struct {
	From time.Time  `json:"from"`
	To   *time.Time `json:"to"`
}

// Section is a titled list of rows on its area's page.
type Section struct {
	Area  string `json:"area"`
	Title Text   `json:"title"`
	Rows  []Row  `json:"rows"`
}

type Row struct {
	Title  Text  `json:"title"`
	Value  *Text `json:"value,omitempty"`
	Detail *Text `json:"detail,omitempty"`
	// The value's colour: green, orange, red, blue or gray.
	Tone string `json:"tone,omitempty"`
	// How full, drawn as a bar under the row.
	Meter *Meter `json:"meter,omitempty"`
}

type Meter struct {
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
}

// Figure is a number for today, sometimes with the hours that make it up.
type Figure struct {
	Area string `json:"area"`
	// One of the page's icons, such as shield or photos.
	Icon   string  `json:"icon"`
	Label  Text    `json:"label"`
	Value  float64 `json:"value"`
	Unit   *Text   `json:"unit,omitempty"`
	Detail *Text   `json:"detail,omitempty"`
	Tone   string  `json:"tone,omitempty"`
	// The last 24 hours, oldest first, ending with this one.
	Hours []float64 `json:"hours,omitempty"`
}

// Event is something the homelab did this week, as a source remembers it.
type Event struct {
	At   time.Time `json:"at"`
	Area string    `json:"area"`
	Icon string    `json:"icon"`
	Tone string    `json:"tone,omitempty"`
	Text Text      `json:"text"`
}

// Update is one waiting to be merged.
type Update struct {
	Name string    `json:"name"`
	URL  string    `json:"url"`
	At   time.Time `json:"at"`
}

// Parts are what one source gives back, in Overlook's shapes.
type Parts struct {
	Checks   []Check   `json:"checks"`
	Sections []Section `json:"sections"`
	Figures  []Figure  `json:"figures"`
	Events   []Event   `json:"events"`
	Updates  []Update  `json:"updates"`
}

// Snapshot is every source's parts as read at one moment, and the page's
// links. A source that didn't answer is marked so and gives no parts.
type Snapshot struct {
	At      time.Time     `json:"at"`
	Sources []SourceState `json:"sources"`
	Parts
	Apps  []Link `json:"apps"`
	Tools []Link `json:"tools"`
}

type SourceState struct {
	Name     string `json:"name"`
	Answered bool   `json:"answered"`
	Error    string `json:"error,omitempty"`
}

type source struct {
	name string
	read func(context.Context) (Parts, error)
}

func sources(c Config) []source {
	var all []source
	for _, g := range c.Sources.Gatus {
		all = append(all, source{g.Name, func(ctx context.Context) (Parts, error) { return readGatus(ctx, g) }})
	}
	return all
}

const (
	fresh   = 30 * time.Second
	timeout = 10 * time.Second
)

// snapshots reads every source when the page asks and the last snapshot is
// older than fresh, and remembers nothing else (ADR 0004).
type snapshots struct {
	config  Config
	sources []source
	mu      sync.Mutex
	last    Snapshot
}

func (s *snapshots) get() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.last.At) < fresh {
		return s.last
	}
	parts := make([]Parts, len(s.sources))
	errs := make([]error, len(s.sources))
	var wg sync.WaitGroup
	for i, src := range s.sources {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			parts[i], errs[i] = src.read(ctx)
		})
	}
	wg.Wait()
	// Empty lists, never null, so the page needn't tell them apart.
	snap := Snapshot{At: time.Now(), Sources: []SourceState{}, Parts: Parts{[]Check{}, []Section{}, []Figure{}, []Event{}, []Update{}},
		Apps: append([]Link{}, s.config.Apps...), Tools: append([]Link{}, s.config.Tools...)}
	for i, src := range s.sources {
		state := SourceState{Name: src.name, Answered: errs[i] == nil}
		if errs[i] != nil {
			state.Error = errs[i].Error()
		} else {
			p := parts[i]
			snap.Checks = append(snap.Checks, p.Checks...)
			snap.Sections = append(snap.Sections, p.Sections...)
			snap.Figures = append(snap.Figures, p.Figures...)
			snap.Events = append(snap.Events, p.Events...)
			snap.Updates = append(snap.Updates, p.Updates...)
		}
		snap.Sources = append(snap.Sources, state)
	}
	s.last = snap
	return snap
}
