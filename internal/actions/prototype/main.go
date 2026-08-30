// THROWAWAY PROTOTYPE (wayfinder ticket #22) — not production code.
//
// Two jobs:
//  1. `go run ./internal/actions/prototype capture` — drive the real engine
//     through scripted scenarios with a fixed seed and emit per-turn JSON
//     (input, output, room name, room objnum, moves) for the HTML matcher demo.
//  2. `go run ./internal/actions/prototype objects <game>` — dump the story
//     file's object table (objnum → short name) and report duplicate room
//     names, proving the ZIL-id→objnum correlation technique from ticket #21.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/maloquacious/quetzal"

	omazork "github.com/lucasbertoni/omazork"
	"github.com/lucasbertoni/omazork/internal/engine"
)

type turnRec struct {
	Input   string `json:"input"` // "" for the opening banner
	Output  string `json:"output"`
	Room    string `json:"room"`    // status-line name after the turn
	RoomObj uint16 `json:"roomObj"` // global 0 after the turn
	Score   int    `json:"score"`
	Moves   int    `json:"moves"`
}

func main() {
	switch cmd := arg(1); cmd {
	case "capture":
		capture(arg(2), arg(3), readScript())
	case "objects":
		objects(arg(2))
	default:
		fmt.Fprintln(os.Stderr, "usage: prototype capture <game> <seed-label> < script.txt | prototype objects <game>")
		os.Exit(2)
	}
}

func arg(n int) string {
	if len(os.Args) > n {
		return os.Args[n]
	}
	return ""
}

func readScript() []string {
	data, err := os.ReadFile("/dev/stdin")
	must(err)
	var cmds []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		cmds = append(cmds, l)
	}
	return cmds
}

// capture runs the command script against a seeded engine and prints one
// JSON array of turn records (banner first).
func capture(game, seedLabel string, cmds []string) {
	var seed uint64 = 1
	fmt.Sscanf(seedLabel, "%d", &seed)
	e, err := engine.New(game, engine.WithSeed(seed))
	must(err)
	story, err := omazork.Games.ReadFile("assets/games/" + game + ".z3")
	must(err)
	qstory, err := quetzal.ParseStory(story)
	must(err)

	peekRoom := func(state []byte) uint16 {
		if len(state) == 0 {
			return 0
		}
		f, err := quetzal.Decode(bytes.NewReader(state))
		must(err)
		mem, err := f.Memory(qstory)
		must(err)
		globals := binary.BigEndian.Uint16(mem.Data[0x0C:])
		return binary.BigEndian.Uint16(mem.Data[int(globals):])
	}

	var recs []turnRec
	t, err := e.Start()
	must(err)
	recs = append(recs, turnRec{Output: t.Output, Room: t.Room, RoomObj: peekRoom(t.State), Score: t.Score, Moves: t.Moves})
	for _, c := range cmds {
		t, err = e.Run(c)
		must(err)
		recs = append(recs, turnRec{Input: c, Output: t.Output, Room: t.Room, RoomObj: peekRoom(t.State), Score: t.Score, Moves: t.Moves})
		if t.Halted {
			break
		}
	}
	out, err := json.MarshalIndent(recs, "", " ")
	must(err)
	fmt.Println(string(out))
}

// objects dumps objnum → short name straight from the story file (v3 object
// table, Z-Standard §12) and reports duplicated names.
func objects(game string) {
	mem, err := omazork.Games.ReadFile("assets/games/" + game + ".z3")
	must(err)
	objTable := int(binary.BigEndian.Uint16(mem[0x0A:]))
	abbrevTable := int(binary.BigEndian.Uint16(mem[0x18:]))

	var zstring func(addr int) string
	zstring = func(addr int) string {
		var zc []byte
		for {
			w := binary.BigEndian.Uint16(mem[addr:])
			zc = append(zc, byte(w>>10&31), byte(w>>5&31), byte(w&31))
			addr += 2
			if w&0x8000 != 0 {
				break
			}
		}
		const a0 = "abcdefghijklmnopqrstuvwxyz"
		const a1 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		const a2 = " \n0123456789.,!?_#'\"/\\-:()"
		var sb strings.Builder
		alpha := 0
		for i := 0; i < len(zc); i++ {
			c := zc[i]
			switch {
			case c == 0:
				sb.WriteByte(' ')
			case c >= 1 && c <= 3: // abbreviation
				if i+1 < len(zc) {
					i++
					idx := 32*(int(c)-1) + int(zc[i])
					waddr := abbrevTable + 2*idx
					sb.WriteString(zstring(2 * int(binary.BigEndian.Uint16(mem[waddr:]))))
				}
			case c == 4:
				alpha = 1
				continue
			case c == 5:
				alpha = 2
				continue
			case c == 6 && alpha == 2: // ZSCII escape
				if i+2 < len(zc) {
					sb.WriteByte(byte(zc[i+1]<<5 | zc[i+2]))
					i += 2
				}
			default:
				switch alpha {
				case 0:
					sb.WriteByte(a0[c-6])
				case 1:
					sb.WriteByte(a1[c-6])
				case 2:
					sb.WriteByte(a2[c-6])
				}
			}
			alpha = 0
		}
		return sb.String()
	}

	// v3: 31 default words, then 9-byte entries; the table ends where the
	// lowest property table begins.
	first := objTable + 62
	minProps := len(mem)
	names := map[int]string{}
	for n, off := 1, first; off+9 <= minProps; n, off = n+1, off+9 {
		props := int(binary.BigEndian.Uint16(mem[off+7:]))
		if props > 0 && props < len(mem) {
			minProps = min(minProps, props)
			if int(mem[props]) > 0 {
				names[n] = zstring(props + 1)
			}
		}
	}
	dup := map[string][]int{}
	for n, name := range names {
		dup[name] = append(dup[name], n)
	}
	fmt.Printf("%s: %d named objects\n", game, len(names))
	fmt.Println("duplicated short names (rooms and objects mixed):")
	for name, ns := range dup {
		if len(ns) > 1 {
			fmt.Printf("  %-24q ×%d %v\n", name, len(ns), ns)
		}
	}
	out, _ := json.MarshalIndent(names, "", " ")
	_ = os.WriteFile("internal/actions/prototype/"+game+"-objects.json", out, 0o644)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
