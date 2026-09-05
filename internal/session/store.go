// Package session owns the durable state of play: the per-game Playthrough
// (autosave, checkpoints, pending outcome, play stats) and the per-game
// lifetime achievement unlocks. Everything lives as JSON files under one
// state root (~/.local/state/omazork in production) so it survives plugin
// rescans, shell restarts, and plugin removal/update (#6).
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Mode is a playthrough's fixed play mode (#5): Casual mediates outcomes,
// Classic bypasses the mediation layer entirely.
type Mode string

const (
	Classic Mode = "classic"
	Casual  Mode = "casual"
)

// Playthrough is the durable unit of play — at most one per game (#6).
type Playthrough struct {
	Game       string    `json:"game"`
	Mode       Mode      `json:"mode"`
	CreatedAt  time.Time `json:"createdAt"`
	LastPlayed time.Time `json:"lastPlayed"`

	// Autosave is the Quetzal snapshot taken after every turn; Resume is
	// always exactly where play stopped.
	Autosave []byte `json:"autosave"`
	Room     string `json:"room"`
	Score    int    `json:"score"`
	Moves    int    `json:"moves"`

	Checkpoints []Checkpoint `json:"checkpoints,omitempty"`
	Pending     *Pending     `json:"pending,omitempty"`
	Stats       Stats        `json:"stats"`

	// Transcript is the tail of recent console lines, kept so the Console can
	// re-render context on resume. Bounded by TrimTranscript.
	Transcript []string `json:"transcript,omitempty"`

	// Milestones records once-per-playthrough facts the achievement engine
	// needs (death counts, shadow-event ordering).
	Deaths           int  `json:"deaths"`
	LandOfShadowHits int  `json:"landOfShadowHits,omitempty"`
	Finished         bool `json:"finished,omitempty"`

	// Combat is the villain the action-wait classifier has the player engaged
	// with (docs/action-waits.md §3.3), "" out of combat. Persisted so a
	// wrapper respawn mid-fight keeps pricing combat turns instant; saves
	// from before the field simply resume out of combat.
	Combat string `json:"combat,omitempty"`
}

// Checkpoint is a player-created snapshot made with the in-game SAVE command.
type Checkpoint struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Room      string    `json:"room"`
	Score     int       `json:"score"`
	State     []byte    `json:"state"`
}

// Pending is a withheld outcome: the turn already ran (and was autosaved);
// only its display is held back until maturation (#5).
type Pending struct {
	Command   string    `json:"command"`
	Output    string    `json:"output"`
	Delta     int       `json:"delta"`
	EventID   string    `json:"eventId,omitempty"`
	Room      string    `json:"room"` // post-turn status, withheld with the output
	Score     int       `json:"score"`
	Moves     int       `json:"moves"`
	MaturesAt time.Time `json:"maturesAt"`
	Notified  bool      `json:"notified"`
	// Achievements that would have unlocked on the withheld turn; they unlock
	// at the reveal, folded into the recap (#10).
	Achievements []string `json:"achievements,omitempty"`
}

// Stats are the passive per-playthrough counters (#10). Reset with the
// playthrough; never gamified.
type Stats struct {
	Sessions        int   `json:"sessions"`
	Deaths          int   `json:"deaths"`
	PlaytimeSeconds int64 `json:"playtimeSeconds"`
	Commands        int   `json:"commands"`
}

// Store reads and writes playthroughs and unlocks under one root directory.
type Store struct{ root string }

func NewStore(root string) *Store { return &Store{root: root} }

func (s *Store) playthroughPath(game string) string {
	return filepath.Join(s.root, game, "playthrough.json")
}
func (s *Store) unlocksPath(game string) string {
	return filepath.Join(s.root, game, "achievements.json")
}

// LoadPlaythrough returns the game's playthrough, or ok=false when none exists.
func (s *Store) LoadPlaythrough(game string) (*Playthrough, bool, error) {
	raw, err := os.ReadFile(s.playthroughPath(game))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("session: load %s: %w", game, err)
	}
	var p Playthrough
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, false, fmt.Errorf("session: decode %s playthrough: %w", game, err)
	}
	return &p, true, nil
}

// SavePlaythrough writes the playthrough atomically (temp file + rename), so
// a SIGTERM mid-write can never corrupt the autosave.
func (s *Store) SavePlaythrough(p *Playthrough) error {
	return s.writeJSON(s.playthroughPath(p.Game), p)
}

// DeletePlaythrough removes a game's playthrough (new-game replacement, #6).
// Lifetime achievement unlocks are untouched.
func (s *Store) DeletePlaythrough(game string) error {
	err := os.Remove(s.playthroughPath(game))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Unlocks returns the game's lifetime achievement unlocks: id -> first unlock time.
func (s *Store) Unlocks(game string) (map[string]time.Time, error) {
	raw, err := os.ReadFile(s.unlocksPath(game))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: load %s unlocks: %w", game, err)
	}
	m := map[string]time.Time{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("session: decode %s unlocks: %w", game, err)
	}
	return m, nil
}

// Unlock records an achievement unlock. Idempotent: the first timestamp wins.
func (s *Store) Unlock(game, id string, at time.Time) error {
	m, err := s.Unlocks(game)
	if err != nil {
		return err
	}
	if _, done := m[id]; done {
		return nil
	}
	m[id] = at
	return s.writeJSON(s.unlocksPath(game), m)
}

func (s *Store) writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	return nil
}

// TrimTranscript bounds the stored transcript tail.
const maxTranscript = 400

func (p *Playthrough) AppendTranscript(lines ...string) {
	p.Transcript = append(p.Transcript, lines...)
	if n := len(p.Transcript); n > maxTranscript {
		p.Transcript = append([]string(nil), p.Transcript[n-maxTranscript:]...)
	}
}
