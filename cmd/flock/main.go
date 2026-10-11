// Command flock is a keyboard-first i3 companion for CLI coding agents.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/protocol"
)

var version = "dev"

const usage = `flock — a sidebar, labels, notes and notifications for CLI agents in i3

Usage:
  flock daemon                 run the daemon (start it from your i3 config)
  flock term [-- command...]   start a flock terminal (run it as your terminal's command)
  flock install claude|pi|all  add the Claude Code hooks / Pi extension
  flock uninstall claude|pi|all

Bindings (call these from i3):
  flock sidebar [toggle|condense|expand|show]
  flock label                  name the focused terminal or agent
  flock notes                  open/close the focused agent's notes
  flock next                   focus the next agent that needs you
  flock slot N                 move the focused window to position N in its stack
  flock jump CON_ID            focus a window

Feeds and tools:
  flock bar [--plain]          status line for polybar (custom/script, tail = true)
  flock status                 print what flock sees
  flock tmux-config            print the private tmux configuration
  flock version
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "flock:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	// hooks must stay silent and fast whatever happens, so they skip config errors
	if args[0] == "hook" {
		runHook(args[1:])
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "daemon":
		return runDaemon(cfg)
	case "term":
		return runTerm(cfg, args[1:])
	case "sidebar":
		if len(args) == 1 {
			return runSidebar(cfg)
		}
		return command(cfg, args...)
	case "label", "notes", "next", "slot", "jump", "refresh":
		return command(cfg, args...)
	case "notes-ui":
		if len(args) != 3 || args[1] != "--pane" {
			return fmt.Errorf("usage: flock notes-ui --pane PANE")
		}
		return runNotesUI(cfg, args[2])
	case "bar":
		return runBar(cfg, len(args) > 1 && args[1] == "--plain")
	case "status":
		return runStatus(cfg)
	case "install", "uninstall":
		return runInstall(args[0] == "install", args[1:])
	case "tmux-config":
		fmt.Print(tmuxConfig())
		return nil
	case "version", "--version", "-v":
		fmt.Println("flock", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q (see `flock help`)", args[0])
}

// command forwards a binding command to the daemon.
func command(cfg config.Config, args ...string) error {
	resp, err := protocol.Call(cfg.Socket, protocol.Request{Op: protocol.OpCommand, Args: args}, 2*time.Second)
	if err != nil {
		return err
	}
	if resp.Message != "" {
		fmt.Println(resp.Message)
	}
	return nil
}

// binPath is the absolute path of this executable (used in hooks and popups).
func binPath() string {
	p, err := os.Executable()
	if err != nil {
		return "flock"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func userTmuxConf() string {
	return filepath.Join(filepath.Dir(config.Path()), "tmux.conf")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
