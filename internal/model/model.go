// Package model holds the types shared by the daemon and its clients
// (sidebar, bar, notes editor). Snapshots travel over the socket as JSON.
package model

import "time"

// AgentState is what an agent harness reported last.
type AgentState string

const (
	Working AgentState = "working"
	Input   AgentState = "input" // waiting on the user: permission prompt, dialog or question
	Idle    AgentState = "idle"
)

// Status is the display state, derived from an agent (or a workspace of agents).
type Status string

const (
	StatusNone    Status = ""
	StatusInput   Status = "input"
	StatusDone    Status = "done" // idle and not yet looked at
	StatusWorking Status = "working"
	StatusIdle    Status = "idle"
)

type Note struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Agent is a Claude Code or Pi session running in a flock terminal.
type Agent struct {
	Kind      string     `json:"kind"` // "claude" | "pi"
	SessionID string     `json:"session_id"`
	State     AgentState `json:"state"`
	Unseen    bool       `json:"unseen"`
	Since     time.Time  `json:"since"`
	Prompt    string     `json:"prompt,omitempty"` // latest prompt, used as a name until labelled
	Detail    string     `json:"detail,omitempty"` // permission command, question or last message
	Activity  string     `json:"activity,omitempty"`
	Label     string     `json:"label,omitempty"`
	Notes     []Note     `json:"notes,omitempty"`
}

// Term is a flock terminal: one tmux session on the private flock server.
type Term struct {
	Session string `json:"session"`
	Pane    string `json:"pane"`
	Cwd     string `json:"cwd"`
	Command string `json:"command"` // foreground command in the pane
	Label   string `json:"label,omitempty"`
	Agent   *Agent `json:"agent,omitempty"`
}

// Window is an i3 window as the sidebar shows it.
type Window struct {
	ConID   int64  `json:"con_id"`
	XID     int64  `json:"xid"`
	Class   string `json:"class"`
	Title   string `json:"title"`
	App     string `json:"app"`  // normalised app key, e.g. "firefox", "term"
	Slot    int    `json:"slot"` // 1-based position inside a stack/tabs container, 0 otherwise
	Focused bool   `json:"focused"`
	Visible bool   `json:"visible"`
	Term    *Term  `json:"term,omitempty"`
}

// Group is one top-level child of a workspace: a lone window or a container.
type Group struct {
	Label   string   `json:"label"` // "left", "right · stack of 3", ...
	Layout  string   `json:"layout"`
	Focused bool     `json:"focused"`
	Windows []Window `json:"windows"`
}

type Workspace struct {
	Num     int64   `json:"num"`
	Name    string  `json:"name"` // display name without the "N:" prefix
	Focused bool    `json:"focused"`
	Visible bool    `json:"visible"`
	Urgent  bool    `json:"urgent"`
	Output  string  `json:"output"`
	Groups  []Group `json:"groups"`
}

// Windows returns every window in the workspace in display order.
func (w Workspace) Windows() []Window {
	var out []Window
	for _, g := range w.Groups {
		out = append(out, g.Windows...)
	}
	return out
}

// Snapshot is the full state the daemon broadcasts on every change.
type Snapshot struct {
	Workspaces []Workspace `json:"workspaces"`
	Condensed  bool        `json:"condensed"`
	Font       float64     `json:"font,omitempty"` // sidebar font size (points)
	Now        time.Time   `json:"now"`
}

// Current returns the focused workspace, or false if there is none.
func (s Snapshot) Current() (Workspace, bool) {
	for _, w := range s.Workspaces {
		if w.Focused {
			return w, true
		}
	}
	return Workspace{}, false
}
