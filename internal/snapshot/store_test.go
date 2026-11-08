package snapshot

import "testing"

// Every store is a row in Storage: its room with a meter, then its health,
// red when the source counts it unwell, whichever adapter reads it.
func TestStores(t *testing.T) {
	t.Parallel()
	p := Stores{
		{Name: Text{Name: "rpool"}, Used: 10, Size: 100, Health: "ONLINE", Healthy: true},
		{Name: Text{Name: "tank"}, Used: 60, Size: 100, Health: "DEGRADED"},
		{Name: Text{Name: "local"}, Used: 5, Size: 50, Healthy: true},
		{Name: Text{Name: "nfs"}, Health: "unknown"},
	}.Parts()
	if len(p.Sections) != 1 || p.Sections[0].Area != "storage" || p.Sections[0].Title.Key != "store.section" || len(p.Figures) != 0 {
		t.Fatalf("one section in Storage, and no figure that adds stores up: %+v", p)
	}
	rows := p.Sections[0].Rows
	if r := rows[0]; r.Value.Key != "store.room" || r.Value.Args["size"] != int64(100) || r.Detail.Key != "store.healthy" || r.Tone != "" || r.Meter.Used != 10 {
		t.Errorf("rpool, its room and healthy: %+v", r)
	}
	if r := rows[1]; r.Value.Name != "DEGRADED" || r.Tone != "red" || r.Detail.Key != "store.room" || r.Meter == nil {
		t.Errorf("tank, degraded in red, its room under it: %+v", r)
	}
	if r := rows[2]; r.Value.Key != "store.room" || r.Detail != nil || r.Tone != "" {
		t.Errorf("local, whose source gives no health: %+v", r)
	}
	if r := rows[3]; r.Value.Name != "unknown" || r.Tone != "red" || r.Detail != nil || r.Meter != nil {
		t.Errorf("a store the source can't reach, with no room to show: %+v", r)
	}
	if p := (Stores{}).Parts(); len(p.Sections) != 0 {
		t.Errorf("no stores, no section: %+v", p)
	}
}
