package snapshot

import (
	"slices"
	"testing"
	"time"
)

// minute is n minutes after 10:00 UTC on 28 October 2026, a time the tests
// invent.
func minute(n int) time.Time {
	return time.Date(2026, 10, 28, 10, n, 0, 0, time.UTC)
}

func at(n int) *time.Time {
	t := minute(n)
	return &t
}

// results are a check's, a minute apart from 10:00: + passes and - fails.
func results(s string) []Result {
	out := []Result{}
	for i, r := range s {
		out = append(out, Result{OK: r == '+', At: minute(i)})
	}
	return out
}

func TestCheckOf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name      string
		results   string
		stretches []Stretch
		run       int
		since     *time.Time
		problem   bool
		outages   []Outage
	}{
		{name: "passing, with nothing remembered", results: "+++"},
		{
			name: "failing below its threshold", results: "++--",
			stretches: []Stretch{{From: minute(2)}},
			run:       2, since: at(2),
		},
		{
			name: "a problem since its first failure, its outage from the same moment", results: "+---",
			stretches: []Stretch{{From: minute(1), Alerted: true}},
			run:       3, since: at(1), problem: true,
			outages: []Outage{{From: minute(1)}},
		},
		{
			name: "firing but silenced, so no problem, and no outage that ends now", results: "+---",
			stretches: []Stretch{{From: minute(1)}},
			run:       3, since: at(1),
		},
		{
			name: "a past problem kept, a past blip dropped", results: "+++",
			stretches: []Stretch{
				{From: minute(-30), To: at(-20), Alerted: true},
				{From: minute(-10), To: at(-9)},
			},
			outages: []Outage{{From: minute(-30), To: at(-20)}},
		},
		{
			name: "passing now, though the engine still says failing: it ended at the newest result", results: "--+",
			stretches: []Stretch{{From: minute(0), Alerted: true}},
			outages:   []Outage{{From: minute(0), To: at(2)}},
		},
		{
			name: "failing now, though the engine remembers nothing: since the first failure in a row", results: "++--",
			run: 2, since: at(2),
		},
		{
			name: "no results, though the engine says failing: nothing", results: "",
			stretches: []Stretch{{From: minute(0), Alerted: true}},
		},
	} {
		ch := CheckOf(Check{Threshold: 3, Results: results(c.results)}, c.stretches)
		if ch.Run != c.run || ch.Problem != c.problem || ch.Threshold != 3 {
			t.Errorf("%s: run %d, problem %v, threshold %d", c.name, ch.Run, ch.Problem, ch.Threshold)
		}
		if (ch.Since == nil) != (c.since == nil) || ch.Since != nil && !ch.Since.Equal(*c.since) {
			t.Errorf("%s: since %v, want %v", c.name, ch.Since, c.since)
		}
		same := func(a, b Outage) bool {
			return a.From.Equal(b.From) && (a.To == nil) == (b.To == nil) && (a.To == nil || a.To.Equal(*b.To))
		}
		if !slices.EqualFunc(ch.Outages, c.outages, same) {
			t.Errorf("%s: outages %+v, want %+v", c.name, ch.Outages, c.outages)
		}
	}
}

// A check engine that keeps more results than the page shows gives the
// newest, oldest first.
func TestCheckOfKeepsTheNewest(t *testing.T) {
	t.Parallel()
	ch := CheckOf(Check{Results: results("----------++++++++++++++++++++++++++++++")}, nil)
	if len(ch.Results) != Recent || !ch.Results[0].At.Equal(minute(10)) || ch.Run != 0 {
		t.Errorf("the newest %d, from 10:10: %d from %v, run %d", Recent, len(ch.Results), ch.Results[0].At, ch.Run)
	}
}

func TestGrid(t *testing.T) {
	t.Parallel()
	if got := Grid(time.Time{}, time.Minute, nil); got == nil || len(got) != 0 {
		t.Errorf("never evaluated, no results: %+v", got)
	}
	stretches := []Stretch{{From: minute(5), To: at(7)}, {From: minute(28)}}
	got := Grid(minute(29), 0, stretches)
	var failed []int
	for i, r := range got {
		if !r.OK {
			failed = append(failed, i)
		}
	}
	if len(got) != Recent || !got[0].At.Equal(minute(0)) || !slices.Equal(failed, []int{5, 6, 28, 29}) {
		t.Errorf("%d results a minute apart to 10:29, failing at 5, 6, 28 and 29: from %v, failing %v", len(got), got[0].At, failed)
	}
}

func TestThresholdOf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		pending, interval time.Duration
		want              int
	}{
		{0, time.Minute, 1},
		{60 * time.Second, time.Minute, 2},
		{70 * time.Second, time.Minute, 3},
		{90 * time.Second, 0, 3},
		{time.Hour, 10 * time.Second, 361},
	} {
		if got := ThresholdOf(c.pending, c.interval); got != c.want {
			t.Errorf("%v pending at %v: %d, want %d", c.pending, c.interval, got, c.want)
		}
	}
}
