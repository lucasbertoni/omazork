// Package game is the wrapper's core: it drives one playthrough through the
// engine, applies the casual-mode mediation rules (#5, #7), owns the save and
// session model (#6), and evaluates achievements and play stats (#10, #13).
package game

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lucasbertoni/omazork/internal/engine"
	"github.com/lucasbertoni/omazork/internal/session"
	"github.com/lucasbertoni/omazork/internal/tables"
)

// Config wires a Session to its environment. Now is injectable for tests.
type Config struct {
	Store *session.Store
	Now   func() time.Time
	Seed  uint64 // engine RNG seed; 0 = nondeterministic
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Response kinds.
const (
	KindOutput      = "output"      // a turn's outcome, shown now
	KindWithheld    = "withheld"    // outcome held back; Pending says until when
	KindBlocked     = "blocked"     // input refused while an outcome is pending
	KindCheckpoints = "checkpoints" // list of checkpoints (intercepted RESTORE)
	KindCheckpoint  = "checkpoint"  // checkpoint created (intercepted SAVE)
	KindEnded       = "ended"       // the story halted; Ended says how
)

// Status is the visible status line state.
type Status struct {
	Room  string `json:"room"`
	Score int    `json:"score"`
	Moves int    `json:"moves"`
}

// Reveal is the "While you were away…" recap for a matured outcome (#5).
type Reveal struct {
	Command  string               `json:"command"`
	Output   string               `json:"output"`
	Delta    int                  `json:"delta"`
	Status   Status               `json:"status"`
	Unlocked []tables.Achievement `json:"unlocked,omitempty"`
}

// PendingInfo describes a not-yet-matured outcome without spoiling it.
type PendingInfo struct {
	MaturesAt time.Time     `json:"maturesAt"`
	Remaining time.Duration `json:"remaining"`
}

// CheckpointInfo is a checkpoint as shown to the player (no raw state).
type CheckpointInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Room      string    `json:"room"`
	Score     int       `json:"score"`
}

// Response is the outcome of one player interaction.
type Response struct {
	Kind        string               `json:"kind"`
	Output      string               `json:"output,omitempty"`
	Status      *Status              `json:"status,omitempty"`
	Reveal      *Reveal              `json:"reveal,omitempty"`
	Pending     *PendingInfo         `json:"pending,omitempty"`
	Checkpoints []CheckpointInfo     `json:"checkpoints,omitempty"`
	Unlocked    []tables.Achievement `json:"unlocked,omitempty"`
	Ended       string               `json:"ended,omitempty"` // "won", "quit", "died"
}

// ErrPlaythroughExists is returned by New when the game already has a
// playthrough; starting over requires the explicit Replace (#6).
var ErrPlaythroughExists = errors.New("playthrough exists; replacing requires confirmation")

// Session is one live playthrough. Not safe for concurrent use.
type Session struct {
	cfg      Config
	eng      *engine.Engine
	p        *session.Playthrough
	waits    *tables.Game
	achs     []tables.Achievement
	openedAt time.Time // zero when the console is closed

	// quit tracking: the QUIT command asks for confirmation before halting;
	// a halt during that exchange is a quit, not a victory.
	quitAsked bool
}

// New starts a fresh playthrough. It fails with ErrPlaythroughExists when one
// already exists — call Replace to overwrite after user confirmation.
func New(cfg Config, gameName string, mode session.Mode) (*Session, Response, error) {
	if _, ok, err := cfg.Store.LoadPlaythrough(gameName); err != nil {
		return nil, Response{}, err
	} else if ok {
		return nil, Response{}, ErrPlaythroughExists
	}
	return start(cfg, gameName, mode)
}

// Replace discards any existing playthrough (lifetime achievements survive)
// and starts fresh.
func Replace(cfg Config, gameName string, mode session.Mode) (*Session, Response, error) {
	if err := cfg.Store.DeletePlaythrough(gameName); err != nil {
		return nil, Response{}, err
	}
	return start(cfg, gameName, mode)
}

