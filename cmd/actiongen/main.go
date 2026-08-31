// actiongen is the offline duration generator (docs/action-waits.md §5.2): it
// rule-prices data/extract/<game>.json into the committed duration table
// data/actions/<game>.json, then validates the table against the game's
// overlay and the shared verb defaults. -check regenerates without writing and
// fails on any drift, which is what CI runs.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/gen"
	"github.com/lucasbertoni/omazork/internal/extract"
)

var games = []string{"zork1", "zork2", "zork3"}

func main() {
	game := flag.String("game", "", "game to generate: zork1, zork2, or zork3 (default all three)")
	root := flag.String("root", ".", "repository root holding data/")
	check := flag.Bool("check", false, "regenerate in memory and fail on any difference from the committed table")
	flag.Parse()

	targets := games
	if *game != "" {
		targets = []string{*game}
	}
	failed := false
	for _, g := range targets {
		if err := run(*root, g, *check); err != nil {
			fmt.Fprintln(os.Stderr, "actiongen:", err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func run(root, game string, check bool) error {
	fsys := os.DirFS(root)
	verbs, err := durations.LoadVerbsFS(fsys)
	if err != nil {
		return err
	}
	// Read the shared thresholds only to assert their schemaVersion here: a
	// stale calibration file should stop a regeneration, not surface later.
	if _, err := durations.LoadCalibrationFS(fsys); err != nil {
		return err
	}
	x, err := readExtract(filepath.Join(root, "data", "extract", game+".json"))
	if err != nil {
		return err
	}
	table, err := gen.Generate(x, verbs)
	if err != nil {
		return err
	}
	overlay, err := durations.LoadOverlayFS(fsys, game)
	if err != nil {
		return err
	}
	if errs := durations.Validate(table, overlay, verbs); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "actiongen:", game+":", e)
		}
		return fmt.Errorf("%s: %d validation errors", game, len(errs))
	}
	data, err := table.JSON()
	if err != nil {
		return err
	}
	out := filepath.Join(root, "data", "actions", game+".json")
	if check {
		committed, err := os.ReadFile(out)
		if err != nil {
			return err
		}
		if !bytes.Equal(committed, data) {
			return fmt.Errorf("%s is stale — regenerate with actiongen", out)
		}
		fmt.Printf("%s: table matches the committed file (%d edges)\n", game, len(table.Edges))
		return nil
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d rooms, %d edges, %d actions, %d verb words → %s (%d routine exits deferred to the LLM pass)\n",
		game, len(table.Rooms), len(table.Edges), len(table.Actions), len(table.Vocab.Verbs), out, routineExits(x))
	return nil
}

// routineExits counts the PER-routine exits no rule can price: they have no
// static destination, so they never become rows and wait on full LLM
// classification (§5.3).
func routineExits(x *extract.Extract) int {
	n := 0
	for _, e := range x.Edges {
		if e.Kind == extract.KindRoutine && e.To == "" {
			n++
		}
	}
	return n
}

func readExtract(path string) (*extract.Extract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var x extract.Extract
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &x, nil
}
