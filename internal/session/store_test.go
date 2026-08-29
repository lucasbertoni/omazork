package session_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/session"
)

func TestPlaythroughRoundTrip(t *testing.T) {
	st := session.NewStore(t.TempDir())
	if _, ok, err := st.LoadPlaythrough("zork1"); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v, want absent", ok, err)
	}
	p := &session.Playthrough{
		Game: "zork1", Mode: session.Casual,
		CreatedAt: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
		Autosave:  []byte{1, 2, 3},
		Room:      "Kitchen", Score: 10, Moves: 4,
		Checkpoints: []session.Checkpoint{{ID: "cp1", Room: "Kitchen", Score: 10, State: []byte{9}}},
		Pending: &session.Pending{
			Command: "enter window", Output: "...", Delta: 10,
			MaturesAt: time.Date(2026, 8, 29, 12, 5, 0, 0, time.UTC),
		},
		Stats: session.Stats{Sessions: 1, Commands: 4},
	}
	if err := st.SavePlaythrough(p); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.LoadPlaythrough("zork1")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Mode != session.Casual || got.Score != 10 || string(got.Autosave) != string(p.Autosave) {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.Pending == nil || !got.Pending.MaturesAt.Equal(p.Pending.MaturesAt) {
		t.Errorf("pending lost: %+v", got.Pending)
	}
	if len(got.Checkpoints) != 1 || got.Checkpoints[0].ID != "cp1" {
		t.Errorf("checkpoints lost: %+v", got.Checkpoints)
	}
}

func TestReplaceDeletesPlaythroughButKeepsUnlocks(t *testing.T) {
	dir := t.TempDir()
	st := session.NewStore(dir)
	_ = st.SavePlaythrough(&session.Playthrough{Game: "zork1", Mode: session.Classic})
	when := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	if err := st.Unlock("zork1", "into-the-dark", when); err != nil {
		t.Fatal(err)
	}
	if err := st.DeletePlaythrough("zork1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.LoadPlaythrough("zork1"); ok {
		t.Error("playthrough survived delete")
	}
	unlocks, err := st.Unlocks("zork1")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := unlocks["into-the-dark"]; !ok || !got.Equal(when) {
		t.Errorf("unlock lost after playthrough delete: %+v", unlocks)
	}
	// Unlocks are idempotent and keep the first timestamp.
	_ = st.Unlock("zork1", "into-the-dark", when.Add(time.Hour))
	unlocks, _ = st.Unlocks("zork1")
	if !unlocks["into-the-dark"].Equal(when) {
		t.Error("re-unlock overwrote first timestamp")
	}
}

func TestStoreCreatesNestedDirs(t *testing.T) {
	st := session.NewStore(filepath.Join(t.TempDir(), "a", "b"))
	if err := st.SavePlaythrough(&session.Playthrough{Game: "zork3", Mode: session.Casual}); err != nil {
		t.Fatal(err)
	}
}
