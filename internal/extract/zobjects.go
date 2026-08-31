package extract

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// This file resolves each ZIL room id to its z-machine object number by
// matching short names and exit structure against the story file's v3 object
// table (technique proven in the action-matcher prototype, ticket #22).
// Display names alone are not a key — "Maze" ×15 — so ambiguous names are
// narrowed by exit-property shape and target object numbers to a fixpoint.

// exitSize maps an edge kind to its z-machine exit-property length in bytes
// (gverbs.zil: UEXIT 1, NEXIT 2, FEXIT 3, CEXIT 4, DEXIT 5).
var exitSize = map[string]int{
	"plain":     1,
	"blocked":   2,
	"routine":   3,
	"cond_flag": 4,
	"cond_door": 5,
}

// exitTargetByte reports whether byte 0 of the exit property is the target
// room's object number (V-WALK: GETB .PT ,REXIT on UEXIT/CEXIT/DEXIT).
func exitTargetByte(kind string) bool {
	return kind == "plain" || kind == "cond_flag" || kind == "cond_door"
}

type zObject struct {
	name  string
	props map[int][]byte // propnum → data
}

// readZObjects parses the v3 object table (Z-Standard §12): 31 default
// words, then 9-byte entries until the lowest property table begins.
func readZObjects(mem []byte) (map[int]zObject, error) {
	if len(mem) < 0x40 {
		return nil, fmt.Errorf("story file too short (%d bytes)", len(mem))
	}
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
					sb.WriteByte(zc[i+1]<<5 | zc[i+2])
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

	first := objTable + 62
	minProps := len(mem)
	objs := map[int]zObject{}
	for n, off := 1, first; off+9 <= minProps; n, off = n+1, off+9 {
		propAddr := int(binary.BigEndian.Uint16(mem[off+7:]))
		if propAddr <= 0 || propAddr >= len(mem) {
			continue
		}
		minProps = min(minProps, propAddr)
		o := zObject{props: map[int][]byte{}}
		textLen := int(mem[propAddr])
		if textLen > 0 {
			o.name = zstring(propAddr + 1)
		}
		p := propAddr + 1 + 2*textLen
		for p < len(mem) && mem[p] != 0 {
			size := int(mem[p])
			num := size & 31
			length := size/32 + 1
			p++
			if p+length > len(mem) {
				return nil, fmt.Errorf("object %d: property %d overruns memory", n, num)
			}
			o.props[num] = mem[p : p+length]
			p += length
		}
		objs[n] = o
	}
	return objs, nil
}

// correlate fills in Room.Obj for every extracted room, or errors: every
// room must resolve to exactly one object number.
func (ex *extractor) correlate(story []byte) error {
	objs, err := readZObjects(story)
	if err != nil {
		return err
	}
	return ex.correlateObjects(objs)
}

