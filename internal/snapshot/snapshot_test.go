package snapshot

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReader(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	var given time.Time
	up := Source{Name: "Up", Adapter: "adguard", Area: "network", Read: func(_ context.Context, now time.Time) (Parts, error) {
		reads.Add(1)
		given = now
		return Parts{
			Checks:   []Check{{Key: "a"}},
			Charts:   []Chart{{Area: "network"}},
			Sections: []Section{{Area: "network"}},
			Figures:  []Figure{{Area: "network"}},
			Events:   []Event{{Area: "network"}},
			Chores:   []Chore{{Name: "Dust"}},
			Updates:  []Update{{Name: "older", At: time.Unix(100, 0)}},
		}, nil
	}}
	down := Source{Name: "Down", Adapter: "adguard", Read: func(context.Context, time.Time) (Parts, error) {
		return Parts{Checks: []Check{{Key: "lost"}}}, errors.New("timeout")
	}}
	forge := Source{Name: "Forge", Adapter: "gitea", Read: func(context.Context, time.Time) (Parts, error) {
		return Parts{Updates: []Update{{Name: "newer", At: time.Unix(200, 0)}}}, nil
	}}
	r := NewReader([]Source{up, down, forge}, nil, []Link{{Name: "Gatus"}})

	snap := r.Snapshot()
	if given.IsZero() || !given.Equal(snap.At) {
		t.Errorf("a source is read at the snapshot's moment: %v, not %v", given, snap.At)
	}
	if len(snap.Sources) != 3 || !snap.Sources[0].Answered || snap.Sources[1].Answered || snap.Sources[1].Error != "timeout" {
		t.Errorf("a source that doesn't answer is marked so: %+v", snap.Sources)
	}
	if len(snap.Checks) != 1 || snap.Checks[0].Key != "a" {
		t.Errorf("a source that doesn't answer gives no parts: %+v", snap.Checks)
	}
	if snap.Apps == nil || snap.Sections == nil || len(snap.Tools) != 1 {
		t.Errorf("lists are empty, never null: %+v", snap)
	}
	if len(snap.Updates) != 2 || snap.Updates[0].Name != "newer" {
		t.Errorf("the updates are newest first, whichever source they wait in: %+v", snap.Updates)
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
	if snap.Checks[0].State != "unknown" || len(snap.Areas) != len(Areas) || snap.Homelab.State != "up" || snap.Tools[0].State != "unknown" {
		t.Errorf("the states are decided as the read's last step: %+v %+v %+v", snap.Checks, snap.Areas, snap.Homelab)
	}
	r.Snapshot()
	if reads.Load() != 1 {
		t.Errorf("a fresh snapshot was read again: %d reads", reads.Load())
	}
}

// A store that is another source's own shows once: the reader leaves it out
// while a source of that adapter answers, with its section if it was the
// only row, and keeps it while none does.
func TestReaderShowsAStoreOnce(t *testing.T) {
	t.Parallel()
	pve := Source{Name: "Proxmox", Adapter: "proxmox", Storage: true, Read: func(context.Context, time.Time) (Parts, error) {
		return Stores{{Name: Text{Name: "rpool"}, Healthy: true}, {Name: Text{Name: "backups"}, Healthy: true, On: "pbs"}}.Parts(), nil
	}}
	remote := Source{Name: "Remote", Adapter: "proxmox", Storage: true, Read: func(context.Context, time.Time) (Parts, error) {
		return Stores{{Name: Text{Name: "offsite"}, Healthy: true, On: "pbs"}}.Parts(), nil
	}}
	for _, c := range []struct {
		pbs  string
		err  error
		want []string
	}{
		{"answers", nil, []string{"Proxmox: rpool"}},
		{"doesn't answer", errors.New("timeout"), []string{"Proxmox: rpool backups", "Remote: offsite"}},
	} {
		pbs := Source{Name: "Backup Server", Adapter: "pbs", Storage: true, Read: func(context.Context, time.Time) (Parts, error) {
			return Parts{}, c.err
		}}
		var got []string
		for _, s := range NewReader([]Source{pve, remote, pbs}, nil, nil).Snapshot().Sections {
			var names []string
			for _, r := range s.Rows {
				names = append(names, r.Title.Name)
			}
			got = append(got, s.Source+": "+strings.Join(names, " "))
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("while the backup server %s: %q, not %q", c.pbs, got, c.want)
		}
	}
}

// A link whose check's name two check engines share, and which doesn't say
// which, is warned of; one that names its engine isn't.
func TestReaderWarns(t *testing.T) {
	t.Parallel()
	engine := func(name string) Source {
		return Source{Name: name, Adapter: "gatus", Read: func(context.Context, time.Time) (Parts, error) {
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
