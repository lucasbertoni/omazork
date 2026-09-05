# Action waits are stored in seconds

Every action-wait duration on disk — generated tables, overlays, the shared verb
defaults, and the calibration thresholds and reports — is an integer count of
**seconds**, under `schemaVersion: 2`. Until [#42](https://github.com/lucasbertoni/omazork/issues/42)
it was integer minutes. The migration multiplied every value by 60; no wait changed
length. The spec ([docs/action-waits.md](../action-waits.md) §2) keeps writing bands,
caps, and thresholds in minutes, because that is the unit people reason about, and
the code expresses them as `N * Minute`.

The driver is granularity: sub-minute waits (a quick grab, a short step) have no
representation in a minutes table, and the mediation layer already measures in
`time.Duration`. Switching the storage unit now, while every value is a clean
multiple of 60, costs one mechanical diff; switching later, once curators have
hand-tuned rows, would not.

## Considered options

- **Keep minutes, add a fractional or separate `seconds` field**: rejected — two
  units in one row invites mixing them, and a float duration invites drift in a
  table that is otherwise byte-stable across regeneration.
- **Neutral field name (`wait`, `duration`) with the unit fixed by schema**:
  rejected — the file convention already puts units in names
  (`cumulativeHours`, `…Seconds`), and an explicit `seconds` makes a stale
  minute-valued file fail loudly instead of reading sixty times too short.
- **Accept version-1 files and convert on load**: rejected — every file is
  committed and regenerated together, and nothing outside this repo produces
  them, so a compatibility shim is dead code. Version 1 is refused.
- **Move the LLM contract to seconds too**: rejected — the model judges at
  minute granularity anyway, and the prompt is hashed into every cached row's
  `inputHash`, so the change would have forced a full re-inference run for no
  gain. `<game>.llm.json` keeps `minutes`; the generator multiplies on write.
