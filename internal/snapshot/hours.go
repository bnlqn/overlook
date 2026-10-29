package snapshot

import "time"

// The one rule for the hour (ADR 0012): what a source counts or measures
// goes by the hour where Overlook runs, the zone of now, the last one being
// this hour so far, and today starts at midnight there.

// hourOf is the start of the hour t falls in where Overlook runs: on the
// half hour in a half-hour zone, and two of them at 2:00 on the night the
// clocks go back.
func hourOf(t time.Time, where *time.Location) time.Time {
	_, offset := t.In(where).Zone()
	shift := time.Duration(offset) * time.Second
	return t.Add(shift).Truncate(time.Hour).Add(-shift)
}

// hoursAgo is how many of Overlook's hours t falls before now's: 0 in this
// one, 23 at this hour yesterday, and below 0 after it.
func hoursAgo(t, now time.Time) int {
	return int(hourOf(now, now.Location()).Sub(hourOf(t, now.Location())) / time.Hour)
}

// tally adds up counts by the hour, over the hours ending with now's, oldest
// first, and those since midnight, today's.
type tally struct {
	now, midnight time.Time
	hours         []float64
	today         int
}

func newTally(now time.Time, hours int) *tally {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return &tally{now: now, midnight: midnight, hours: make([]float64, hours)}
}

// add counts n at the moment at, in the hour it falls in.
func (t *tally) add(at time.Time, n int) {
	ago := hoursAgo(at, t.now)
	if ago < 0 {
		return
	}
	if ago < len(t.hours) {
		t.hours[len(t.hours)-1-ago] += float64(n)
	}
	if !at.Before(t.midnight) {
		t.today += n
	}
}

// orange is the tone of a kind's figure: orange when a row of its section is
// red, such as a resolver's blocking turned off, so that the overview shows
// it too.
func orange(rows []Row) string {
	for _, r := range rows {
		if r.Tone == "red" {
			return "orange"
		}
	}
	return ""
}
