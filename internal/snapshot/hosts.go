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

// HostCharts are hosts' processor and memory by the hour in area, a line
// each named by names, from readings, each host's in names' order. They run
// over the hours from when every host has a reading to the last hour any
// has one: each hour the mean of its readings, and an hour with none takes
// the last one's. With a host without a reading, or under two hours, there
// are none.
//
// ponytail: an hour with no reading draws the last, which a line can't tell
// from a level that held. If gaps inside a week turn up, draw them as gaps.
func HostCharts(area string, names []string, readings [][]Reading) []Chart {
	type level struct{ cpu, mem []float64 }
	by := make([]map[int64]level, len(names))
	var from, to int64
	for i := range names {
		by[i] = map[int64]level{}
		for _, r := range readings[i] {
			hour := r.At.Unix() / 3600
			l := by[i][hour]
			l.cpu, l.mem = append(l.cpu, r.CPU), append(l.mem, r.Mem)
			by[i][hour] = l
		}
		if len(by[i]) == 0 {
			return nil
		}
		first, last := slices.Min(slices.Collect(maps.Keys(by[i]))), slices.Max(slices.Collect(maps.Keys(by[i])))
		from, to = max(from, first), max(to, last)
	}
	if to-from < 1 {
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
		for hour := slices.Min(slices.Collect(maps.Keys(by[i]))); hour <= to; hour++ {
			if l, ok := by[i][hour]; ok {
				lastCPU, lastMem = mean(l.cpu), mean(l.mem)
			}
			if hour >= from {
				c, m = append(c, lastCPU), append(m, lastMem)
			}
		}
		label := Text{Name: name}
		cpu.Series = append(cpu.Series, Series{Label: label, Hours: c})
		mem.Series = append(mem.Series, Series{Label: label, Hours: m})
	}
	return []Chart{cpu, mem}
}
