package snapshot

import (
	"cmp"
	"slices"
	"time"
)

// BackupRun is one run that makes backups (CONTEXT.md), such as a night's
// run of Proxmox VE's backup job, as an adapter reads it. BackupRuns makes
// Storage's rows and events of them, so that every backup run looks the
// same on the page, whichever adapter reads it (ADR 0016).
type BackupRun struct {
	// Host is the host it ran on, where a source runs them on several, such
	// as a node of a cluster: each host's newest run gets a row.
	Host string
	// Start is when it started, and End when it ended, zero while it runs
	// or where the source doesn't say, as a backup server doesn't, which
	// knows only when each backup started.
	Start, End time.Time
	// How it went: Running while it runs; once it ended, Failed if it
	// failed and Warnings if it ran with warnings, and Why, as the source
	// words either, such as its error.
	Running, Warnings, Failed bool
	Why                       string
	// Backups is how many backups it made, or 0 where the source doesn't
	// say.
	Backups int
}

// BackupRuns are the backup runs one source reads.
type BackupRuns []BackupRun

// Parts are the backup runs' section in Storage, each host's newest run by
// the host's name, and this week's runs as events, as at now. A failed run
// is a red row, never a problem, since only a check makes one (ADR 0001).
func (b BackupRuns) Parts(now time.Time) Parts {
	if len(b) == 0 {
		return Parts{}
	}
	runs := slices.Clone(b)
	slices.SortStableFunc(runs, func(x, y BackupRun) int { return y.Start.Compare(x.Start) })

	var p Parts
	newest := map[string]bool{}
	var rows []Row
	for _, r := range runs {
		args := map[string]any{"at": r.Start}
		took := r.End.After(r.Start)
		if took {
			args["ms"] = r.End.Sub(r.Start).Milliseconds()
		}
		if r.Why != "" {
			args["why"] = r.Why
		}
		if r.Backups > 0 {
			args["n"] = r.Backups
		}
		// A key for each of what the source says: how long it took, and
		// why it went as it did.
		key := func(base string, why bool) string {
			if took {
				base += "_in"
			}
			if why && r.Why != "" {
				base += "_why"
			}
			return base
		}

		e := Event{At: r.Start, Area: "storage", Icon: "archive"}
		row := Row{Title: Text{Key: "backuprun.last"}, Detail: Msg(key("backuprun.at", false), args)}
		if r.Host != "" {
			row.Title = Text{Name: r.Host}
		}
		switch {
		case r.Running:
			e.Text = Text{Key: "backuprun.event_running"}
			row.Value, row.Tone, row.Detail = Msg("backuprun.running", nil), "blue", Msg("backuprun.since", args)
		case r.Failed:
			e.Text, e.Tone = Text{Key: "backuprun.event_failed", Args: args}, "red"
			if r.Why != "" {
				e.Text.Key = "backuprun.event_failed_why"
			}
			row.Value, row.Tone, row.Detail = Msg("backuprun.failed", nil), "red", Msg(key("backuprun.at", true), args)
		case r.Warnings:
			e.Text, e.Tone = Text{Key: key("backuprun.event_warned", false), Args: args}, "orange"
			row.Value, row.Tone, row.Detail = Msg("backuprun.warnings", nil), "orange", Msg(key("backuprun.at", true), args)
		case r.Backups > 0:
			e.Text = Text{Key: key("backuprun.event_made", false), Args: args}
			row.Value, row.Tone = Msg("backuprun.backups", args), "green"
		default:
			e.Text = Text{Key: key("backuprun.event_ran", false), Args: args}
			row.Value, row.Tone = Msg("backuprun.done", nil), "green"
		}
		if now.Sub(r.Start) <= 7*24*time.Hour {
			p.Events = append(p.Events, e)
		}
		if !newest[r.Host] {
			newest[r.Host] = true
			rows = append(rows, row)
		}
	}
	slices.SortStableFunc(rows, func(x, y Row) int { return cmp.Compare(x.Title.Name, y.Title.Name) })
	p.Sections = []Section{{Area: "storage", Title: Text{Key: "backuprun.section"}, Rows: rows}}
	return p
}
