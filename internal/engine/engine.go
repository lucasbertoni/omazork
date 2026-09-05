// Package engine wraps the embedded Z-machine and the read-only Quetzal peek
// adapter. It is the only package that talks to the zmachine/quetzal modules;
// everything above deals in Turns.
package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/maloquacious/quetzal"
	"github.com/maloquacious/zmachine"

	omazork "github.com/lucasbertoni/omazork"
)

// Turn is what one player command (or the opening banner) produced.
type Turn struct {
	Output  string // story text, whitespace preserved; never the status line
	Room    string // status-line room name; true room even in darkness
	RoomObj uint16 // global 0: current room object number; 0 when Halted (no snapshot)
	Score   int
	Moves   int
	State   []byte // Quetzal snapshot at the input boundary; nil when Halted
	Halted  bool   // the story terminated itself (quit, victory, Zork III final death)
}

// Engine is one running story. Not safe for concurrent use.
type Engine struct {
	m      *zmachine.Machine
	qstory quetzal.Story
	room   string // last known status-line room name
}

type Option func(*config)

type config struct{ seed *uint64 }

// WithSeed makes the story's randomness deterministic (tests).
func WithSeed(seed uint64) Option { return func(c *config) { c.seed = &seed } }

// New builds an engine for one of the bundled games: zork1, zork2, zork3.
func New(game string, opts ...Option) (*Engine, error) {
	storyBytes, err := omazork.Games.ReadFile("assets/games/" + game + ".z3")
	if err != nil {
		return nil, fmt.Errorf("engine: unknown game %q: %w", game, err)
	}
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	story, err := zmachine.LoadStory(storyBytes)
	if err != nil {
		return nil, fmt.Errorf("engine: load story: %w", err)
	}
	qstory, err := quetzal.ParseStory(storyBytes)
	if err != nil {
		return nil, fmt.Errorf("engine: parse story for quetzal: %w", err)
	}
	var mopts []zmachine.Option
	if cfg.seed != nil {
		mopts = append(mopts, zmachine.WithRandomSeed(*cfg.seed))
	}
	m, err := zmachine.New(story, mopts...)
	if err != nil {
		return nil, fmt.Errorf("engine: new machine: %w", err)
	}
	return &Engine{m: m, qstory: qstory}, nil
}

// Start runs the story to its first prompt (the opening banner).
func (e *Engine) Start() (Turn, error) {
	res, err := e.m.Start(runCtx())
	if err != nil {
		return Turn{}, fmt.Errorf("engine: start: %w", err)
	}
	return e.turn(res)
}

// Run supplies one line of player input and returns its outcome.
func (e *Engine) Run(input string) (Turn, error) {
	res, err := e.m.Run(runCtx(), input)
	if err != nil {
		return Turn{}, fmt.Errorf("engine: run: %w", err)
	}
	return e.turn(res)
}

// Restore rewinds the engine to a snapshot previously returned in Turn.State.
func (e *Engine) Restore(state []byte) error {
	if err := e.m.Restore(state); err != nil {
		return fmt.Errorf("engine: restore: %w", err)
	}
	// The status line (and so the display room name) only refreshes at the
	// next input boundary; until then the peek's object number is all there is.
	e.room = ""
	return nil
}

// Peek reads the room object number, score, and moves out of a snapshot
// without touching the running machine — what a resumed session needs to
// re-seed turn classification from its autosave.
func (e *Engine) Peek(state []byte) (Turn, error) {
	p, err := e.peek(state)
	if err != nil {
		return Turn{}, fmt.Errorf("engine: peek: %w", err)
	}
	return Turn{RoomObj: p.Room, Score: int(p.Score), Moves: int(p.Moves)}, nil
}

func runCtx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_ = cancel // the deadline bounds a runaway story; the engine returns first in practice
	return ctx
}

type peekView struct {
	Room  uint16 // global 0: current room object number
	Score int16  // global 1
	Moves uint16 // global 2
}

// peek Quetzal-decodes a snapshot and reads the v3 globals out of the
// reconstructed dynamic memory. Header word 0x0C is the global table address;
// global g is the big-endian word at table + 2g (Standard §6.2, §11).
func (e *Engine) peek(state []byte) (peekView, error) {
	f, err := quetzal.Decode(bytes.NewReader(state))
	if err != nil {
		return peekView{}, fmt.Errorf("decode quetzal: %w", err)
	}
	mem, err := f.Memory(e.qstory)
	if err != nil {
		return peekView{}, fmt.Errorf("reconstruct memory: %w", err)
	}
	globals := binary.BigEndian.Uint16(mem.Data[0x0C:])
	g := func(n int) uint16 {
		return binary.BigEndian.Uint16(mem.Data[int(globals)+2*n:])
	}
	return peekView{Room: g(0), Score: int16(g(1)), Moves: g(2)}, nil
}

func (e *Engine) turn(res zmachine.Result) (Turn, error) {
	t := Turn{Output: res.Output, Halted: res.Status == zmachine.Halted, State: res.State}
	if res.StatusLine.Available {
		e.room = res.StatusLine.Name
	}
	t.Room = e.room
	switch {
	case len(res.State) > 0:
		// The Quetzal peek is canonical for score/moves: it reads the globals
		// directly and doubles as a decode check on the autosave payload.
		p, err := e.peek(res.State)
		if err != nil {
			return Turn{}, fmt.Errorf("engine: peek: %w", err)
		}
		t.Score, t.Moves, t.RoomObj = int(p.Score), int(p.Moves), p.Room
	case res.StatusLine.Available:
		// Halted: no snapshot exists; the last status line is all there is.
		t.Score, t.Moves = int(res.StatusLine.Score), int(res.StatusLine.Turns)
	}
	return t, nil
}