func start(cfg Config, gameName string, mode session.Mode) (*Session, Response, error) {
	s, err := newSession(cfg, gameName)
	if err != nil {
		return nil, Response{}, err
	}
	now := cfg.now()
	s.p = &session.Playthrough{Game: gameName, Mode: mode, CreatedAt: now, LastPlayed: now}
	turn, err := s.eng.Start()
	if err != nil {
		return nil, Response{}, err
	}
	s.recordTurn(turn)
	s.p.Stats.Sessions++
	s.p.AppendTranscript(turn.Output)
	if err := s.save(); err != nil {
		return nil, Response{}, err
	}
	return s, Response{Kind: KindOutput, Output: turn.Output, Status: s.status()}, nil
}

// Resume reopens the game's existing playthrough from its autosave.
func Resume(cfg Config, gameName string) (*Session, Response, error) {
	p, ok, err := cfg.Store.LoadPlaythrough(gameName)
	if err != nil {
		return nil, Response{}, err
	}
	if !ok {
		return nil, Response{}, fmt.Errorf("game: no playthrough for %s", gameName)
	}
	s, err := newSession(cfg, gameName)
	if err != nil {
		return nil, Response{}, err
	}
	s.p = p
	if len(p.Autosave) > 0 && !p.Finished {
		if err := s.eng.Restore(p.Autosave); err != nil {
			return nil, Response{}, err
		}
	}
	s.p.Stats.Sessions++
	s.p.LastPlayed = cfg.now()
	if err := s.save(); err != nil {
		return nil, Response{}, err
	}
	resp := Response{Kind: KindOutput, Status: s.status()}
	s.decorate(&resp)
	return s, resp, nil
}

func newSession(cfg Config, gameName string) (*Session, error) {
	var opts []engine.Option
	if cfg.Seed != 0 {
		opts = append(opts, engine.WithSeed(cfg.Seed))
	}
	eng, err := engine.New(gameName, opts...)
	if err != nil {
		return nil, err
	}
	waits, err := tables.Load(gameName)
	if err != nil {
		return nil, err
	}
	achs, err := tables.LoadAchievements(gameName)
	if err != nil {
		return nil, err
	}
	return &Session{cfg: cfg, eng: eng, waits: waits, achs: achs}, nil
}

// Mode reports the playthrough's fixed mode.
func (s *Session) Mode() session.Mode { return s.p.Mode }

// Game reports which game this session plays.
func (s *Session) Game() string { return s.p.Game }

// Command handles one line of player input: meta interception first, then the
// mediation rules, then the engine.
func (s *Session) Command(text string) (Response, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Response{Kind: KindOutput, Status: s.status()}, nil
	}
	if s.p.Finished {
		return Response{Kind: KindEnded, Ended: "over", Status: s.status()}, nil
	}
	now := s.cfg.now()

	// Intercepted meta commands: SAVE/RESTORE become checkpoints (#6).
	switch strings.ToLower(text) {
	case "save":
		return s.saveCheckpoint(now)
	case "restore":
		return s.listCheckpoints(now)
	}

	var reveal *Reveal
	if s.p.Pending != nil {
		if now.Before(s.p.Pending.MaturesAt) {
			// Input is blocked while an outcome is pending (#5).
			resp := Response{
				Kind:   KindBlocked,
				Output: "The outcome of your last action is still unfolding...",
				Status: s.status(),
			}
			s.decorate(&resp)
			return resp, nil
		}
		reveal = s.reveal(now)
	}

	resp, err := s.runTurn(text, now)
	if err != nil {
		return Response{}, err
	}
	resp.Reveal = reveal
	if err := s.save(); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// runTurn executes one game turn and applies the mediation rules.
