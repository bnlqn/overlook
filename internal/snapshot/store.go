package snapshot

import "cmp"

// Store is one place the homelab keeps its data (CONTEXT.md), such as a ZFS
// pool, a Proxmox VE storage or a backup server's datastore, as an adapter
// reads it. Stores makes Storage's rows of them, so that every store looks
// the same on the page, whichever adapter reads it (ADR 0016).
type Store struct {
	// Name is the source's own, such as rpool, or a message of the
	// adapter's own, such as a pool and the node it's on.
	Name Text
	// Used and Size are the room it takes and the room it has, in bytes.
	// Size is 0 where the source doesn't know them, such as for a store it
	// can't reach.
	Used, Size int64
	// Health is its health as the source words it, such as ONLINE or
	// DEGRADED, or none where the source gives none, as Proxmox VE gives
	// none for a storage it reaches. Healthy says the source counts it
	// well: a store that isn't is a red row, never a problem, since only a
	// check makes one (ADR 0001).
	Health  string
	Healthy bool
}

// Stores are the stores one source reads.
type Stores []Store

// Parts are the stores' section in Storage, a row each in their order: the
// room each takes of the room it has, and its health. No figure adds them
// up, since a Proxmox VE storage on a ZFS pool is the same room twice
// (ADR 0016).
func (s Stores) Parts() Parts {
	if len(s) == 0 {
		return Parts{}
	}
	rows := make([]Row, len(s))
	for i, st := range s {
		row := Row{Title: st.Name}
		var room *Text
		if st.Size > 0 {
			room = Msg("store.room", map[string]any{"used": st.Used, "size": st.Size})
			row.Meter = &Meter{Used: float64(st.Used), Total: float64(st.Size)}
		}
		var health *Text
		if st.Health != "" {
			health = &Text{Name: st.Health}
		}
		switch {
		case !st.Healthy:
			// What's wrong first, the room under it.
			row.Value, row.Detail, row.Tone = cmp.Or(health, Msg("store.unhealthy", nil)), room, "red"
		case room != nil:
			row.Value = room
			if health != nil {
				row.Detail = Msg("store.healthy", nil)
			}
		default:
			row.Value = health
		}
		rows[i] = row
	}
	return Parts{Sections: []Section{{Area: "storage", Title: Text{Key: "store.section"}, Rows: rows}}}
}
