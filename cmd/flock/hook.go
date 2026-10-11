package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/hook"
	"github.com/murrayb52/flock/internal/protocol"
)

// runHook handles `flock hook claude <Event>` from Claude Code. It never prints
// and never fails: stdout of some hooks is fed back to the model, and a failing
// hook would show up in the agent's UI.
func runHook(args []string) {
	if len(args) != 2 || args[0] != "claude" {
		return
	}
	pane := os.Getenv("TMUX_PANE")
	sock := strings.SplitN(os.Getenv("TMUX"), ",", 2)[0]
	tmuxName := os.Getenv("FLOCK_TMUX")
	if tmuxName == "" {
		tmuxName = "flock"
	}
	if pane == "" || filepath.Base(sock) != tmuxName {
		return // not running in a flock terminal
	}
	payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return
	}
	ev, ok := hook.FromClaude(args[1], payload, pane)
	if !ok {
		return
	}
	socket := os.Getenv("FLOCK_SOCKET")
	if socket == "" {
		cfg, _ := config.Load()
		socket = cfg.Socket
	}
	protocol.Call(socket, protocol.Request{Op: protocol.OpEvent, Event: &ev}, 300*time.Millisecond)
}
