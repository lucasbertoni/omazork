package zil

import "testing"

func parseOne(t *testing.T, src string) *Node {
	t.Helper()
	nodes, err := Parse([]byte(src), "test.zil")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(nodes) != 1 {
		t.Fatalf("Parse(%q): got %d nodes, want 1", src, len(nodes))
	}
	return nodes[0]
}

func TestParseRoomForm(t *testing.T) {
	src := `<ROOM WEST-OF-HOUSE
      (IN ROOMS)
      (DESC "West of House")
      (NORTH TO NORTH-OF-HOUSE)
      (EAST "The door is boarded and you can't remove the boards.")
      (SW TO STONE-BARROW IF WON-FLAG)
      (FLAGS RLANDBIT ONBIT SACREDBIT)>`
	n := parseOne(t, src)
	if n.Kind != KForm || n.Head() != "ROOM" {
		t.Fatalf("got kind=%v head=%q", n.Kind, n.Head())
	}
	if got := n.Kids[1].Atom(); got != "WEST-OF-HOUSE" {
		t.Errorf("room id = %q", got)
	}
	if len(n.Kids) != 8 {
		t.Fatalf("got %d kids, want 8", len(n.Kids))
	}
	desc := n.Kids[3]
	if desc.Head() != "DESC" || desc.Kids[1].Kind != KString || desc.Kids[1].Text != "West of House" {
		t.Errorf("DESC list mis-parsed: %+v", desc)
	}
	if n.Start != 0 || n.End != len(src) {
		t.Errorf("span = [%d,%d), want [0,%d)", n.Start, n.End, len(src))
	}
}

func TestStringEscapes(t *testing.T) {
	n := parseOne(t, `"say \"hi\" and a back\\slash"`)
	if n.Kind != KString || n.Text != `say "hi" and a back\slash` {
		t.Errorf("got %q", n.Text)
	}
}

func TestAtomEscapes(t *testing.T) {
	n := parseOne(t, `(SYNONYM DAM GATE FCD\#3)`)
	if got := n.Kids[2].Atom(); got != "GATE" {
		t.Errorf("kid 2 = %q", got)
	}
	if got := n.Kids[3].Atom(); got != "FCD#3" {
		t.Errorf("escaped atom = %q", got)
	}
}

func TestComments(t *testing.T) {
	nodes, err := Parse([]byte(`<CONSTANT P-LEXWORDS 1> ;"Word offset" <SETG X ;<OLD-FORM 1> 2>`), "test.zil")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (comments skipped)", len(nodes))
	}
	setg := nodes[1]
	if len(setg.Kids) != 3 || setg.Kids[2].Atom() != "2" {
		t.Errorf("commented form not skipped: %+v", setg.Kids)
	}
}

func TestCharLiteralsAndBangs(t *testing.T) {
	// From zork1.zil: <SETG WBREAKS <STRING !\" !,WBREAKS>>
	n := parseOne(t, `<SETG WBREAKS <STRING !\" !,WBREAKS>>`)
	inner := n.Kids[2]
	if inner.Kids[1].Kind != KChar || inner.Kids[1].Text != `"` {
		t.Errorf("char literal = %+v", inner.Kids[1])
	}
	if inner.Kids[2].Atom() != ",WBREAKS" {
		t.Errorf("bang-prefixed atom = %+v", inner.Kids[2])
	}
}

func TestMacroAndQuote(t *testing.T) {
	n := parseOne(t, `<COND %<COND (<==? ,ZORK-NUMBER 3> '(<F> <RTRUE>)) (T '(<NULL-F> <RFALSE>))>>`)
	if n.Head() != "COND" || n.Kids[1].Kind != KMacro {
		t.Fatalf("macro not wrapped: %+v", n.Kids[1])
	}
	if n.Kids[1].Kids[0].Head() != "COND" {
		t.Errorf("macro child head = %q", n.Kids[1].Kids[0].Head())
	}
}

func TestHashLiteral(t *testing.T) {
	n := parseOne(t, `#DECL ((X) FIX)`)
	if n.Kind != KHash || len(n.Kids) != 2 || n.Kids[0].Atom() != "DECL" {
		t.Errorf("hash literal = %+v", n)
	}
}

func TestPageBreaksAndBareStrings(t *testing.T) {
	nodes, err := Parse([]byte("\"section header\"\n\f\n<SETG A 1>"), "test.zil")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].Kind != KString || nodes[1].Head() != "SETG" {
		t.Errorf("got %+v", nodes)
	}
}

func TestUnclosedFormErrors(t *testing.T) {
	if _, err := Parse([]byte("<ROOM FOO (IN ROOMS)"), "test.zil"); err == nil {
		t.Error("want error for unclosed form")
	}
	if _, err := Parse([]byte(`<SETG X ">`), "test.zil"); err == nil {
		t.Error("want error for unclosed string")
	}
}

func TestLineNumbers(t *testing.T) {
	src := "<A>\n\n<ROUTINE MAZE-DIODES ()\n <TELL \"x\">>"
	nodes, err := Parse([]byte(src), "test.zil")
	if err != nil {
		t.Fatal(err)
	}
	if nodes[1].Line != 3 {
		t.Errorf("routine line = %d, want 3", nodes[1].Line)
	}
	raw := src[nodes[1].Start:nodes[1].End]
	if raw != "<ROUTINE MAZE-DIODES ()\n <TELL \"x\">>" {
		t.Errorf("raw span = %q", raw)
	}
}
