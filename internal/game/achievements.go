package game

import (
	"time"

	"github.com/lucasbertoni/omazork/internal/engine"
	"github.com/lucasbertoni/omazork/internal/tables"
)

var engineTurnZero engine.Turn

// milestones are the wrapper-detected non-scoring trigger kinds of one turn (#13).
type milestones struct {
	death      bool
	grue       bool
	checkpoint bool
	won        bool
	finalDeath bool
}

// evalAchievements returns the achievements this turn would unlock, without
// unlocking them: Casual withholds the unlock until the reveal (#10).
func (s *Session) evalAchievements(turn engine.Turn, delta int, eventID string, died bool, m milestones, now time.Time) []tables.Achievement {
	unlocks, err := s.cfg.Store.Unlocks(s.p.Game)
	if err != nil {
		return nil
	}
	var out []tables.Achievement
	for _, a := range s.achs {
		if _, done := unlocks[a.ID]; done {
			continue
		}
		fire := false
		switch a.Trigger.Type {
		case "score-event":
			fire = eventID != "" && a.Trigger.EventID == eventID
		case "score-reaches":
			fire = turn.Score >= a.Trigger.Score && turn.Score > 0
		case "milestone":
			switch a.Trigger.Kind {
			case "first-death":
				fire = m.death
			case "grue-death":
				fire = m.grue
			case "first-checkpoint":
				fire = m.checkpoint
			case "game-won":
				fire = m.won
			case "final-death":
				fire = m.finalDeath
			}
		}
		if fire {
			out = append(out, a)
		}
	}
	return out
}

// unlock persists unlocks (idempotent, lifetime per game).
func (s *Session) unlock(achs []tables.Achievement, now time.Time) {
	for _, a := range achs {
		_ = s.cfg.Store.Unlock(s.p.Game, a.ID, now)
	}
}

// Achievements lists the game's full achievement table with unlock state.
type AchievementState struct {
	tables.Achievement
	Unlocked   bool       `json:"unlocked"`
	UnlockedAt *time.Time `json:"unlockedAt,omitempty"`
}

func (s *Session) Achievements() ([]AchievementState, error) {
	return AchievementsFor(s.cfg.Store, s.p.Game, s.achs)
}

// AchievementsFor builds the unlock-state view for any game (the Picker's
// browsable list works without a live session).
func AchievementsFor(store interface {
	Unlocks(string) (map[string]time.Time, error)
}, game string, achs []tables.Achievement) ([]AchievementState, error) {
	unlocks, err := store.Unlocks(game)
	if err != nil {
		return nil, err
	}
	out := make([]AchievementState, 0, len(achs))
	for _, a := range achs {
		st := AchievementState{Achievement: a}
		if at, ok := unlocks[a.ID]; ok {
			st.Unlocked = true
			t := at
			st.UnlockedAt = &t
		}
		out = append(out, st)
	}
	return out, nil
}
