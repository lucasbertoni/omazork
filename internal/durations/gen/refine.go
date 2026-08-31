package gen

import (
	"fmt"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/llm"
	"github.com/lucasbertoni/omazork/internal/extract"
)

// The LLM pass (§5.3) layered onto the rule pricing: static edges are nudged
// inside the Movement band around their rule baseline, routine-typed edges and
// handler pairs are classified outright. Every answer comes from the committed
// cache — this file never calls the API; it only decides what a call would
// have to cover.

// applyEdge renders an edge's prompt, records the request, and folds a fresh
// cached answer into the row.
func applyEdge(x *extract.Extract, rooms map[string]*extract.Room, p pricedEdge, cache *llm.Cache, result *Result) (durations.EdgeRow, error) {
	row := p.row
	payload, kind := edgePayload(x, rooms, p)
	req := llm.NewRequest(durations.EdgeKey(row.From, row.To), kind, payload.Render(kind))
	result.Requests = append(result.Requests, req)

	if answer, ok := cache.Fresh(req); ok {
		if err := llm.Check(kind, answer.Class, answer.Minutes); err != nil {
			return row, fmt.Errorf("gen: %s: cached answer for %s: %w", x.Game, req.Key, err)
		}
		minutes, class, nomination := capNomination(answer)
		row.Minutes = minutes
		row.Class = class
		row.Source = durations.SourceRuleLLM
		arith := p.arith + fmt.Sprintf(", llm %d", minutes)
		if nomination != "" {
			arith += ", " + nomination
		}
		row.Note = joinNote(arith, p.collision, answer.Rationale)
		return row, nil
	}
	result.Missing = append(result.Missing, req)
	row.Note = joinNote(p.arith, p.collision, "")
	return row, nil
}

// applyHandler classifies one (verb, object) pair. Without a cached answer
// there is no row at all: the rules have nothing to say about a handler, and a
// made-up duration would be worse than the verb default the runtime falls back
// to.
func applyHandler(x *extract.Extract, pr pair, cache *llm.Cache, result *Result) (durations.ActionRow, bool, error) {
	payload := llm.HandlerPayload{
		Game:          x.Game,
		Verb:          pr.verb,
		Object:        pr.object.ID,
		ObjectName:    pr.object.Name,
		ObjectDesc:    objectDesc(pr.object),
		Syntax:        pr.syntax,
		Handler:       pr.handler,
		HandlerSource: pr.source,
	}
	key := durations.ActionKey(pr.verb, pr.object.ID)
	req := llm.NewRequest(key, llm.KindHandler, payload.Render())
	result.Requests = append(result.Requests, req)

	answer, ok := cache.Fresh(req)
	if !ok {
		result.Missing = append(result.Missing, req)
		return durations.ActionRow{}, false, nil
	}
	if err := llm.Check(llm.KindHandler, answer.Class, answer.Minutes); err != nil {
		return durations.ActionRow{}, false, fmt.Errorf("gen: %s: cached answer for %s: %w", x.Game, key, err)
	}
	minutes, class, nomination := capNomination(answer)
	return durations.ActionRow{
		Verb: pr.verb, Object: pr.object.ID,
		Minutes: minutes, Class: class, Source: durations.SourceLLM,
		Note: joinNote(nomination, "", answer.Rationale),
	}, true, nil
}

// capNomination enforces §2's "drama requires a human signature" on every
// generated row, whatever its kind: a Dramatic nomination — or any answer
// above the uncurated ceiling — is written at the cap and carries the note
// that puts it in the curation queue. The cache keeps the nomination raw; only an overlay row can
// let it run its full length. The returned note is empty when nothing was
// capped.
func capNomination(answer llm.Row) (minutes int, class durations.Class, nomination string) {
	if answer.Class != durations.ClassDramatic && answer.Minutes <= durations.UncuratedCap {
		return answer.Minutes, answer.Class, ""
	}
	minutes = answer.Minutes
	if minutes > durations.UncuratedCap {
		minutes = durations.UncuratedCap
	}
	return minutes, answer.Class,
		fmt.Sprintf("llm nominated %s %dm — overlay candidate", answer.Class, answer.Minutes)
}

// joinNote renders a row's note in one order every curator can read: the
// arithmetic, then any exit collision, then the model's own sentence.
func joinNote(arith, collision, rationale string) string {
	note := arith
	if collision != "" {
		note += "; " + collision
	}
	if rationale != "" {
		if note == "" {
			return rationale
		}
		note += " — " + rationale
	}
	return note
}

// edgePayload assembles an edge's §5.3 payload and says which template prices
// it: a passage whose exit runs a routine is classified outright, everything
// else is a nudge around the rule baseline.
func edgePayload(x *extract.Extract, rooms map[string]*extract.Room, p pricedEdge) (llm.EdgePayload, llm.Kind) {
	from, to := rooms[p.row.From], rooms[p.row.To]
	band, _ := durations.BandOf(durations.ClassMovement)
	payload := llm.EdgePayload{
		Game:         x.Game,
		From:         p.row.From,
		To:           p.row.To,
		FromName:     from.Name,
		ToName:       to.Name,
		FromDesc:     from.LDesc,
		ToDesc:       to.LDesc,
		FromFlags:    from.Flags,
		ToFlags:      to.Flags,
		Dir:          p.edge.Dir,
		Kind:         p.row.Kind,
		Baseline:     p.row.Minutes,
		BaselineNote: p.arith,
		BandLow:      band.Low,
		BandHigh:     band.High,
	}
	if p.edge.Kind == extract.KindRoutine {
		payload.Routine = p.edge.Per
		payload.Source = routineSource(x, p.edge.Per)
		return payload, llm.KindRoutineEdge
	}
	return payload, llm.KindEdge
}

// routineSource returns a named routine's verbatim text, or "" when the
// extract did not capture it.
func routineSource(x *extract.Extract, name string) string {
	for _, r := range x.Routines {
		if r.Name == name {
			return r.Source
		}
	}
	return ""
}

// objectDesc picks the fullest description the extract holds for an object:
// its long description, else the first-appearance one, else its read text.
func objectDesc(o *extract.Object) string {
	for _, s := range []string{o.LDesc, o.FDesc, o.Text} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
