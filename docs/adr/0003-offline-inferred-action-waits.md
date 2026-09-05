# Action waits are inferred offline and committed as deterministic tables

Casual-mode waits stopped keying on score deltas and now key on the action itself
(directed room edge, or verb + object), which requires a duration for every edge and
action in three games — far too many to hand-author. We infer them **offline** from
the historicalsource ZIL: rule-based pricing first, an LLM pass for the tail, with
the LLM's output committed as a hash-keyed cache (`data/actions/<game>.llm.json`) so
regeneration is deterministic and every re-inference is a reviewed diff. A
hand-curated overlay layers on top at load time and is the only place hour-scale
durations may be granted. Runtime never calls an LLM and ships no inference code —
release behavior is fixed by the committed tables. Full spec:
[docs/action-waits.md](../action-waits.md).

## Considered options

- **Runtime inference** (LLM or heuristics in the wrapper): rejected — network
  dependency and nondeterminism in a shipped game, un-reviewable pricing.
- **Fully hand-authored tables**: rejected — thousands of rows across three games;
  the overlay keeps hand-authorship where it matters (drama, fixes).
- **Keeping score-keyed waits**: rejected — score events are sparse and unrelated
  to what the action plausibly takes; the old wait tables survive only as pure
  score-*event* tables for achievements (`data/events/`).
