// Command flockr is a keyboard-first i3 companion for CLI coding agents.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/protocol"
)

var version = "dev"

const usage = `flockr — labels, notes and notifications for CLI agents in i3

Usage:
  flockr daemon                 run the daemon (start it from your i3 config)
  flockr term [-- command...]   start a flockr terminal (run it as your terminal's command)
  flockr install claude|pi|all  add the Claude Code hooks / Pi extension
  flockr uninstall claude|pi|all

Bindings (call these from i3):
  flockr notifications [on|off|toggle]
  flockr label                  name the focused terminal or agent
  flockr wslabel [LABEL]        name the focused workspace (stored separately from i3)
  flockr notes                  open/close the focused agent's notes
  flockr next                   focus the next agent that needs you

Other agents:
  flockr event AGENT NAME session=ID [prompt=…] [detail=…] [activity=…] [source=…]
                               report state from any harness (NAME: start, prompt,
                               tool, input, resume_work, stop, end); run inside a
                               flockr terminal

Feeds and tools:
  flockr bar [--plain]          status line for polybar (custom/script, tail = true)
  flockr status                 print what flockr sees
  flockr tmux-config            print the private tmux configuration
  flockr version
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "flockr:", err)
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
	case "label", "wslabel", "notes", "next", "refresh", "notifications":
		return command(cfg, args...)
	case "event":
		return runEvent(cfg, args[1:])
	case "notes-ui":
		if len(args) != 3 || args[1] != "--pane" {
			return fmt.Errorf("usage: flockr notes-ui --pane PANE")
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
		fmt.Println("flockr", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q (see `flockr help`)", args[0])
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
		return "flockr"
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
