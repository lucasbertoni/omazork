// Package server speaks the wrapper's NDJSON protocol: one JSON object per
// line over any reader/writer pair (stdio in production, per #4; kept
// transport-agnostic so a socket daemon stays a drop-in upgrade).
package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/session"
	"github.com/lucasbertoni/omazork/internal/tables"
)

var gameTitles = []struct{ ID, Title string }{
	{"zork1", "Zork I"},
	{"zork2", "Zork II"},
	{"zork3", "Zork III"},
}

// request is the union of all incoming message fields.
type request struct {
	Type       string `json:"type"`
	ID         string `json:"id,omitempty"` // echoed back for correlation
	Game       string `json:"game,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Replace    bool   `json:"replace,omitempty"`
	Text       string `json:"text,omitempty"`
	Checkpoint string `json:"checkpoint,omitempty"`
}

// Server owns at most one live game session and serializes writes so the
// maturation watcher can emit events between responses.
type Server struct {
	cfg  game.Config
	sess *game.Session

	mu  sync.Mutex // guards enc
	enc *json.Encoder
}

func New(cfg game.Config) *Server { return &Server{cfg: cfg} }

// Serve reads NDJSON requests until EOF. Every request gets exactly one
// response line; malformed input gets an error line rather than a dropped
// connection.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	s.enc = json.NewEncoder(w)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.emit(map[string]any{"type": "error", "message": "malformed request: " + err.Error()})
			continue
		}
		s.emit(s.handle(req))
	}
	return sc.Err()
}

func (s *Server) emit(msg any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(msg)
}

// CheckMaturation emits one matured event when a pending outcome has matured
// and not yet been notified (#5: one non-spoiler notification). Safe to call
// from a ticker goroutine.
func (s *Server) CheckMaturation() {
	if msg := s.maturationEvent(); msg != nil {
		s.emit(msg)
	}
}

func (s *Server) maturationEvent() map[string]any {
	if s.sess == nil || !s.sess.PendingMatured() {
		return nil
	}
	if err := s.sess.MarkNotified(); err != nil {
		return map[string]any{"type": "error", "message": err.Error()}
	}
	return map[string]any{"type": "matured", "game": s.sess.Game()}
}

func (s *Server) handle(req request) any {
	resp, err := s.dispatch(req)
	if err != nil {
		if errors.Is(err, game.ErrPlaythroughExists) {
			return withID(req, map[string]any{"type": "confirm-replace", "game": req.Game})
		}
		return withID(req, map[string]any{"type": "error", "message": err.Error()})
	}
	return withID(req, resp)
}

func withID(req request, msg any) any {
	if req.ID == "" {
		return msg
	}
	m, ok := msg.(map[string]any)
	if !ok {
		raw, _ := json.Marshal(msg)
		m = map[string]any{}
		_ = json.Unmarshal(raw, &m)
	}
	m["id"] = req.ID
	return m
}

// responseMsg flattens a game.Response into a protocol message keyed by Kind.
func responseMsg(resp game.Response) map[string]any {
	raw, _ := json.Marshal(resp)
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	m["type"] = resp.Kind
	delete(m, "kind")
	return m
}

func (s *Server) dispatch(req request) (any, error) {
	switch req.Type {
	case "picker":
		return s.picker()
	case "new":
		mode := session.Mode(req.Mode)
		if mode != session.Classic && mode != session.Casual {
			return nil, fmt.Errorf("unknown mode %q", req.Mode)
		}
		var (
			sess *game.Session
			resp game.Response
			err  error
		)
		if req.Replace {
			sess, resp, err = game.Replace(s.cfg, req.Game, mode)
		} else {
			sess, resp, err = game.New(s.cfg, req.Game, mode)
		}
		if err != nil {
			return nil, err
		}
		s.swapSession(sess)
		return responseMsg(resp), nil
	case "resume":
		sess, resp, err := game.Resume(s.cfg, req.Game)
		if err != nil {
			return nil, err
		}
		s.swapSession(sess)
		m := responseMsg(resp)
		m["transcript"] = sess.Transcript()
		return m, nil
	case "input":
		if s.sess == nil {
			return nil, errors.New("no active session")
		}
		resp, err := s.sess.Command(req.Text)
		if err != nil {
			return nil, err
		}
		return responseMsg(resp), nil
	case "restore-checkpoint":
		if s.sess == nil {
			return nil, errors.New("no active session")
		}
		resp, err := s.sess.RestoreCheckpoint(req.Checkpoint)
		if err != nil {
			return nil, err
		}
		return responseMsg(resp), nil
	case "opened":
		if s.sess == nil {
			return map[string]any{"type": "output"}, nil
		}
		resp, err := s.sess.Opened()
		if err != nil {
			return nil, err
		}
		return responseMsg(resp), nil
	case "closed":
		if s.sess != nil {
			if err := s.sess.Closed(); err != nil {
				return nil, err
			}
		}
		return map[string]any{"type": "closed"}, nil
	case "stats":
		if s.sess == nil {
			return nil, errors.New("no active session")
		}
		return map[string]any{"type": "stats", "game": s.sess.Game(), "stats": s.sess.Stats()}, nil
	case "achievements":
		return s.achievements(req.Game)
	case "check-maturation":
		if msg := s.maturationEvent(); msg != nil {
			return msg, nil
		}
		return map[string]any{"type": "quiet"}, nil
	default:
		return nil, fmt.Errorf("unknown request type %q", req.Type)
	}
}

// swapSession closes the playtime clock on any previous session.
func (s *Server) swapSession(next *game.Session) {
	if s.sess != nil {
		_ = s.sess.Closed()
	}
	s.sess = next
}

// Shutdown flushes playtime accounting (SIGTERM: autosave is already per-turn).
func (s *Server) Shutdown() {
	if s.sess != nil {
		_ = s.sess.Closed()
	}
}

func (s *Server) picker() (any, error) {
	type ptView struct {
		Mode       session.Mode `json:"mode"`
		Room       string       `json:"room"`
		Score      int          `json:"score"`
		Moves      int          `json:"moves"`
		LastPlayed time.Time    `json:"lastPlayed"`
		Pending    bool         `json:"pending"`
		// When Pending: lets the bar icon rediscover a recap that matured
		// while the wrapper was not running (no session, no matured event).
		PendingMaturesAt *time.Time `json:"pendingMaturesAt,omitempty"`
		Finished         bool       `json:"finished"`
	}
	type entry struct {
		Game        string  `json:"game"`
		Title       string  `json:"title"`
		Playthrough *ptView `json:"playthrough"`
	}
	var out []entry
	for _, g := range gameTitles {
		e := entry{Game: g.ID, Title: g.Title}
		if p, ok, err := s.cfg.Store.LoadPlaythrough(g.ID); err != nil {
			return nil, err
		} else if ok {
			e.Playthrough = &ptView{
				Mode: p.Mode, Room: p.Room, Score: p.Score, Moves: p.Moves,
				LastPlayed: p.LastPlayed, Pending: p.Pending != nil, Finished: p.Finished,
			}
			if p.Pending != nil {
				at := p.Pending.MaturesAt
				e.Playthrough.PendingMaturesAt = &at
			}
		}
		out = append(out, e)
	}
	return map[string]any{"type": "picker", "games": out}, nil
}

func (s *Server) achievements(gameName string) (any, error) {
	achs, err := tables.LoadAchievements(gameName)
	if err != nil {
		return nil, err
	}
	states, err := game.AchievementsFor(s.cfg.Store, gameName, achs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": "achievements", "game": gameName, "achievements": states}, nil
}
