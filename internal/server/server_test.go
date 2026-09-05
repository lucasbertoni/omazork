package server_test

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/server"
	"github.com/lucasbertoni/omazork/internal/session"
)

type harness struct {
	t    *testing.T
	in   io.WriteCloser
	out  *bufio.Scanner
	done chan struct{}
}

func start(t *testing.T, clock *time.Time) *harness {
	t.Helper()
	cfg := game.Config{
		Store: session.NewStore(t.TempDir()),
		Now:   func() time.Time { return *clock },
		Seed:  1,
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := server.New(cfg)
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(inR, outW); outW.Close() }()
	return &harness{t: t, in: inW, out: bufio.NewScanner(outR), done: done}
}

func (h *harness) send(msg map[string]any) map[string]any {
	h.t.Helper()
	raw, _ := json.Marshal(msg)
	if _, err := h.in.Write(append(raw, '\n')); err != nil {
		h.t.Fatal(err)
	}
	if !h.out.Scan() {
		h.t.Fatal("no response line")
	}
	var resp map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &resp); err != nil {
		h.t.Fatalf("bad response %q: %v", h.out.Text(), err)
	}
	return resp
}

// play sends commands, advancing the clock past any withheld outcome so the
// next command reveals and runs.
func (h *harness) play(clock *time.Time, cmds ...string) {
	h.t.Helper()
	for _, cmd := range cmds {
		r := h.send(map[string]any{"type": "input", "text": cmd})
		if r["type"] == "withheld" {
			rem := r["pending"].(map[string]any)["remaining"].(float64)
			*clock = clock.Add(time.Duration(rem))
		}
	}
}

func TestProtocolFlow(t *testing.T) {
	now := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	h := start(t, &now)

	// Picker: three games, no playthroughs yet.
	pick := h.send(map[string]any{"type": "picker", "id": "1"})
	if pick["type"] != "picker" || pick["id"] != "1" {
		t.Fatalf("picker resp: %v", pick)
	}
	games := pick["games"].([]any)
	if len(games) != 3 {
		t.Fatalf("games = %v", games)
	}
	if g0 := games[0].(map[string]any); g0["playthrough"] != nil {
		t.Errorf("unexpected playthrough: %v", g0)
	}

	// New casual game.
	resp := h.send(map[string]any{"type": "new", "game": "zork1", "mode": "casual"})
	if resp["type"] != "output" || !strings.Contains(resp["output"].(string), "West of House") {
		t.Fatalf("new: %v", resp)
	}

	// Play to the window: a room-changing action is withheld for its wait.
	h.play(&now, "south", "east", "open window")
	wh := h.send(map[string]any{"type": "input", "text": "enter window"})
	if wh["type"] != "withheld" {
		t.Fatalf("enter window: %v", wh)
	}

	// Blocked while pending.
	if b := h.send(map[string]any{"type": "input", "text": "west"}); b["type"] != "blocked" {
		t.Fatalf("blocked: %v", b)
	}

	// Mature, reopen: reveal recap rides on the response.
	now = now.Add(time.Duration(wh["pending"].(map[string]any)["remaining"].(float64)))
	op := h.send(map[string]any{"type": "opened"})
	if op["reveal"] == nil {
		t.Fatalf("no reveal on open: %v", op)
	}

	// Picker now shows the playthrough.
	pick = h.send(map[string]any{"type": "picker"})
	for _, g := range pick["games"].([]any) {
		gm := g.(map[string]any)
		if gm["game"] == "zork1" {
			pt := gm["playthrough"].(map[string]any)
			if pt["mode"] != "casual" || pt["room"] != "Kitchen" {
				t.Errorf("picker playthrough: %v", pt)
			}
		}
	}

	// Achievements list is queryable.
	ach := h.send(map[string]any{"type": "achievements", "game": "zork1"})
	if ach["type"] != "achievements" || len(ach["achievements"].([]any)) < 10 {
		t.Fatalf("achievements: %v", ach)
	}

	// Unknown message type answers with an error, not a dropped connection.
	if e := h.send(map[string]any{"type": "bogus"}); e["type"] != "error" {
		t.Fatalf("bogus: %v", e)
	}

	// New over an existing playthrough requires confirmation.
	if c := h.send(map[string]any{"type": "new", "game": "zork1", "mode": "classic"}); c["type"] != "confirm-replace" {
		t.Fatalf("overwrite: %v", c)
	}
	if r := h.send(map[string]any{"type": "new", "game": "zork1", "mode": "classic", "replace": true}); r["type"] != "output" {
		t.Fatalf("replace: %v", r)
	}

	h.in.Close()
	<-h.done
}

