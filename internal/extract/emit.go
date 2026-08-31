package extract

import (
	"encoding/json"
	"sort"
)

// JSON renders the extract deterministically: rooms, edges, objects, syntax,
// and synonyms in source order; routines sorted by name; map keys sorted by
// encoding/json. Re-running extraction produces an identical file.
func (x *Extract) JSON() ([]byte, error) {
	out, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func sortRoutines(rs []Routine) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Name < rs[j].Name })
}
