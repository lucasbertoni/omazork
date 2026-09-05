package actions

import "path/filepath"

// Fixture is one committed walkthrough: the script's repo-relative path and
// the engine seed it was tuned against (spec §6.1). The pair is the
// calibration ground truth — change either and the replay is a different
// walkthrough.
type Fixture struct {
	Script string
	Seed   uint64
}

// Fixtures maps each game to its committed walkthrough.
var Fixtures = map[string]Fixture{
	"zork1": {Script: "internal/actions/testdata/zork1-walkthrough.txt", Seed: 12},
	"zork2": {Script: "internal/actions/testdata/zork2-walkthrough.txt", Seed: 1},
	"zork3": {Script: "internal/actions/testdata/zork3-walkthrough.txt", Seed: 11},
}

// Path is the script's location under a repository root.
func (f Fixture) Path(root string) string { return filepath.Join(root, filepath.FromSlash(f.Script)) }