func TestMaturationEvent(t *testing.T) {
	now := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	h := start(t, &now)
	h.send(map[string]any{"type": "new", "game": "zork1", "mode": "casual"})
	h.play(&now, "south", "east", "open window")
	h.send(map[string]any{"type": "input", "text": "enter window"})
	now = now.Add(10 * time.Minute)
	// The check-maturation tick emits a matured event exactly once.
	ev := h.send(map[string]any{"type": "check-maturation"})
	if ev["type"] != "matured" || ev["game"] != "zork1" {
		t.Fatalf("matured event: %v", ev)
	}
	if again := h.send(map[string]any{"type": "check-maturation"}); again["type"] != "quiet" {
		t.Fatalf("second check: %v", again)
	}
	h.in.Close()
	<-h.done
}

// The bar icon derives "recap waiting" from the picker after a shell restart,
// so the picker must say when a pending outcome matures, not just that one
// exists (qml/Service.qml, docs/adr/0002).
func TestPickerCarriesPendingMaturesAt(t *testing.T) {
	now := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	h := start(t, &now)

	h.send(map[string]any{"type": "new", "game": "zork1", "mode": "casual"})
	h.play(&now, "south", "east", "open window")
	if wh := h.send(map[string]any{"type": "input", "text": "enter window"}); wh["type"] != "withheld" {
		t.Fatalf("enter window: %v", wh)
	}

	pick := h.send(map[string]any{"type": "picker"})
	for _, g := range pick["games"].([]any) {
		gm := g.(map[string]any)
		if gm["game"] != "zork1" {
			continue
		}
		pt := gm["playthrough"].(map[string]any)
		if pt["pending"] != true {
			t.Fatalf("pending flag: %v", pt)
		}
		raw, ok := pt["pendingMaturesAt"].(string)
		if !ok {
			t.Fatalf("no pendingMaturesAt: %v", pt)
		}
		when, err := time.Parse(time.RFC3339, raw)
		if err != nil || !when.After(now) {
			t.Fatalf("pendingMaturesAt = %q (err %v), want a future RFC3339 time", raw, err)
		}
	}
	h.in.Close()
	<-h.done
}

// The 5-minute tier threshold lives only in the wrapper: the protocol carries
// the resolved tier on pending, on the matured event (so the QML side can skip
// the notification for quiet waits), and on the picker (so the bar's in-flight
// dot survives a shell restart).
func TestProtocolCarriesTier(t *testing.T) {
	now := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	h := start(t, &now)
	h.send(map[string]any{"type": "new", "game": "zork1", "mode": "casual"})
	h.play(&now, "south", "east", "open window")

	wh := h.send(map[string]any{"type": "input", "text": "enter window"}) // 2 min: quiet
	pend, _ := wh["pending"].(map[string]any)
	if pend == nil || pend["tier"] != "quiet" || pend["command"] != "enter window" {
		t.Fatalf("withheld pending = %v", wh["pending"])
	}
	pick := h.send(map[string]any{"type": "picker"})
	for _, g := range pick["games"].([]any) {
		gm := g.(map[string]any)
		if gm["game"] != "zork1" {
			continue
		}
		if pt := gm["playthrough"].(map[string]any); pt["pendingTier"] != "quiet" {
			t.Errorf("picker pendingTier = %v", pt["pendingTier"])
		}
	}
	now = now.Add(2 * time.Minute)
	if ev := h.send(map[string]any{"type": "check-maturation"}); ev["type"] != "matured" || ev["tier"] != "quiet" {
		t.Fatalf("matured event = %v", ev)
	}
	opened := h.send(map[string]any{"type": "opened"})
	if rv, _ := opened["reveal"].(map[string]any); rv == nil || rv["tier"] != "quiet" {
		t.Fatalf("reveal = %v", opened["reveal"])
	}

	// A full-tier wait says so everywhere too.
	if wh := h.send(map[string]any{"type": "input", "text": "up"}); wh["pending"].(map[string]any)["tier"] != "full" {
		t.Fatalf("up pending = %v", wh["pending"])
	}
	now = now.Add(5 * time.Minute)
	if ev := h.send(map[string]any{"type": "check-maturation"}); ev["tier"] != "full" {
		t.Fatalf("matured event = %v", ev)
	}
	h.in.Close()
	<-h.done
}
