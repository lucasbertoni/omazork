package calibrate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// Gate is one threshold from the shared calibration.json evaluated against
// this game: the measured value, the bound(s) it must sit inside, and the
// verdict. Min and Max are omitted when the threshold is one-sided.
type Gate struct {
	Name  string   `json:"name"`
	Value float64  `json:"value"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	Pass  bool     `json:"pass"`
}

func (g Gate) describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%g", g.Value)
	switch {
	case g.Min != nil && g.Max != nil:
		fmt.Fprintf(&b, " is outside %g–%g", *g.Min, *g.Max)
	case g.Min != nil:
		fmt.Fprintf(&b, " is below %g", *g.Min)
	case g.Max != nil:
		fmt.Fprintf(&b, " is above %g", *g.Max)
	}
	return b.String()
}

// evaluate runs every §6 threshold over the replay profile and the histogram.
// The class-median gate covers each class the table has rows for, except
// Dramatic: an uncurated nomination is written at the 30-minute cap by
// construction (§2), so its median can never reach the Dramatic band. That
// class is gated by its share of rows instead, and every nomination is
// already in the curation queue.
func evaluate(rep Replay, h Histogram, c *durations.Calibration) []Gate {
	gates := []Gate{
		between("replay.cumulativeHours", rep.CumulativeHours, c.Replay.CumulativeHoursMin, c.Replay.CumulativeHoursMax),
		between("replay.medianMovementSeconds", rep.MedianMovementSeconds,
			float64(c.Replay.MedianMovementSecondsMin), float64(c.Replay.MedianMovementSecondsMax)),
		atLeast("replay.turnsUnderQuiet", rep.TurnsUnderQuiet, c.Replay.TurnsUnderQuietMin),
		between("histogram.meanEdgeSeconds", h.Edges.Mean, c.Histogram.MeanEdgeSecondsMin, c.Histogram.MeanEdgeSecondsMax),
		atMost("histogram.dramaticShare", h.DramaticShare, c.Histogram.DramaticRowsMax),
	}
	if c.Histogram.ClassMediansInner {
		classes := make([]string, 0, len(h.Classes))
		for class := range h.Classes {
			if class != string(durations.ClassDramatic) {
				classes = append(classes, class)
			}
		}
		sort.Strings(classes)
		for _, class := range classes {
			s := h.Classes[class]
			gates = append(gates, between("histogram.classMedian."+class, s.Median, s.InnerLow, s.InnerHigh))
		}
	}
	gates = append(gates, atMost("rows.longRows", float64(len(h.LongRows)), float64(c.Rows.LongRowsMax)))
	return gates
}

// between, atLeast, and atMost build a two-sided, lower-bound, or upper-bound
// gate respectively.
func between(name string, value, lo, hi float64) Gate {
	return Gate{Name: name, Value: value, Min: &lo, Max: &hi, Pass: value >= lo && value <= hi}
}

func atLeast(name string, value, lo float64) Gate {
	return Gate{Name: name, Value: value, Min: &lo, Pass: value >= lo}
}

func atMost(name string, value, hi float64) Gate {
	return Gate{Name: name, Value: value, Max: &hi, Pass: value <= hi}
}
