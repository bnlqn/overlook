package snapshot

import (
	"maps"
	"math"
	"slices"
	"time"
)

// The titles every adapter that reads hosts gives its parts: the section of
// its hosts, which the page names by its source where another source gives
// one too, and the charts of what they measure, which the hosts share, a
// line each, whichever source reads them (CONTEXT.md, Chart).
var (
	HostsSection   = Text{Key: "section.hosts"}
	ProcessorChart = Text{Key: "chart.processor"}
	MemoryChart    = Text{Key: "chart.memory"}
)

// Reading is how busy a host's processor and how full its memory were at a
// moment, in percent.
type Reading struct {
	At       time.Time
	CPU, Mem float64
}

// HostCharts are hosts' processor and memory by the hour in area, as at
// now, a line each named by names, from readings, each host's in names'
// order. They run over Overlook's hours (ADR 0012) from when every host has
// a reading to this one: each hour the mean of its readings, and an hour
// with none takes the last one's. With a host without a reading, or under
// two hours, there are none.
//
// ponytail: an hour with no reading draws the last, which a line can't tell
// from a level that held. If gaps inside a week turn up, draw them as gaps.
func HostCharts(area string, now time.Time, names []string, readings [][]Reading) []Chart {
	type level struct{ cpu, mem []float64 }
	// Each host's levels by how many hours before this one, and from how
	// many hours ago every host has one.
	by := make([]map[int]level, len(names))
	from := math.MaxInt
	for i := range names {
		by[i] = map[int]level{}
		for _, r := range readings[i] {
			ago := hoursAgo(r.At, now)
			if ago < 0 {
				continue
			}
			l := by[i][ago]
			l.cpu, l.mem = append(l.cpu, r.CPU), append(l.mem, r.Mem)
			by[i][ago] = l
		}
		if len(by[i]) == 0 {
			return nil
		}
		from = min(from, slices.Max(slices.Collect(maps.Keys(by[i]))))
	}
	if from < 1 {
		return nil
	}
	mean := func(v []float64) float64 {
		var sum float64
		for _, x := range v {
			sum += x
		}
		return math.Round(10*sum/float64(len(v))) / 10
	}
	cpu := Chart{Area: area, Title: ProcessorChart, Measures: true}
	mem := Chart{Area: area, Title: MemoryChart, Measures: true}
	for i, name := range names {
		// From the host's own first reading, so that it has one by from.
		var c, m []float64
		var lastCPU, lastMem float64
		for ago := slices.Max(slices.Collect(maps.Keys(by[i]))); ago >= 0; ago-- {
			if l, ok := by[i][ago]; ok {
				lastCPU, lastMem = mean(l.cpu), mean(l.mem)
			}
			if ago <= from {
				c, m = append(c, lastCPU), append(m, lastMem)
			}
		}
		label := Text{Name: name}
		cpu.Series = append(cpu.Series, Series{Label: label, Hours: c})
		mem.Series = append(mem.Series, Series{Label: label, Hours: m})
	}
	return []Chart{cpu, mem}
}
