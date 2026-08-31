package actions

import "strings"

// ParseScript reads a walkthrough fixture script: one command per line,
// blank lines and #-comments skipped. The committed (script, seed) fixtures
// under testdata/ are the calibration ground truth (spec §6.1).
func ParseScript(src []byte) []string {
	var cmds []string
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cmds = append(cmds, line)
	}
	return cmds
}