func (ex *extractor) correlateObjects(objs map[int]zObject) error {
	exits := map[string]map[string]*Edge{} // room id → dir → edge
	for _, e := range ex.edges {
		if exits[e.From] == nil {
			exits[e.From] = map[string]*Edge{}
		}
		exits[e.From][e.Dir] = e
	}

	// Name candidates.
	byName := map[string][]int{}
	for num, o := range objs {
		if o.name != "" {
			byName[o.name] = append(byName[o.name], num)
		}
	}
	cands := map[string][]int{}
	for _, r := range ex.rooms {
		c := append([]int(nil), byName[r.Name]...)
		sort.Ints(c)
		if len(c) == 0 {
			return fmt.Errorf("room %s: no object named %q in the story file", r.ID, r.Name)
		}
		cands[r.ID] = c
	}

	dirProp, err := ex.solveDirectionProps(objs, exits, cands)
	if err != nil {
		return err
	}

	// Fixpoint: filter candidates by exit signature, assign singletons,
	// remove claimed object numbers from other rooms' candidate sets.
	assigned := map[string]int{}
	claimed := map[int]string{}
	for changed := true; changed; {
		changed = false
		for _, r := range ex.rooms {
			if _, done := assigned[r.ID]; done {
				continue
			}
			var keep []int
			for _, num := range cands[r.ID] {
				if owner, taken := claimed[num]; taken && owner != r.ID {
					continue
				}
				if compatible(objs[num], exits[r.ID], dirProp, assigned, cands) {
					keep = append(keep, num)
				}
			}
			if len(keep) != len(cands[r.ID]) {
				changed = true
			}
			cands[r.ID] = keep
			if len(keep) == 0 {
				return fmt.Errorf("room %s (%q): no story object matches its exit structure", r.ID, r.Name)
			}
			if len(keep) == 1 {
				assigned[r.ID] = keep[0]
				claimed[keep[0]] = r.ID
				changed = true
			}
		}
	}
	var unresolved []string
	for _, r := range ex.rooms {
		if _, ok := assigned[r.ID]; !ok {
			unresolved = append(unresolved, fmt.Sprintf("%s%v", r.ID, cands[r.ID]))
		}
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("ambiguous room↔object correlation: %s", strings.Join(unresolved, " "))
	}

	// Final strict verification, now that every target is resolved.
	for _, r := range ex.rooms {
		o := objs[assigned[r.ID]]
		for dir, prop := range dirProp {
			e := exits[r.ID][dir]
			data, has := o.props[prop]
			if e == nil {
				if has {
					return fmt.Errorf("room %s → obj %d: story has a %s exit the ZIL lacks", r.ID, assigned[r.ID], dir)
				}
				continue
			}
			if !has || len(data) != exitSize[e.Kind] {
				return fmt.Errorf("room %s → obj %d: %s exit shape mismatch", r.ID, assigned[r.ID], dir)
			}
			if exitTargetByte(e.Kind) && int(data[0]) != assigned[e.To] {
				return fmt.Errorf("room %s → obj %d: %s exit targets obj %d, ZIL says %s (obj %d)",
					r.ID, assigned[r.ID], dir, data[0], e.To, assigned[e.To])
			}
		}
	}

	for _, r := range ex.rooms {
		r.Obj = assigned[r.ID]
	}
	return nil
}

// solveDirectionProps discovers which z-machine property number encodes each
// direction, by voting across rooms whose short name is unique in the story
// file: the true property number is present with the right length exactly in
// the rooms that have that exit.
func (ex *extractor) solveDirectionProps(objs map[int]zObject, exits map[string]map[string]*Edge, cands map[string][]int) (map[string]int, error) {
	type pin struct {
		obj   zObject
		exits map[string]*Edge
	}
	var pins []pin
	for _, r := range ex.rooms {
		if len(cands[r.ID]) == 1 {
			pins = append(pins, pin{objs[cands[r.ID][0]], exits[r.ID]})
		}
	}
	if len(pins) == 0 {
		return nil, fmt.Errorf("no uniquely-named rooms to seed direction-property discovery")
	}
	dirProp := map[string]int{}
	seen := map[int]string{}
	for _, dir := range ex.directions {
		used := false
		for _, p := range pins {
			if p.exits[dir] != nil {
				used = true
				break
			}
		}
		if !used {
			continue // direction never used from a uniquely-named room; not needed as a constraint
		}
		var matches []int
		for prop := 1; prop <= 31; prop++ {
			ok := true
			for _, p := range pins {
				e := p.exits[dir]
				data, has := p.obj.props[prop]
				if (e == nil) != !has || (e != nil && len(data) != exitSize[e.Kind]) {
					ok = false
					break
				}
			}
			if ok {
				matches = append(matches, prop)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("direction %s: property number ambiguous or unsolvable (candidates %v)", dir, matches)
		}
		if prev, dup := seen[matches[0]]; dup {
			return nil, fmt.Errorf("directions %s and %s both solve to property %d", prev, dir, matches[0])
		}
		seen[matches[0]] = dir
		dirProp[dir] = matches[0]
	}
	return dirProp, nil
}

// compatible checks a candidate object against a room's exit signature,
// using target object numbers where the target room is already resolved and
// falling back to its candidate set where not.
func compatible(o zObject, roomExits map[string]*Edge, dirProp map[string]int, assigned map[string]int, cands map[string][]int) bool {
	for dir, prop := range dirProp {
		e := roomExits[dir]
		data, has := o.props[prop]
		if e == nil {
			if has {
				return false
			}
			continue
		}
		if !has || len(data) != exitSize[e.Kind] {
			return false
		}
		if exitTargetByte(e.Kind) {
			target := int(data[0])
			if num, done := assigned[e.To]; done {
				if target != num {
					return false
				}
			} else if !contains(cands[e.To], target) {
				return false
			}
		}
	}
	return true
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
