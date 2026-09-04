package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaude writes a stand-in `claude` that records its arguments and stdin
// and prints a canned result envelope.
func fakeClaude(t *testing.T, envelope string) (binary, log string) {
	dir := t.TempDir()
	binary = filepath.Join(dir, "claude")
	log = filepath.Join(dir, "log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + log + "\npwd >> " + log + "\ncat >> " + log + "\nprintf '%s' '" + envelope + "'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, log
}

func TestCLIRunsHeadlessWithThePinnedModel(t *testing.T) {
	binary, log := fakeClaude(t, `{"is_error":false,"result":"{\"class\":\"movement\"}","subtype":"success"}`)
	cli := NewCLI()
	cli.Binary = binary
	text, err := cli.Infer(context.Background(), "price this")
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if text != `{"class":"movement"}` {
		t.Fatalf("text = %q", text)
	}
	got, _ := os.ReadFile(log)
	if cwd, _ := os.Getwd(); strings.Contains(string(got), cwd+"\n") {
		t.Errorf("ran in the caller's directory, where a CLAUDE.md could be picked up:\n%s", got)
	}
	for _, want := range []string{"-p", "--model " + ModelID, "--tools ", "--max-turns 1", "--output-format json", "\nprice this"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("invocation lacks %q:\n%s", want, got)
		}
	}
}

func TestCLIReportsToolErrors(t *testing.T) {
	binary, _ := fakeClaude(t, `{"is_error":true,"result":"Not logged in · Please run /login"}`)
	cli := &CLI{Binary: binary, Model: ModelID}
	if _, err := cli.Infer(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "Not logged in") {
		t.Fatalf("err = %v, want the tool's message", err)
	}
}
