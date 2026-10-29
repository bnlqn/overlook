package snapshot

import (
	"slices"
	"testing"
	"time"
)

// Hosts' lines start when every host has a reading, each carrying its last
// over an hour without one, so that they're all as long.
func TestHostCharts(t *testing.T) {
	t.Parallel()
	at := func(hour int64, cpu float64) Reading {
		return Reading{At: time.Unix(hour*3600+60, 0), CPU: cpu, Mem: 50}
	}
	alder := []Reading{at(10, 10), at(11, 15), at(11, 25), at(13, 40)} // none at 12
	birch := []Reading{at(11, 50), at(12, 60), at(13, 70)}
	c := HostCharts("servers", []string{"alder", "birch"}, [][]Reading{alder, birch})
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
	if HostCharts("servers", []string{"alder", "cedar"}, [][]Reading{alder, nil}) != nil {
		t.Error("a host with no reading at all: no charts")
	}
	if HostCharts("servers", []string{"birch"}, [][]Reading{{at(13, 70)}}) != nil {
		t.Error("an hour alone: no charts")
	}
}
