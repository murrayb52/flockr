// Package agents turns harness events (Claude Code hooks, Pi extension events)
// into per-pane agent state.
package agents

import (
	"sort"
	"time"

	"github.com/murrayb52/flockr/internal/model"
)

// Event is the harness-neutral event the hook adapters and the Pi extension send.
//
// Names: start, prompt, tool, input, resume_work, stop, end.
type Event struct {
	Agent     string `json:"agent"`
	Name      string `json:"name"`
	Pane      string `json:"pane"`
	SessionID string `json:"session_id"`
	Source    string `json:"source,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Activity  string `json:"activity,omitempty"`
}

// Change tells the daemon what to do after an event.
type Change struct {
	Pane    string
	Notify  model.Status // StatusInput or StatusDone when a notification is due
	Started bool         // a (new or resumed) session now owns the pane: load its record
	Resumed bool
	Ended   bool
	Changed bool
}

type Table struct {
	byPane map[string]*model.Agent
}

func New() *Table { return &Table{byPane: map[string]*model.Agent{}} }

func (t *Table) Get(pane string) *model.Agent { return t.byPane[pane] }

// Panes returns the panes that currently host an agent, sorted.
func (t *Table) Panes() []string {
	out := make([]string, 0, len(t.byPane))
	for p := range t.byPane {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (t *Table) Apply(ev Event, now time.Time, isFocused func(pane string) bool) Change {
	ch := Change{Pane: ev.Pane}
	if ev.Pane == "" {
		return ch
	}
	a := t.byPane[ev.Pane]
	if ev.Name == "end" {
		if a != nil && (ev.SessionID == "" || a.SessionID == ev.SessionID) {
			delete(t.byPane, ev.Pane)
			ch.Ended, ch.Changed = true, true
		}
		return ch
	}
	if a == nil || (ev.SessionID != "" && a.SessionID != ev.SessionID) {
		a = &model.Agent{Kind: ev.Agent, SessionID: ev.SessionID, State: model.Idle, Since: now}
		t.byPane[ev.Pane] = a
		ch.Started = true
		ch.Resumed = ev.Source == "resume"
	}
	ch.Changed = true
	set := func(s model.AgentState) {
		if a.State != s {
			a.State, a.Since = s, now
		}
	}
	switch ev.Name {
	case "start":
		ch.Resumed = ev.Source == "resume"
		ch.Started = true
	case "prompt":
		set(model.Working)
		a.Prompt, a.Detail, a.Activity, a.Unseen = ev.Prompt, "", "thinking", false
	case "tool", "resume_work":
		set(model.Working)
		a.Unseen = false
		if ev.Activity != "" {
			a.Activity = ev.Activity
		}
	case "input":
		if ev.Detail != "" {
			a.Detail = ev.Detail
		}
		if a.State != model.Input {
			set(model.Input)
			ch.Notify = model.StatusInput
		}
	case "stop":
		set(model.Idle)
		a.Activity = ""
		if ev.Detail != "" {
			a.Detail = ev.Detail
		}
		a.Unseen = !isFocused(ev.Pane)
		if a.Unseen {
			ch.Notify = model.StatusDone
		}
	}
	return ch
}

// Seen marks the pane's agent as looked at. It reports whether anything changed.
func (t *Table) Seen(pane string) bool {
	a := t.byPane[pane]
	if a == nil || !a.Unseen {
		return false
	}
	a.Unseen = false
	return true
}

// Unseen marks the pane's agent as not looked at yet.
func (t *Table) Unseen(pane string) {
	if a := t.byPane[pane]; a != nil {
		a.Unseen = true
	}
}

// Clear forgets the pane's agent (the pane closed or its agent process died).
func (t *Table) Clear(pane string) bool {
	if _, ok := t.byPane[pane]; !ok {
		return false
	}
	delete(t.byPane, pane)
	return true
}