func (s *Session) runTurn(text string, now time.Time) (Response, error) {
	before := *s.p // copy of pre-turn status for withheld responses
	turn, err := s.eng.Run(text)
	if err != nil {
		return Response{}, err
	}
	s.p.Stats.Commands++
	s.p.LastPlayed = now

	lower := strings.ToLower(text)
	if lower == "quit" || lower == "q" {
		s.quitAsked = true
	} else if !turn.Halted {
		s.quitAsked = false
	}

	died := detectDeath(turn.Output)
	if died {
		s.p.Deaths++
		s.p.Stats.Deaths++
	}

	if turn.Halted {
		return s.endTurn(text, turn, died, now)
	}

	delta := turn.Score - before.Score
	eventID := s.matchEventID(delta, turn.Room)
	unlocked := s.evalAchievements(turn, delta, eventID, died, milestones{
		death: died, grue: died && strings.Contains(strings.ToLower(turn.Output), "grue"),
	}, now)

	// Mediation: delay iff score delta != 0, Casual mode only (#5). Deaths and
	// halts always reveal immediately.
	if s.p.Mode == session.Casual && delta != 0 && !died {
		wait := s.waitFor(delta, turn.Room, eventID)
		if wait > 0 {
			// The turn ran and is autosaved, but the visible status must not
			// advance: the status line is part of the outcome (#7) and stays
			// at its pre-turn values on every surface until the reveal.
			s.p.Autosave = turn.State
			s.p.Pending = &session.Pending{
				Command: text, Output: turn.Output, Delta: delta, EventID: eventID,
				Room: turn.Room, Score: turn.Score, Moves: turn.Moves,
				MaturesAt: now.Add(wait),
			}
			for _, a := range unlocked {
				s.p.Pending.Achievements = append(s.p.Pending.Achievements, a.ID)
			}
			s.p.AppendTranscript("> "+text, "[Something is unfolding...]")
			return Response{
				Kind:    KindWithheld,
				Output:  "The outcome of your action will take some time to unfold...",
				Status:  s.status(),
				Pending: s.pendingInfo(now),
			}, nil
		}
	}

	s.recordTurn(turn)
	// Pass through: unlock immediately.
	s.unlock(unlocked, now)
	s.p.AppendTranscript("> "+text, turn.Output)
	return Response{Kind: KindOutput, Output: turn.Output, Status: s.status(), Unlocked: unlocked}, nil
}

// endTurn handles a halted story: quit, victory, or Zork III's final death.
func (s *Session) endTurn(text string, turn engine.Turn, died bool, now time.Time) (Response, error) {
	ended := "quit"
	var kinds milestones
	switch {
	case died || (s.p.Game == "zork3" && strings.Contains(strings.ToLower(turn.Output), "good night")):
		ended = "died"
		kinds = milestones{death: died, finalDeath: s.p.Game == "zork3"}
	case s.quitAsked:
		ended = "quit"
	default:
		ended = "won"
		kinds = milestones{won: true}
	}
	unlocked := s.evalAchievements(turn, 0, "", died, kinds, now)
	s.unlock(unlocked, now)
	s.recordTurn(turn)
	if ended != "quit" {
		s.p.Finished = true
	}
	// A halted machine has no snapshot; keep the last autosave so a quit can
	// resume from just before it.
	s.p.AppendTranscript("> "+text, turn.Output)
	return Response{Kind: KindEnded, Ended: ended, Output: turn.Output, Status: s.status(), Unlocked: unlocked}, nil
}

// waitFor resolves a scoring turn's wait duration (#5, #9).
func (s *Session) waitFor(delta int, room, eventID string) time.Duration {
	if eventID != "" {
		if ev, ok := s.waits.Match(delta, room); ok {
			return ev.Wait
		}
	}
	return s.waits.FallbackWait()
}

// matchEventID resolves the turn's score event, applying the Zork III
// Land-of-Shadow ordering rule (#13): first +1 there is the appearance,
// second is the strike.
func (s *Session) matchEventID(delta int, room string) string {
	if delta == 0 {
		return ""
	}
	if s.p.Game == "zork3" && delta == 1 && room == "Land of Shadow" {
		s.p.LandOfShadowHits++
		if s.p.LandOfShadowHits == 1 {
			return "shadow-appears"
		}
		return "shadow-struck"
	}
	if ev, ok := s.waits.Match(delta, room); ok {
		return ev.ID
	}
	return ""
}

