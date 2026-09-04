package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CLI infers through the Claude Code command-line tool in headless mode, so a
// pass runs on the user's Claude subscription instead of an API key. Each
// prompt is one `claude -p` process with tools off, one turn, no session on
// disk, settings files ignored, and a fixed system prompt in place of the
// tool's own. It runs in a fresh empty directory so no CLAUDE.md or auto-memory
// is discovered: the call carries the same two things the API client sends,
// the pinned model and the rendered prompt. (The tool still adds a few lines
// of its own — the date, the platform, the model name — which `--bare` would
// drop, but `--bare` also drops subscription auth, so they stay.)
type CLI struct {
	Binary string
	Model  string
}

// NewCLI returns a client for the pinned model.
func NewCLI() *CLI {
	return &CLI{Binary: "claude", Model: ModelID}
}

// cliSystemPrompt replaces Claude Code's own system prompt, which changes
// with every release and would otherwise drift under the hash.
const cliSystemPrompt = "Answer the user's question directly, with the JSON they ask for and nothing else."

type cliResult struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Subtype string `json:"subtype"`
}

// Infer runs one headless call and returns the assistant's text.
func (c *CLI) Infer(ctx context.Context, prompt string) (string, error) {
	dir, err := os.MkdirTemp("", "actiongen-llm-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	cmd := exec.CommandContext(ctx, c.Binary,
		"-p",
		"--model", c.Model,
		"--system-prompt", cliSystemPrompt,
		"--tools", "",
		"--max-turns", "1",
		"--no-session-persistence",
		"--setting-sources", "",
		"--output-format", "json",
	)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude: %v: %s", err, clip(strings.TrimSpace(stderr.String()+stdout.String())))
	}
	var parsed cliResult
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return "", fmt.Errorf("claude: unreadable result: %s", clip(stdout.String()))
	}
	if parsed.IsError {
		return "", fmt.Errorf("claude: %s", clip(parsed.Result))
	}
	if strings.TrimSpace(parsed.Result) == "" {
		return "", fmt.Errorf("claude: empty result (%s)", parsed.Subtype)
	}
	return parsed.Result, nil
}
