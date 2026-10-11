package main

import (
	"fmt"
	"os"
	"time"

	"github.com/murrayb52/flock/internal/bar"
	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/install"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/notesui"
	"github.com/murrayb52/flock/internal/protocol"
	"github.com/murrayb52/flock/internal/sidebar"
	"github.com/murrayb52/flock/internal/ui"
)

func runSidebar(cfg config.Config) error { return sidebar.Run(cfg.Socket) }

func runNotesUI(cfg config.Config, pane string) error { return notesui.Run(cfg.Socket, pane) }

// runBar prints a line whenever the bar text changes, reconnecting forever.
func runBar(cfg config.Config, plain bool) error {
	style := bar.Polybar
	if plain {
		style = bar.Plain
	}
	last := "\x00"
	emit := func(s string) {
		if s != last {
			fmt.Println(s)
			last = s
		}
	}
	for {
		ch, _, err := protocol.Subscribe(cfg.Socket)
		if err != nil {
			emit("")
			time.Sleep(2 * time.Second)
			continue
		}
		var snap model.Snapshot
		frame := 0
		tick := time.NewTicker(250 * time.Millisecond)
	stream:
		for {
			select {
			case s, ok := <-ch:
				if !ok {
					break stream
				}
				snap = s
			case <-tick.C:
				frame++
			}
			emit(bar.Format(snap, style, frame))
		}
		tick.Stop()
	}
}

func runStatus(cfg config.Config) error {
	ch, closeFn, err := protocol.Subscribe(cfg.Socket)
	if err != nil {
		return err
	}
	defer closeFn()
	snap, ok := <-ch
	if !ok {
		return fmt.Errorf("daemon closed the connection")
	}
	home, _ := os.UserHomeDir()
	for _, ws := range snap.Workspaces {
		mark := " "
		if ws.Focused {
			mark = "*"
		}
		fmt.Printf("%s %d %s\n", mark, ws.Num, ws.Name)
		for _, g := range ws.Groups {
			if g.Label != "" {
				fmt.Printf("    [%s]\n", g.Label)
			}
			for _, w := range g.Windows {
				name, _ := ui.Name(w)
				fmt.Printf("      %-28s %s\n", name, ui.Sub(w, time.Now(), home))
			}
		}
	}
	return nil
}

func runInstall(add bool, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: flock install|uninstall claude|pi|all")
	}
	what := args[0]
	verb := map[bool]string{true: "installed", false: "removed"}[add]
	if what == "claude" || what == "all" {
		path, err := install.Claude(binPath(), add)
		if err != nil {
			return fmt.Errorf("claude: %w", err)
		}
		fmt.Printf("Claude Code hooks %s in %s\n", verb, path)
	}
	if what == "pi" || what == "all" {
		path, err := install.Pi(add)
		if err != nil {
			return fmt.Errorf("pi: %w", err)
		}
		fmt.Printf("Pi extension %s: %s\n", verb, path)
	}
	if what != "claude" && what != "pi" && what != "all" {
		return fmt.Errorf("unknown integration %q (claude, pi or all)", what)
	}
	return nil
}