// reveal clears the pending outcome, unlocks its achievements, and builds the
// recap (#5).
func (s *Session) reveal(now time.Time) *Reveal {
	pend := s.p.Pending
	s.p.Pending = nil
	s.p.Room, s.p.Score, s.p.Moves = pend.Room, pend.Score, pend.Moves
	r := &Reveal{
		Command: pend.Command, Output: pend.Output, Delta: pend.Delta,
		Status: Status{Room: pend.Room, Score: pend.Score, Moves: pend.Moves},
	}
	for _, id := range pend.Achievements {
		for _, a := range s.achs {
			if a.ID == id {
				r.Unlocked = append(r.Unlocked, a)
			}
		}
	}
	s.unlock(r.Unlocked, now)
	s.p.AppendTranscript(fmt.Sprintf("[While you were away... (%+d points)]", pend.Delta), pend.Output)
	return r
}

// Opened marks the console open: reveals a matured outcome (#5: "on the next
// console open after maturing") and starts the playtime clock.
func (s *Session) Opened() (Response, error) {
	now := s.cfg.now()
	s.openedAt = now
	resp := Response{Kind: KindOutput, Status: s.status()}
	if s.p.Pending != nil && !now.Before(s.p.Pending.MaturesAt) {
		resp.Reveal = s.reveal(now)
		resp.Status = s.status()
	}
	s.decorate(&resp)
	if err := s.save(); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Closed marks the console closed and accumulates active playtime.
func (s *Session) Closed() error {
	if s.openedAt.IsZero() {
		return nil
	}
	s.p.Stats.PlaytimeSeconds += int64(s.cfg.now().Sub(s.openedAt) / time.Second)
	s.openedAt = time.Time{}
	return s.save()
}

// Transcript returns the stored console tail for re-rendering on resume.
func (s *Session) Transcript() []string { return s.p.Transcript }

// Stats returns the playthrough's play stats.
func (s *Session) Stats() session.Stats { return s.p.Stats }

// PendingMatured reports whether a pending outcome has matured but not yet
// been revealed; the caller fires the one non-spoiler notification (#5).
func (s *Session) PendingMatured() bool {
	p := s.p.Pending
	return p != nil && !s.cfg.now().Before(p.MaturesAt) && !p.Notified
}

// MarkNotified records that the maturation notification has been sent.
func (s *Session) MarkNotified() error {
	if s.p.Pending != nil {
		s.p.Pending.Notified = true
	}
	return s.save()
}

func (s *Session) status() *Status {
	return &Status{Room: s.p.Room, Score: s.p.Score, Moves: s.p.Moves}
}

// decorate attaches pending info (non-spoiler) to a response; while an
// outcome is pending the visible status stays at its pre-turn values because
// recordTurn was never run past... the playthrough status is only advanced on
// reveal for withheld turns.
func (s *Session) decorate(resp *Response) {
	if s.p.Pending != nil {
		resp.Pending = s.pendingInfo(s.cfg.now())
	}
}

func (s *Session) pendingInfo(now time.Time) *PendingInfo {
	p := s.p.Pending
	if p == nil {
		return nil
	}
	rem := p.MaturesAt.Sub(now)
	if rem < 0 {
		rem = 0
	}
	return &PendingInfo{MaturesAt: p.MaturesAt, Remaining: rem}
}

func (s *Session) recordTurn(turn engine.Turn) {
	if len(turn.State) > 0 {
		s.p.Autosave = turn.State
	}
	s.p.Room, s.p.Score, s.p.Moves = turn.Room, turn.Score, turn.Moves
}

func (s *Session) save() error { return s.cfg.Store.SavePlaythrough(s.p) }

// detectDeath recognises a death turn by the canonical JIGS-UP text, which is
// score-independent (Zork III deaths carry no score penalty, #11).
func detectDeath(output string) bool {
	return strings.Contains(strings.ToLower(output), "you have died")
}

// Store exposes the session's backing store (server wiring, tests).
func (s *Session) Store() *session.Store { return s.cfg.Store }
