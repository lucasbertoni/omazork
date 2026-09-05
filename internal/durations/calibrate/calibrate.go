// Package calibrate is the proof that a generated duration table feels right
// (docs/action-waits.md §6, §7): the committed walkthrough fixture replayed
// through the real seeded engine and the shared matcher, every turn priced
// exactly as the runtime prices it against the layered table; whole-table
// histograms; the shared thresholds evaluated as pass/fail gates; and the
// curation queue. The result is data/actions/<game>.calibration.json.
//
// Everything here evaluates the layered result — base + overlay + shared
// verb defaults — so an overlay edit can fail a gate. Otherwise the overlay
// would be a gate bypass.
package calibrate

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/durations"
)

// SchemaVersion is stamped on the report, like every duration file (§4).
const SchemaVersion = 2

// Input is everything one calibration run reads.
type Input struct {
	Layered    *durations.Layered
	Thresholds *durations.Calibration
	Fixture    actions.Fixture
	Script     []byte // the fixture's walkthrough, already read

	// Hashes maps every generated row key to the inputHash of the LLM request
	// that prices it — what an acknowledgement pins (§7).
	Hashes map[string]string
	// Nominations maps a row key to the LLM pass's Dramatic nomination note,
	// for every row the generator wrote at the cap instead (§5.3).
	Nominations map[string]string
}

// Report is one game's data/actions/<game>.calibration.json.
type Report struct {
	SchemaVersion      int         `json:"schemaVersion"`
	Game               string      `json:"game"`
	Replay             Replay      `json:"replay"`
	Histogram          Histogram   `json:"histogram"`
	Gates              []Gate      `json:"gates"`
	Coverage           Coverage    `json:"coverage"`
	CurationCandidates []Candidate `json:"curationCandidates"`
}

// Coverage is the non-gating share of layered rows the walkthrough touched.
type Coverage struct {
	Edges            int     `json:"edges"`
	EdgesExercised   int     `json:"edgesExercised"`
	Actions          int     `json:"actions"`
	ActionsExercised int     `json:"actionsExercised"`
	Share            float64 `json:"share"`
}

// Run replays the fixture, profiles the table, evaluates the gates, and builds
// the curation queue. A returned error means the replay itself could not run;
// gate breaches are reported in the Report and surfaced by Failures.
func Run(in Input) (*Report, error) {
	replay, hit, err := replay(in.Layered, in.Fixture, in.Script)
	if err != nil {
		return nil, err
	}
	edges, acts := in.Layered.Rows()
	hist := histogram(edges, acts)
	cov := Coverage{Edges: len(edges), Actions: len(acts)}
	for _, e := range edges {
		if hit[e.Key] {
			cov.EdgesExercised++
		}
	}
	for _, a := range acts {
		if hit[a.Key] {
			cov.ActionsExercised++
		}
	}
	cov.Share = share(cov.EdgesExercised+cov.ActionsExercised, cov.Edges+cov.Actions)
	return &Report{
		SchemaVersion:      SchemaVersion,
		Game:               in.Layered.Game,
		Replay:             replay,
		Histogram:          hist,
		Gates:              evaluate(replay, hist, in.Thresholds),
		Coverage:           cov,
		CurationCandidates: candidates(edges, acts, in.Layered.Acknowledged(), in.Hashes, in.Nominations),
	}, nil
}

// Failures returns one error per breached gate. Every threshold is a
// generation-failing validation error (§6), the same severity as an orphaned
// overlay key.
func (r *Report) Failures() []error {
	var errs []error
	for _, g := range r.Gates {
		if !g.Pass {
			errs = append(errs, fmt.Errorf("calibration gate %s: %s", g.Name, g.describe()))
		}
	}
	return errs
}

// JSON renders the report deterministically: the replay is seeded, rows are in
// key order, and maps encode with sorted keys, so an unchanged table yields an
// unchanged file — which is what the CI freshness check compares (§7).
func (r *Report) JSON() ([]byte, error) {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// share is a ratio rounded for the report; 0 when there is nothing to share.
func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return round(float64(part) / float64(whole))
}

// round keeps report floats readable and stable: three decimals is finer than
// any threshold.
func round(f float64) float64 { return math.Round(f*1000) / 1000 }

// median of a slice of seconds, as the mean of the two middle values for an
// even count; 0 for an empty slice.
func median(sorted []int) float64 {
	n := len(sorted)
	switch {
	case n == 0:
		return 0
	case n%2 == 1:
		return float64(sorted[n/2])
	default:
		return float64(sorted[n/2-1]+sorted[n/2]) / 2
	}
}

// mean of a slice of seconds, rounded; 0 for an empty slice.
func mean(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0
	for _, v := range values {
		sum += v
	}
	return round(float64(sum) / float64(len(values)))
}
