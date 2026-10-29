package snapshot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestReader(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	up := Source{Name: "Up", Adapter: "adguard", Area: "network", Read: func(context.Context) (Parts, error) {
		reads.Add(1)
		return Parts{
			Checks:   []Check{{Key: "a"}},
			Charts:   []Chart{{Area: "network"}},
			Sections: []Section{{Area: "network"}},
			Figures:  []Figure{{Area: "network"}},
			Events:   []Event{{Area: "network"}},
			Chores:   []Chore{{Name: "Dust"}},
		}, nil
	}}
	down := Source{Name: "Down", Adapter: "adguard", Read: func(context.Context) (Parts, error) {
		return Parts{Checks: []Check{{Key: "lost"}}}, errors.New("timeout")
	}}
	r := NewReader([]Source{up, down}, nil, []Link{{Name: "Gatus"}})

	snap := r.Snapshot()
	if len(snap.Sources) != 2 || !snap.Sources[0].Answered || snap.Sources[1].Answered || snap.Sources[1].Error != "timeout" {
		t.Errorf("a source that doesn't answer is marked so: %+v", snap.Sources)
	}
	if len(snap.Checks) != 1 || snap.Checks[0].Key != "a" {
		t.Errorf("a source that doesn't answer gives no parts: %+v", snap.Checks)
	}
	if snap.Apps == nil || snap.Sections == nil || len(snap.Tools) != 1 {
		t.Errorf("lists are empty, never null: %+v", snap)
	}
	if len(snap.Chores) != 1 || snap.Chores[0].Name != "Dust" || snap.Chores[0].Source != "Up" {
		t.Errorf("a source's chores are the snapshot's: %+v", snap.Chores)
	}
	if snap.Checks[0].Source != "Up" || snap.Charts[0].Source != "Up" || snap.Sections[0].Source != "Up" || snap.Figures[0].Source != "Up" || snap.Events[0].Source != "Up" {
		t.Errorf("each part is named by its source: %+v", snap.Parts)
	}
	if snap.Sources[0].Adapter != "adguard" || snap.Sources[1].Adapter != "adguard" || snap.Sources[0].Area != "network" {
		t.Errorf("each source says its adapter, answered or not: %+v", snap.Sources)
	}
	r.Snapshot()
	if reads.Load() != 1 {
		t.Errorf("a fresh snapshot was read again: %d reads", reads.Load())
	}
}

// A link whose check's name two check engines share, and which doesn't say
// which, is warned of; one that names its engine isn't.
func TestReaderWarns(t *testing.T) {
	t.Parallel()
	engine := func(name string) Source {
		return Source{Name: name, Adapter: "gatus", Read: func(context.Context) (Parts, error) {
			return Parts{Checks: []Check{{Key: "photos", Name: "Photos"}}}, nil
		}}
	}
	r := NewReader([]Source{engine("Gatus"), engine("Gatus Outside")},
		[]Link{{Name: "Photos", Check: "Photos"}, {Name: "Albums", Check: "Photos", Engine: "Gatus"}}, nil)
	r.Snapshot()
	if !r.warned["Photos"] || r.warned["Albums"] {
		t.Errorf("warned of %v", r.warned)
	}
}
