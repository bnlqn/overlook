package snapshot

import (
	"cmp"
	"slices"
	"time"
)

// MediaServer is what an adapter reads from a media server (CONTEXT.md),
// which Parts makes the figure and section every media server gives, so
// that two media servers' plays add up, whichever adapters read them
// (ADR 0012).
type MediaServer struct {
	// Plays are when each play of the last day started, at least.
	Plays []time.Time
	// Playing is what plays now.
	Playing []Play
	// Rows are its own, after what plays, such as its libraries.
	Rows []Row
}

// Play is one person playing something now.
type Play struct {
	// Who plays it, and on which of their devices.
	Who, Device string
	// What is the film, episode or song, as the source words it, or a
	// message of the adapter's own, such as a show and its episode.
	What                Text
	Paused, Transcoding bool
}

// Parts are the media server's parts in area, as at now: its plays today
// with their last 24 hours, and its section, what plays now first.
func (m MediaServer) Parts(area string, now time.Time) Parts {
	// By person: paused in grey, and how it reaches them, as it is or
	// transcoded, which takes the processor.
	playing := slices.Clone(m.Playing)
	slices.SortFunc(playing, func(a, b Play) int {
		return cmp.Or(cmp.Compare(a.Who, b.Who), cmp.Compare(a.Device, b.Device))
	})
	var rows []Row
	for _, p := range playing {
		row := Row{Title: Text{Name: p.Who}, Value: &p.What}
		how := "mediaserver.direct"
		switch {
		case p.Paused:
			how, row.Tone = "mediaserver.paused", "gray"
		case p.Transcoding:
			how = "mediaserver.transcoding"
		}
		row.Detail = Msg(how, map[string]any{"device": p.Device})
		rows = append(rows, row)
	}
	if len(playing) == 0 {
		rows = append(rows, Row{Title: Text{Key: "mediaserver.playing"}, Value: Msg("mediaserver.nothing", nil)})
	}
	rows = append(rows, m.Rows...)

	plays := newTally(now, 24)
	for _, at := range m.Plays {
		plays.add(at, 1)
	}
	return Parts{
		Sections: []Section{{Area: area, Title: Text{Key: "mediaserver.section"}, Rows: rows}},
		Figures: []Figure{{
			Area: area, Icon: "play", Label: Text{Key: "mediaserver.plays"}, Adds: true,
			Value: float64(plays.today), Hours: plays.hours,
			Detail: Msg("mediaserver.playing_now", map[string]any{"n": len(playing)}),
			Tone:   orange(rows),
		}},
	}
}
