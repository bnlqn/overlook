package snapshot

import (
	"testing"
	"time"
)

// Every media server's figure is its plays today with their last 24 hours,
// and its section what plays now, by person, then its own rows.
func TestMediaServer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 10, 45, 0, 0, time.UTC)
	m := MediaServer{
		Plays: []time.Time{now, now.Add(-50 * time.Minute), now.Add(-13 * time.Hour), now.Add(-25 * time.Hour)},
		Playing: []Play{
			{Who: "Sam", Device: "TV", What: Text{Name: "Metropolis (1927)"}, Paused: true},
			{Who: "Robin", Device: "Phone", What: Text{Name: "Nosferatu (1922)"}, Transcoding: true},
			{Who: "Alex", Device: "TV", What: Text{Name: "Sherlock Jr. (1924)"}},
		},
		Rows: []Row{{Title: Text{Name: "v1.0"}}},
	}
	p := m.Parts("apps", now)
	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "mediaserver.section" || p.Sections[0].Area != "apps" {
		t.Fatalf("one section, as any media server's: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 4 || rows[0].Title.Name != "Alex" || rows[3].Title.Name != "v1.0" {
		t.Fatalf("by person, then its own: %+v", rows)
	}
	if r := rows[0]; r.Detail.Key != "mediaserver.direct" || r.Detail.Args["device"] != "TV" || r.Tone != "" {
		t.Errorf("Alex's, as it is: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Title.Name != "Robin" || r.Detail.Key != "mediaserver.transcoding" {
		t.Errorf("Robin's, transcoded: %+v %+v", r, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Sam" || r.Detail.Key != "mediaserver.paused" || r.Tone != "gray" {
		t.Errorf("Sam's, paused in grey: %+v %+v", r, r.Detail)
	}
	f := p.Figures[0]
	if f.Label.Key != "mediaserver.plays" || !f.Adds || f.Value != 2 || f.Detail.Args["n"] != 3 || f.Tone != "" {
		t.Errorf("2 plays since midnight, 3 playing now, adding up with any media server's: %+v", f)
	}
	if len(f.Hours) != 24 || f.Hours[23] != 1 || f.Hours[22] != 1 || f.Hours[10] != 1 || sum(f.Hours) != 3 {
		t.Errorf("one this hour, one in the last, one at 21:00, none from the day before: %v", f.Hours)
	}

	idle := MediaServer{Rows: []Row{{Title: Text{Name: "Scan"}, Tone: "red"}}}.Parts("apps", now)
	if rows := idle.Sections[0].Rows; rows[0].Value.Key != "mediaserver.nothing" || rows[1].Title.Name != "Scan" {
		t.Errorf("nothing playing, then its own: %+v", rows)
	}
	if f := idle.Figures[0]; f.Tone != "orange" || f.Value != 0 || f.Detail.Args["n"] != 0 {
		t.Errorf("orange for a red row of its own: %+v", f)
	}
}
