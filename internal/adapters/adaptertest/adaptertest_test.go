package adaptertest

import (
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/snapshot"
)

func TestProblems(t *testing.T) {
	t.Parallel()
	english := map[string]any{
		"pbs.backed_up": map[string]any{"one": "{n} guest backed up", "other": "{n} guests backed up"},
	}
	good := snapshot.Parts{Events: []snapshot.Event{{
		At: time.Now(), Area: "storage", Icon: "archive",
		Text: snapshot.Text{Key: "pbs.backed_up", Args: map[string]any{"n": 4}},
	}}}
	if got := Problems(good, english); len(got) != 0 {
		t.Errorf("a good event is refused: %q", got)
	}

	bad := snapshot.Parts{
		Figures: []snapshot.Figure{{
			Area: "attic", Label: snapshot.Text{Key: "nope.missing"}, Hours: []float64{1, 2}, Tone: "pink",
			Meters: []snapshot.Meter{{Label: &snapshot.Text{}, Total: 0, Tone: "teal"}},
		}},
		Events: []snapshot.Event{{Area: "storage", Text: snapshot.Text{Key: "pbs.backed_up"}}},
		Checks: []snapshot.Check{{Key: "k", Name: "n", Source: "s", URL: "u", Threshold: 3, Run: 0, Problem: true}},
	}
	if got := Problems(bad, english); len(got) != 10 {
		t.Errorf("want the area, the key, the hours, the tone, the meter's label, tone and total, the missing {n}, the missing time and the problem refused, got %d: %q", len(got), got)
	}
}
