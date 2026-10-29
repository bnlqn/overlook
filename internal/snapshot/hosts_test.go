package snapshot

import (
	"slices"
	"testing"
	"time"
)

// Hosts' lines start when every host has a reading and end with this hour,
// each carrying its last over an hour without one, so that they're all as
// long.
func TestHostCharts(t *testing.T) {
	t.Parallel()
	at := func(hour int64, cpu float64) Reading {
		return Reading{At: time.Unix(hour*3600+60, 0), CPU: cpu, Mem: 50}
	}
	alder := []Reading{at(10, 10), at(11, 15), at(11, 25), at(13, 40)} // none at 12
	birch := []Reading{at(11, 50), at(12, 60), at(13, 70)}
	now := time.Unix(13*3600+1800, 0).UTC()
	c := HostCharts("servers", now, []string{"alder", "birch"}, [][]Reading{alder, birch})
	if len(c) != 2 || c[0].Title.Key != ProcessorChart.Key || c[1].Title.Key != MemoryChart.Key || !c[0].Measures || c[0].Area != "servers" {
		t.Fatalf("two charts that measure, the processor's and the memory's: %+v", c)
	}
	if got := c[0].Series[0].Hours; !slices.Equal(got, []float64{20, 20, 40}) || c[0].Series[0].Label.Name != "alder" {
		t.Errorf("alder from hour 11, the mean of its two, its 12th carrying the 11th: %v", got)
	}
	if got := c[0].Series[1].Hours; !slices.Equal(got, []float64{50, 60, 70}) {
		t.Errorf("birch from hour 11: %v", got)
	}
	if got := c[1].Series[0].Hours; !slices.Equal(got, []float64{50, 50, 50}) {
		t.Errorf("memory: %v", got)
	}
	if HostCharts("servers", now, []string{"alder", "cedar"}, [][]Reading{alder, nil}) != nil {
		t.Error("a host with no reading at all: no charts")
	}
	if HostCharts("servers", now, []string{"birch"}, [][]Reading{{at(13, 70)}}) != nil {
		t.Error("an hour alone: no charts")
	}
	later := HostCharts("servers", now.Add(2*time.Hour), []string{"alder"}, [][]Reading{alder})
	if got := later[0].Series[0].Hours; !slices.Equal(got, []float64{10, 20, 20, 40, 40, 40}) {
		t.Errorf("on to this hour, carrying the last: %v", got)
	}
}

// Hosts' hours are Overlook's: from the half hour in a half-hour zone.
func TestHostChartsHalfHourZone(t *testing.T) {
	t.Parallel()
	at := func(clock string, cpu float64) Reading {
		t, _ := time.Parse(time.TimeOnly, clock)
		return Reading{At: time.Date(2026, 10, 29, t.Hour(), t.Minute(), 0, 0, time.UTC), CPU: cpu, Mem: 50}
	}
	readings := [][]Reading{{at("10:10:00", 10), at("10:50:00", 30)}}
	now := time.Date(2026, 10, 29, 10, 55, 0, 0, time.UTC)
	if HostCharts("servers", now, []string{"alder"}, readings) != nil {
		t.Error("one hour of UTC's: no charts")
	}
	c := HostCharts("servers", now.In(zone(t, "Asia/Kolkata")), []string{"alder"}, readings)
	if c == nil || !slices.Equal(c[0].Series[0].Hours, []float64{10, 30}) {
		t.Errorf("two of Kolkata's, either side of 16:00: %+v", c)
	}
}
