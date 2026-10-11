package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murrayb52/flockr/internal/agents"
	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/hook"
	"github.com/murrayb52/flockr/internal/protocol"
)

// runHook handles `flockr hook claude <Event>` from Claude Code. It never prints
// and never fails: stdout of some hooks is fed back to the model, and a failing
// hook would show up in the agent's UI.
func runHook(args []string) {
	if len(args) != 2 || args[0] != "claude" {
		return
	}
	pane := os.Getenv("TMUX_PANE")
	sock := strings.SplitN(os.Getenv("TMUX"), ",", 2)[0]
	tmuxName := os.Getenv("FLOCKR_TMUX")
	if tmuxName == "" {
		tmuxName = "flockr"
	}
	if pane == "" || filepath.Base(sock) != tmuxName {
		return // not running in a flockr terminal
	}
	payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return
	}
	ev, ok := hook.FromClaude(args[1], payload, pane)
	if !ok {
		return
	}
	socket := os.Getenv("FLOCKR_SOCKET")
	if socket == "" {
		cfg, _ := config.Load()
		socket = cfg.Socket
	}
	protocol.Call(socket, protocol.Request{Op: protocol.OpEvent, Event: &ev}, 300*time.Millisecond)
}

// runEvent reports an agent event from a script or another harness.
func runEvent(cfg config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: flockr event AGENT NAME session=ID [prompt=…] [detail=…] [activity=…] [source=…]")
	}
	ev := agents.Event{Agent: args[0], Name: args[1], Pane: os.Getenv("TMUX_PANE")}
	for _, kv := range args[2:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("expected key=value, got %q", kv)
		}
		switch k {
		case "session":
			ev.SessionID = v
		case "prompt":
			ev.Prompt = hook.Trim(v, 200)
		case "detail":
			ev.Detail = hook.Trim(v, 160)
		case "activity":
			ev.Activity = hook.Trim(v, 80)
		case "source":
			ev.Source = v
		default:
			return fmt.Errorf("unknown field %q", k)
		}
	}
	if ev.Pane == "" || ev.SessionID == "" {
		return fmt.Errorf("run inside a flockr terminal and pass session=ID")
	}
	_, err := protocol.Call(cfg.Socket, protocol.Request{Op: protocol.OpEvent, Event: &ev}, time.Second)
	return err
}
