package snapshot

import (
	"slices"
	"time"
)

// Stretch is a run of failures as a check engine remembers it: from the
// check's first failure to its next pass, with no end while it's still
// going, and whether the check engine alerted on it. One still going has
// alerted only if the check engine alerts on it now.
type Stretch struct {
	From    time.Time
	To      *time.Time
	Alerted bool
}

// CheckOf is ch as its results and the stretches its check engine
// remembers, oldest first, make it: its newest Recent results, its failures
// in a row, since when, whether it's a problem, and its outages, which are
// the stretches that alerted (CONTEXT.md, Problem). Its newest result is
// how it is now, whatever the stretches say last: a stretch still going
// ends at a newest result that passes, and one starts at the first of the
// failures in a row when none is going.
func CheckOf(ch Check, stretches []Stretch) Check {
	ch.Results = append([]Result{}, ch.Results[max(0, len(ch.Results)-Recent):]...)
	ch.Run = 0
	for i := len(ch.Results) - 1; i >= 0 && !ch.Results[i].OK; i-- {
		ch.Run++
	}
	stretches = append([]Stretch{}, stretches...)
	going := len(stretches) > 0 && stretches[len(stretches)-1].To == nil
	switch {
	case going && ch.Run == 0:
		if len(ch.Results) == 0 {
			stretches = stretches[:len(stretches)-1]
		} else {
			at := ch.Results[len(ch.Results)-1].At
			stretches[len(stretches)-1].To = &at
		}
	case !going && ch.Run > 0:
		stretches = append(stretches, Stretch{From: ch.Results[len(ch.Results)-ch.Run].At})
	}

	ch.Since, ch.Problem, ch.Outages = nil, false, nil
	if ch.Run > 0 {
		now := stretches[len(stretches)-1]
		ch.Since, ch.Problem = &now.From, now.Alerted
	}
	for _, s := range stretches {
		if s.Alerted && (s.To == nil || s.To.After(s.From)) {
			ch.Outages = append(ch.Outages, Outage{From: s.From, To: s.To})
		}
	}
	return ch
}

// Grid is the results of a check engine that keeps only when a check
// turned, such as Grafana: one at each of its last Recent evaluations, an
// interval apart to the last, failing where a stretch covers it. A check
// never evaluated, whose last evaluation is the zero time or near it, has
// none; with no interval, it's a minute.
//
// ponytail: an evaluation takes no time but the last's, which the check
// engine gives. If one ever keeps each evaluation, read them instead.
func Grid(last time.Time, interval time.Duration, stretches []Stretch) []Result {
	results := []Result{}
	if last.Year() < 2000 {
		return results
	}
	if interval <= 0 {
		interval = time.Minute
	}
	for k := Recent - 1; k >= 0; k-- {
		at := last.Add(-time.Duration(k) * interval)
		failing := slices.ContainsFunc(stretches, func(s Stretch) bool {
			return !at.Before(s.From) && (s.To == nil || at.Before(*s.To))
		})
		results = append(results, Result{OK: !failing, At: at})
	}
	return results
}

// ThresholdOf is how many evaluations in a row a pending period needs, an
// interval apart: a check engine that holds an alert pending fires at the
// first evaluation a whole pending period after the first failure, so 70
// seconds at a minute needs three. With no interval, it's a minute.
func ThresholdOf(pending, interval time.Duration) int {
	if interval <= 0 {
		interval = time.Minute
	}
	return int((pending+interval-1)/interval) + 1
}
