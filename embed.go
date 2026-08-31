// Package omazork embeds the assets the wrapper binary ships with: the three
// authentic story files, the curated wait/achievement tables, and the action
// duration tables (generated, overlay, and shared) the mediation layer prices
// waits from.
package omazork

import "embed"

//go:embed assets/games/zork1.z3 assets/games/zork2.z3 assets/games/zork3.z3
var Games embed.FS

// The action files are listed one by one on purpose: the wrapper loads only
// the generated tables, the overlays, and the two shared files. The generator's
// own artifacts — the LLM cache and the calibration reports — are committed
// next to them but have no business inside the binary.
//
//go:embed data/waits/*.json data/achievements/*.json
//go:embed data/actions/zork1.json data/actions/zork2.json data/actions/zork3.json
//go:embed data/actions/*.overlay.json data/actions/verbs.json data/actions/calibration.json
var Data embed.FS
