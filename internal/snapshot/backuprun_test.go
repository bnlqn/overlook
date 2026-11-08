package snapshot

import (
	"testing"
	"time"
)

// Every backup run is an event this week, and each host's newest a row in
// Storage, in the tone of how it went, whichever adapter reads it.
func TestBackupRuns(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	night := func(days int) time.Time { return time.Date(2026, 10, 28-days, 0, 30, 0, 0, time.UTC) }
	p := BackupRuns{
		{Host: "vega", Start: night(1), End: night(1).Add(9 * time.Minute)},
		{Host: "nova", Start: night(0), End: night(0).Add(8 * time.Minute), Warnings: true, Why: "WARNINGS: 1"},
		{Host: "nova", Start: night(1), End: night(1).Add(time.Hour), Failed: true, Why: "job errors"},
		{Host: "vega", Start: night(9), End: night(9).Add(time.Minute)},
	}.Parts(now)
	if len(p.Sections) != 1 || p.Sections[0].Area != "storage" || p.Sections[0].Title.Key != "backuprun.section" {
		t.Fatalf("one section in Storage: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 2 || rows[0].Title.Name != "nova" || rows[1].Title.Name != "vega" {
		t.Fatalf("each host's newest run, by name: %+v", rows)
	}
	if r := rows[0]; r.Value.Key != "backuprun.warnings" || r.Tone != "orange" || r.Detail.Key != "backuprun.at_in_why" || r.Detail.Args["ms"] != int64(480_000) {
		t.Errorf("nova's last night, with warnings, in 8 min, and why: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Value.Key != "backuprun.done" || r.Tone != "green" || r.Detail.Key != "backuprun.at_in" {
		t.Errorf("vega's the night before, done: %+v", r)
	}
	if len(p.Events) != 3 {
		t.Fatalf("this week's three runs, not the one 9 days ago: %+v", p.Events)
	}
	if e := p.Events[0]; e.Text.Key != "backuprun.event_warned_in" || e.Tone != "orange" || e.At != night(0) {
		t.Errorf("newest first, with warnings: %+v", e)
	}
	if e := p.Events[2]; e.Text.Key != "backuprun.event_failed_why" || e.Tone != "red" || e.Text.Args["why"] != "job errors" {
		t.Errorf("a failed run, with why, in red: %+v", e)
	}

	// A backup server's runs: no host and no end, but the backups made.
	server := BackupRuns{{Start: night(0), Backups: 4}, {Start: night(1), Backups: 1}, {Start: now, Running: true}}.Parts(now)
	if r := server.Sections[0].Rows; len(r) != 1 || r[0].Title.Key != "backuprun.last" || r[0].Value.Key != "backuprun.running" || r[0].Tone != "blue" || r[0].Detail.Key != "backuprun.since" {
		t.Errorf("one row, the run still going: %+v", r)
	}
	if e := server.Events; e[1].Text.Key != "backuprun.event_made" || e[1].Text.Args["n"] != 4 || e[0].Text.Key != "backuprun.event_running" {
		t.Errorf("the backups each made, with no span: %+v", e)
	}
	if e := (BackupRuns{{Start: night(0), Failed: true}}).Parts(now).Events[0]; e.Text.Key != "backuprun.event_failed" {
		t.Errorf("a failed run that doesn't say why: %+v", e)
	}
	if p := (BackupRuns{}).Parts(now); len(p.Sections) != 0 {
		t.Errorf("no runs, no section: %+v", p)
	}
}
