// zilextract is the offline ZIL extractor (docs/action-waits.md §5.1): it
// reads a historicalsource game repo and the shipped story file, and writes
// data/extract/<game>.json — rooms with correlated object numbers, typed
// edges, syntax, handlers, and raw routine text. scripts/extract.sh fetches
// the pinned source commit and runs this.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucasbertoni/omazork/internal/extract"
)

func main() {
	game := flag.String("game", "", "game to extract: zork1, zork2, or zork3")
	src := flag.String("src", "", "path to the historicalsource checkout (required)")
	story := flag.String("story", "", "story file (default assets/games/<game>.z3)")
	out := flag.String("out", "", "output file (default data/extract/<game>.json)")
	flag.Parse()

	if *game == "" || *src == "" {
		fmt.Fprintln(os.Stderr, "usage: zilextract -game zork1 -src <historicalsource checkout> [-story file.z3] [-out file.json]")
		os.Exit(2)
	}
	if *story == "" {
		*story = filepath.Join("assets", "games", *game+".z3")
	}
	if *out == "" {
		*out = filepath.Join("data", "extract", *game+".json")
	}

	mem, err := os.ReadFile(*story)
	if err != nil {
		fatal(err)
	}
	x, err := extract.Run(*game, *src, mem)
	if err != nil {
		fatal(err)
	}
	data, err := x.JSON()
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal(err)
	}

	kinds := map[string]int{}
	for _, e := range x.Edges {
		kinds[e.Kind]++
	}
	fmt.Printf("%s: %d rooms, %d edges (plain %d, blocked %d, cond_flag %d, cond_door %d, routine %d, drift %d), %d objects, %d syntax, %d routines → %s\n",
		*game, len(x.Rooms), len(x.Edges), kinds["plain"], kinds["blocked"], kinds["cond_flag"],
		kinds["cond_door"], kinds["routine"], kinds["drift"], len(x.Objects), len(x.Syntax), len(x.Routines), *out)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "zilextract:", err)
	os.Exit(1)
}
