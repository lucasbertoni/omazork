package calibrate

import (
	"sort"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// Histogram is the whole-table shape (§6), over the layered rows.
type Histogram struct {
	Edges         Distribution         `json:"edges"`
	Actions       Distribution         `json:"actions"`
	Classes       map[string]ClassStat `json:"classes"`
	DramaticShare float64              `json:"dramaticShare"` // Dramatic-class share of (verb, object) rows
	LongRows      []string             `json:"longRows"`      // keys of rows at or above the reason threshold
}

// Distribution summarizes one population of rows.
type Distribution struct {
	Rows   int     `json:"rows"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Bins   []Bin   `json:"bins"`
}

// ClassStat is one class's median against its §2 band and the inner half of
// that band the median is gated to.
type ClassStat struct {
	Rows      int     `json:"rows"`
	Median    float64 `json:"median"`
	BandLow   int     `json:"bandLow"`
	BandHigh  int     `json:"bandHigh"`
	InnerLow  float64 `json:"innerLow"`
	InnerHigh float64 `json:"innerHigh"`
}

// histogram profiles the layered rows: distributions per shape, medians per
// class against the inner half of the class band, the Dramatic share, and the
// rows at or above the reason line.
func histogram(edges, acts []durations.KeyedRow) Histogram {
	h := Histogram{Classes: map[string]ClassStat{}, LongRows: []string{}}
	h.Edges = distribution(edges)
	h.Actions = distribution(acts)

	byClass := map[durations.Class][]int{}
	dramatic := 0
	for _, rows := range [][]durations.KeyedRow{edges, acts} {
		for _, r := range rows {
			byClass[r.Class] = append(byClass[r.Class], r.Minutes)
			if r.Minutes >= durations.ReasonThreshold {
				h.LongRows = append(h.LongRows, r.Key)
			}
		}
	}
	for _, r := range acts {
		if r.Class == durations.ClassDramatic {
			dramatic++
		}
	}
	h.DramaticShare = share(dramatic, len(acts))
	for class, minutes := range byClass {
		band, ok := durations.BandOf(class)
		if !ok {
			continue
		}
		sort.Ints(minutes)
		quarter := float64(band.High-band.Low) / 4
		h.Classes[string(class)] = ClassStat{
			Rows: len(minutes), Median: median(minutes),
			BandLow: band.Low, BandHigh: band.High,
			InnerLow: float64(band.Low) + quarter, InnerHigh: float64(band.High) - quarter,
		}
	}
	return h
}

// distribution summarizes one population of rows.
func distribution(rows []durations.KeyedRow) Distribution {
	minutes := make([]int, 0, len(rows))
	counts := map[int]int{}
	for _, r := range rows {
		minutes = append(minutes, r.Minutes)
		counts[r.Minutes]++
	}
	sort.Ints(minutes)
	return Distribution{Rows: len(rows), Mean: mean(minutes), Median: median(minutes), Bins: bins(counts)}
}
