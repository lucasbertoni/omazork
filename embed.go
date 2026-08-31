// Package omazork embeds the assets the wrapper binary ships with: the three
// authentic story files, the curated wait/achievement tables, and the action
// duration tables (generated, overlay, and shared) the mediation layer prices
// waits from.
package omazork

import "embed"

//go:embed assets/games/zork1.z3 assets/games/zork2.z3 assets/games/zork3.z3
var Games embed.FS

//go:embed data/waits/*.json data/achievements/*.json data/actions/*.json
var Data embed.FS
