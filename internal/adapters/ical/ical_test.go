package ical

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds a list recorded from Radicale 3.9.0 on 29 October 2026, its
// seven to-dos invented: one due on a date, with a note long enough to fold;
// one due at a time in Brussels, and one in UTC; one with only a start date;
// one done, one cancelled, and one that comes back every month.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL + "/owner/chores/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	if c.Name != "Chores" {
		t.Errorf("named %q", c.Name)
	}

	got := map[string]snapshot.Chore{}
	var names []string
	for _, ch := range p.Chores {
		got[ch.Name] = ch
		names = append(names, ch.Name)
	}
	want := []string{"Renew the printer's certificate", "Run the UPS self-test", "Scrub the ZFS pool", "Swap the offsite backup disk", "Label the cables in the rack"}
	if !slices.Equal(names, want) {
		t.Errorf("the to-dos not done or cancelled: %q", names)
	}
	if d := got["Swap the offsite backup disk"]; d.From != "2026-11-02" ||
		d.Note != "Bring the one at the office home, and write the date on its label so the next swap knows which is older.\nThe spare is in the drawer." {
		t.Errorf("a date as it is, and a note unfolded and unescaped: %+v", d)
	}
	if d := got["Renew the printer's certificate"]; d.From != "2026-10-30" {
		t.Errorf("a time by its own zone's calendar: %+v", d)
	}
	if d := got["Label the cables in the rack"]; d.From != "" {
		t.Errorf("a start date doesn't make a to-do due: %+v", d)
	}
	if d := got["Scrub the ZFS pool"]; d.From != "2026-12-01" {
		t.Errorf("one that comes back is due when it says: %+v", d)
	}
}

// A time in UTC is due on its day where Overlook is.
func TestParseUTC(t *testing.T) {
	t.Parallel()
	ics, err := os.ReadFile("testdata/owner/chores.ics")
	if err != nil {
		t.Fatal(err)
	}
	brussels, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		t.Fatal(err)
	}
	for loc, want := range map[*time.Location]string{time.UTC: "2026-11-05", brussels: "2026-11-06"} {
		chores, err := parse(ics, loc)
		if err != nil {
			t.Fatal(err)
		}
		if i := slices.IndexFunc(chores, func(c snapshot.Chore) bool { return c.Name == "Run the UPS self-test" }); i < 0 || chores[i].From != want {
			t.Errorf("23:30 UTC in %s: want %s, got %+v", loc, want, chores)
		}
	}
}

// A reminder's lines are its own, not the to-do's, and an answer that isn't
// iCalendar, such as a login page, is an error rather than no chores.
func TestParseEdges(t *testing.T) {
	t.Parallel()
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR", "BEGIN:VTODO", "SUMMARY:Dust the rack", "DUE;VALUE=DATE:20261103",
		"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:Reminder", "END:VALARM",
		`DESCRIPTION;ALTREP="cid:note@example.com":Top to bottom`, "END:VTODO",
		"BEGIN:VTODO", "SUMMARY:Done long ago", "COMPLETED:20261001T090000Z", "END:VTODO", "END:VCALENDAR",
	}, "\n")
	chores, err := parse([]byte(ics), time.UTC)
	if err != nil || len(chores) != 1 || chores[0] != (snapshot.Chore{Name: "Dust the rack", From: "2026-11-03", Note: "Top to bottom"}) {
		t.Errorf("read %+v, %v", chores, err)
	}
	if _, err := parse([]byte("<!doctype html>"), time.UTC); err == nil {
		t.Error("a page that isn't iCalendar read as no chores")
	}
}
