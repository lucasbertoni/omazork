package game

import (
	"fmt"
	"time"

	"github.com/lucasbertoni/omazork/internal/session"
)

// saveCheckpoint intercepts the in-game SAVE command (#6): the current
// autosave becomes a named checkpoint inside the playthrough. Blocked while an
// outcome is pending.
func (s *Session) saveCheckpoint(now time.Time) (Response, error) {
	if s.blockedResponse() != nil {
		return *s.blockedResponse(), nil
	}
	cp := sessionCheckpoint(s, now)
	s.p.Checkpoints = append(s.p.Checkpoints, cp)
	unlocked := s.evalAchievements(engineTurnZero, 0, "", false, milestones{checkpoint: true}, now)
	s.unlock(unlocked, now)
	s.p.AppendTranscript("> save", "Saved. (checkpoint "+cp.ID+")")
	if err := s.save(); err != nil {
		return Response{}, err
	}
	return Response{
		Kind:     KindCheckpoint,
		Output:   fmt.Sprintf("Saved. You can RESTORE to this moment later. (%s, score %d)", cp.Room, cp.Score),
		Status:   s.status(),
		Unlocked: unlocked,
	}, nil
}

// listCheckpoints intercepts the in-game RESTORE command (#6): it answers
// with the checkpoint list; the actual restore happens via RestoreCheckpoint.
func (s *Session) listCheckpoints(now time.Time) (Response, error) {
	if s.blockedResponse() != nil {
		return *s.blockedResponse(), nil
	}
	resp := Response{Kind: KindCheckpoints, Status: s.status()}
	for _, cp := range s.p.Checkpoints {
		resp.Checkpoints = append(resp.Checkpoints, CheckpointInfo{
			ID: cp.ID, CreatedAt: cp.CreatedAt, Room: cp.Room, Score: cp.Score,
		})
	}
	if len(resp.Checkpoints) == 0 {
		resp.Output = "There are no saved checkpoints yet. Use SAVE to create one."
	}
	return resp, nil
}

// RestoreCheckpoint rewinds the playthrough to a checkpoint.
func (s *Session) RestoreCheckpoint(id string) (Response, error) {
	if s.blockedResponse() != nil {
		return *s.blockedResponse(), nil
	}
	for _, cp := range s.p.Checkpoints {
		if cp.ID != id {
			continue
		}
		if err := s.eng.Restore(cp.State); err != nil {
			return Response{}, err
		}
		s.p.Autosave = cp.State
		s.p.Room, s.p.Score, s.p.Moves = cp.Room, cp.Score, 0
		s.p.Finished = false
		// The classifier follows the engine back in time; a checkpoint is
		// never mid-fight from its own point of view.
		if err := s.reseed(cp.State, ""); err != nil {
			return Response{}, err
		}
		s.p.AppendTranscript(fmt.Sprintf("[Restored checkpoint from %s (score %d).]", cp.Room, cp.Score))
		if err := s.save(); err != nil {
			return Response{}, err
		}
		return Response{
			Kind:   KindOutput,
			Output: fmt.Sprintf("Restored. You are back in %s with a score of %d.", cp.Room, cp.Score),
			Status: s.status(),
		}, nil
	}
	return Response{}, fmt.Errorf("game: no checkpoint %q", id)
}

// blockedResponse returns the block response when an unmatured outcome is
// pending (SAVE/RESTORE are game input, #6), else nil.
func (s *Session) blockedResponse() *Response {
	if s.p.Pending == nil || !s.cfg.now().Before(s.p.Pending.MaturesAt) {
		return nil
	}
	return &Response{
		Kind:    KindBlocked,
		Output:  "The outcome of your last action is still unfolding...",
		Status:  s.status(),
		Pending: s.pendingInfo(s.cfg.now()),
	}
}

func sessionCheckpoint(s *Session, now time.Time) session.Checkpoint {
	return session.Checkpoint{
		ID:        fmt.Sprintf("cp-%d", now.Unix()),
		CreatedAt: now,
		Room:      s.p.Room,
		Score:     s.p.Score,
		State:     append([]byte(nil), s.p.Autosave...),
	}
}
