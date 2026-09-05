package calibrate

import (
	"fmt"
	"sort"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/engine"
)

// Replay is the cumulative wait profile of one walkthrough (§6): every turn of
// the committed script classified by the shared matcher and priced by the
// shared lookup, so the profile is what a player following the walkthrough
// would actually wait.
type Replay struct {
	Script                string            `json:"script"`
	Seed                  uint64            `json:"seed"`
	Turns                 int               `json:"turns"`
	CumulativeSeconds     int               `json:"cumulativeSeconds"`
	CumulativeHours       float64           `json:"cumulativeHours"`
	MedianMovementSeconds float64           `json:"medianMovementSeconds"`
	TurnsUnderQuiet       float64           `json:"turnsUnderQuiet"` // share of turns under the quiet threshold
	ByKind                map[string]Bucket `json:"byKind"`
	BySlot                map[string]Bucket `json:"bySlot"`
	Waits                 []Bin             `json:"waits"`
	LongestTurns          []TurnWait        `json:"longestTurns"`
}

// Bucket tallies turns and their summed wait.
type Bucket struct {
	Turns   int `json:"turns"`
	Seconds int `json:"seconds"`
}

// Bin is one bar of a seconds histogram.
type Bin struct {
	Seconds int `json:"seconds"`
	Count   int `json:"count"`
}

// TurnWait is one priced turn, for the report's list of the longest waits.
type TurnWait struct {
	Turn    int    `json:"turn"`
	Input   string `json:"input"`
	Kind    string `json:"kind"`
	Seconds int    `json:"seconds"`
	Slot    string `json:"slot"`
	Key     string `json:"key,omitempty"`
}

// longestTurns is how many of the walkthrough's longest waits the report lists.
const longestTurns = 10

// replay runs the fixture and returns the profile plus the set of row keys the
// walkthrough exercised.
func replay(l *durations.Layered, fx actions.Fixture, script []byte) (Replay, map[string]bool, error) {
	game := l.Game
	cmds := actions.ParseScript(script)
	e, err := engine.New(game, engine.WithSeed(fx.Seed))
	if err != nil {
		return Replay{}, nil, err
	}
	m, err := actions.NewMatcher(game)
	if err != nil {
		return Replay{}, nil, err
	}
	banner, err := e.Start()
	if err != nil {
		return Replay{}, nil, err
	}
	state := actions.StartState(actions.Turn{Room: banner.Room, RoomObj: banner.RoomObj, Moves: banner.Moves})

	rep := Replay{Script: fx.Script, Seed: fx.Seed, ByKind: map[string]Bucket{}, BySlot: map[string]Bucket{}}
	hit := map[string]bool{}
	waits := map[int]int{}
	var movement []int
	var all []TurnWait
	under := 0
	for i, cmd := range cmds {
		turn, err := e.Run(cmd)
		if err != nil {
			return Replay{}, nil, fmt.Errorf("%s: command %d %q: %w", game, i+1, cmd, err)
		}
		if turn.Halted && i != len(cmds)-1 {
			return Replay{}, nil, fmt.Errorf("%s: story halted at command %d of %d (%q) — fixture drifted from its seed", game, i+1, len(cmds), cmd)
		}
		res := m.Classify(state, actions.Turn{Input: cmd, Output: turn.Output, Room: turn.Room, RoomObj: turn.RoomObj, Moves: turn.Moves})
		state = res.State

		r := l.Resolve(durations.QueryFor(res))
		seconds := r.Row.Seconds
		rep.Turns++
		rep.CumulativeSeconds += seconds
		add(rep.ByKind, res.Kind.String(), seconds)
		add(rep.BySlot, string(r.Slot), seconds)
		waits[seconds]++
		if seconds < durations.QuietThreshold {
			under++
		}
		if res.Kind == actions.Movement {
			movement = append(movement, seconds)
		}
		if r.Key != "" {
			hit[r.Key] = true
		}
		all = append(all, TurnWait{Turn: i + 1, Input: cmd, Kind: res.Kind.String(), Seconds: seconds, Slot: string(r.Slot), Key: r.Key})
	}

	rep.CumulativeHours = round(float64(rep.CumulativeSeconds) / 3600)
	sort.Ints(movement)
	rep.MedianMovementSeconds = median(movement)
	rep.TurnsUnderQuiet = share(under, rep.Turns)
	rep.Waits = bins(waits)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Seconds > all[j].Seconds })
	if len(all) > longestTurns {
		all = all[:longestTurns]
	}
	rep.LongestTurns = all
	return rep, hit, nil
}

// add tallies one turn into a bucket map.
func add(m map[string]Bucket, key string, seconds int) {
	b := m[key]
	b.Turns++
	b.Seconds += seconds
	m[key] = b
}

// bins renders a seconds→count map as bars in ascending order.
func bins(counts map[int]int) []Bin {
	out := make([]Bin, 0, len(counts))
	for seconds, n := range counts {
		out = append(out, Bin{Seconds: seconds, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seconds < out[j].Seconds })
	return out
}
