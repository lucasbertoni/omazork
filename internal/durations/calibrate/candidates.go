package calibrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// Candidate is one row queued for a curator's eye (§7), with every reason it
// qualified and the inputHash an acknowledgement must pin to dismiss it.
type Candidate struct {
	Key       string          `json:"key"`
	Kind      string          `json:"kind"` // edge | action
	Seconds   int             `json:"seconds"`
	Class     durations.Class `json:"class"`
	Source    string          `json:"source"`
	Note      string          `json:"note,omitempty"`
	InputHash string          `json:"inputHash"`
	Reasons   []string        `json:"reasons"`
}

// outliers is how many of the longest rows of each shape the queue nominates.
const outliers = 10

// candidates builds the curation queue: (a) the LLM pass's Dramatic
// nominations, (b) statistical outliers — the longest edges, the longest
// (verb, object) rows, every row sitting at its class cap — (c) minus keys the
// overlay acknowledges at their current inputHash. An acknowledgement whose
// hash no longer matches does not suppress the row: the input changed under
// the curator's signature, so the row comes back with that said.
func candidates(edges, acts []durations.KeyedRow, acked, hashes, nominations map[string]string) []Candidate {
	reasons := map[string][]string{}
	nominate := func(key, why string) { reasons[key] = append(reasons[key], why) }

	for key, note := range nominations {
		nominate(key, "dramatic nomination: "+note)
	}
	for _, r := range longest(edges) {
		nominate(r.Key, fmt.Sprintf("top-%d longest edges", outliers))
	}
	for _, r := range longest(acts) {
		nominate(r.Key, fmt.Sprintf("top-%d longest action rows", outliers))
	}
	for _, rows := range [][]durations.KeyedRow{edges, acts} {
		for _, r := range rows {
			if band, ok := durations.BandOf(r.Class); ok && band.Cap > 0 && r.Seconds == band.Cap {
				nominate(r.Key, fmt.Sprintf("at the %s class cap of %ds", r.Class, band.Cap))
			}
		}
	}

	rows := map[string]durations.KeyedRow{}
	for _, r := range append(append([]durations.KeyedRow(nil), edges...), acts...) {
		rows[r.Key] = r
	}

	var out []Candidate
	for key, why := range reasons {
		row, ok := rows[key]
		if !ok {
			continue // a nomination for a row the layered table no longer has
		}
		hash := rowHash(row, hashes)
		if pinned, ok := acked[key]; ok {
			if pinned == hash {
				continue
			}
			why = append(why, fmt.Sprintf("acknowledged at %s but the input has changed since", short(pinned)))
		}
		sort.Strings(why)
		out = append(out, Candidate{
			Key: key, Kind: kindOf(key), Seconds: row.Seconds, Class: row.Class, Source: row.Source, Note: row.Note,
			InputHash: hash, Reasons: why,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seconds != out[j].Seconds {
			return out[i].Seconds > out[j].Seconds
		}
		return out[i].Key < out[j].Key
	})
	if out == nil {
		out = []Candidate{}
	}
	return out
}

// longest returns the top rows by duration, ties broken by key so the queue is
// stable across runs.
func longest(rows []durations.KeyedRow) []durations.KeyedRow {
	sorted := append([]durations.KeyedRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seconds > sorted[j].Seconds })
	if len(sorted) > outliers {
		sorted = sorted[:outliers]
	}
	return sorted
}

// rowHash is what an acknowledgement pins: the LLM request hash for a row the
// generator prices, else — for a row the overlay added on its own — a digest of
// the row itself, so an edit to that row re-surfaces it too.
func rowHash(row durations.KeyedRow, hashes map[string]string) string {
	if h, ok := hashes[row.Key]; ok {
		return h
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{"overlay", row.Key, fmt.Sprint(row.Seconds), string(row.Class), row.Note}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// kindOf reads a row's shape off its key prefix.
func kindOf(key string) string {
	if strings.HasPrefix(key, "edge:") {
		return "edge"
	}
	return "action"
}

// short abbreviates a hash for a human-readable reason.
func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
