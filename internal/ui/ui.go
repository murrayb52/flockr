// Package ui is the shared visual vocabulary: names, icon tiles, status logos
// and colours used by the sidebar, the bar and the tmux header.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/murrayb52/flock/internal/model"
)

// Palette (matches the approved mockup and the user's i3 colours).
const (
	ColText   = "#f3f4f5"
	ColDim    = "#676E7D"
	ColFaint  = "#4b5263"
	ColActive = "#2D6A4F"
	ColOK     = "#52b788"
	ColRed    = "#E53935"
	ColAmber  = "#f0b429"
	ColBlue   = "#8ea0c8"
	ColBG     = "#14171c"
	ColRowBG  = "#22352b"
)

var Spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var kindNames = map[string]string{"claude": "Claude Code", "pi": "pi"}

var appNames = map[string]string{
	"firefox": "Firefox", "chrome": "Chrome", "code": "VS Code", "obsidian": "Obsidian",
	"mail": "Mail", "chat": "Chat", "files": "Files", "terminal": "Terminal", "window": "Window",
}

var classNames = map[string]string{
	"tidal-hifi": "Tidal", "spotify": "Spotify", "rhythmbox": "Rhythmbox", "tutanota-desktop": "Tuta Mail",
	"thunderbird": "Thunderbird", "slack": "Slack", "discord": "Discord", "signal": "Signal",
}

// KindName is the human name of an agent harness.
func KindName(kind string) string {
	if n, ok := kindNames[kind]; ok {
		return n
	}
	return kind
}

// Name returns what a window is called and whether it was derived automatically
// (shown in italics): label > latest prompt > "<program> — <dir>" > app name.
func Name(w model.Window) (string, bool) {
	if t := w.Term; t != nil {
		if a := t.Agent; a != nil {
			if a.Label != "" {
				return a.Label, false
			}
			if a.Prompt != "" {
				return a.Prompt, true
			}
			return KindName(a.Kind) + " — " + filepath.Base(t.Cwd), true
		}
		if t.Label != "" {
			return t.Label, false
		}
		if k := commandKind(t.Command); k != "" {
			return KindName(k) + " — " + filepath.Base(t.Cwd), true
		}
		cmd := t.Command
		if cmd == "" {
			cmd = "shell"
		}
		return cmd + " — " + filepath.Base(t.Cwd), true
	}
	if n, ok := classNames[strings.ToLower(w.Class)]; ok {
		return n, false
	}
	if n, ok := appNames[w.App]; ok {
		return n, false
	}
	return titleCase(w.Class), false
}

func titleCase(s string) string {
	if s == "" {
		return "Window"
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func StatusOf(a *model.Agent) model.Status {
	if a == nil {
		return model.StatusNone
	}
	switch {
	case a.State == model.Input:
		return model.StatusInput
	case a.State == model.Working:
		return model.StatusWorking
	case a.Unseen:
		return model.StatusDone
	}
	return model.StatusIdle
}

func agentOf(w model.Window) *model.Agent {
	if w.Term == nil {
		return nil
	}
	return w.Term.Agent
}

// AgentOf returns the window's agent, if any.
func AgentOf(w model.Window) *model.Agent { return agentOf(w) }

var priority = map[model.Status]int{model.StatusInput: 4, model.StatusDone: 3, model.StatusWorking: 2, model.StatusIdle: 1}

// WorkspaceStatus is the most urgent status among the workspace's agents.
func WorkspaceStatus(ws model.Workspace) model.Status {
	best := model.StatusNone
	for _, w := range ws.Windows() {
		if s := StatusOf(agentOf(w)); priority[s] > priority[best] {
			best = s
		}
	}
	return best
}

// Logo returns the glyph and colour for a status; frame animates "working".
func Logo(s model.Status, frame int) (string, string) {
	switch s {
	case model.StatusInput:
		return "!", ColRed
	case model.StatusDone:
		return "✓", ColOK
	case model.StatusWorking:
		return Spinner[frame%len(Spinner)], ColAmber
	case model.StatusIdle:
		return "○", ColDim
	}
	return "", ""
}

func StateText(a *model.Agent) string {
	switch StatusOf(a) {
	case model.StatusWorking:
		if a.Activity != "" {
			return a.Activity
		}
		return "working"
	case model.StatusInput:
		return "needs input"
	case model.StatusDone:
		return "done"
	case model.StatusIdle:
		return "idle"
	}
	return ""
}

func Elapsed(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// Home abbreviates home in a path as ~.
func Home(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + path[len(home):]
	}
	return path
}

// Sub is the secondary line under a window's name.
func Sub(w model.Window, now time.Time, home string) string {
	if t := w.Term; t != nil {
		if a := t.Agent; a != nil {
			return StateText(a) + " · " + Elapsed(now.Sub(a.Since))
		}
		cmd := t.Command
		if cmd == "" {
			cmd = "shell"
		}
		return cmd + " · " + Home(t.Cwd, home)
	}
	title := w.Title
	for _, sep := range []string{" — ", " - ", " – "} {
		if i := strings.LastIndex(title, sep); i > 0 {
			title = title[:i]
			break
		}
	}
	return title
}

type TileStyle struct{ Glyph, BG, FG string }

var tiles = map[string]TileStyle{
	"claude": {"✻", "#d97757", "#ffffff"}, "pi": {"π", "#6d4fd8", "#ffffff"}, "shell": {"›_", "#2b303b", "#7ee787"},
	"firefox": {"◐", "#e66000", "#ffd567"}, "chrome": {"◎", "#1a73e8", "#ffffff"}, "code": {"{}", "#1f6feb", "#ffffff"},
	"obsidian": {"◆", "#4c3a8f", "#ffffff"}, "music": {"♪", "#000000", "#ffffff"}, "mail": {"✉", "#b23a48", "#ffffff"},
	"chat": {"✆", "#4a154b", "#ffffff"}, "files": {"▤", "#5c6b7a", "#ffffff"}, "terminal": {"›_", "#2b303b", "#c8ccd4"},
}

// commandKind recognises an agent from the pane's foreground command, so the
// icon changes as soon as it starts (and without hooks installed).
func commandKind(cmd string) string {
	switch cmd {
	case "claude", "pi":
		return cmd
	}
	return ""
}

// Tile is the coloured glyph icon for a window.
func Tile(w model.Window) TileStyle {
	if t := w.Term; t != nil {
		kind := commandKind(t.Command)
		if t.Agent != nil {
			kind = t.Agent.Kind
		}
		if s, ok := tiles[kind]; ok {
			return s
		}
		return tiles["shell"]
	}
	if s, ok := tiles[w.App]; ok {
		return s
	}
	first := "?"
	for _, r := range w.Class {
		first = string(unicode.ToUpper(r))
		break
	}
	return TileStyle{first, "#3a3f4b", ColText}
}

// NotesCount returns "done/total" for an agent's notes, or "" without notes.
func NotesCount(a *model.Agent) (done, total int) {
	if a == nil {
		return 0, 0
	}
	for _, n := range a.Notes {
		if n.Done {
			done++
		}
	}
	return done, len(a.Notes)
}
