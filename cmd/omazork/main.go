// Command omazork is the plugin's backend: a self-contained wrapper around an
// embedded Z-machine, speaking NDJSON over stdio to the QML console (#4).
// The shell spawns it as a keepLoaded Process child and SIGTERMs it on plugin
// rescans; durability is on this side — every turn is autosaved, so a respawn
// resumes invisibly (#6).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/server"
	"github.com/lucasbertoni/omazork/internal/session"
)

func main() {
	stateDir := flag.String("state", defaultStateDir(), "directory holding playthroughs and achievement unlocks")
	tick := flag.Duration("maturation-tick", 30*time.Second, "how often to check for matured outcomes")
	flag.Parse()

	srv := server.New(game.Config{Store: session.NewStore(*stateDir)})

	// A matured pending outcome fires one non-spoiler notification (#5); the
	// QML side turns this event into the desktop notification.
	go func() {
		for range time.Tick(*tick) {
			srv.CheckMaturation()
		}
	}()

	// SIGTERM is the shell killing its child on rescan/restart: flush playtime
	// accounting and exit cleanly. The autosave is already on disk.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		srv.Shutdown()
		os.Exit(0)
	}()

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "omazork: %v\n", err)
		os.Exit(1)
	}
	srv.Shutdown()
}

// defaultStateDir is ~/.local/state/omazork (XDG_STATE_HOME), which survives
// plugin remove/update (#6).
func defaultStateDir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "omazork")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "omazork-state"
	}
	return filepath.Join(home, ".local", "state", "omazork")
}
