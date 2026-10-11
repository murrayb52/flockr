package agents

import (
	"testing"
	"time"

	"github.com/murrayb52/flock/internal/model"
)

var t0 = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func notFocused(string) bool { return false }
func focused(string) bool    { return true }

func apply(tb *Table, ev Event, f func(string) bool) Change {
	return tb.Apply(ev, t0, f)
}

func TestStartCreatesIdleAgent(t *testing.T) {
	tb := New()
	ch := apply(tb, Event{Agent: "claude", Name: "start", Pane: "%1", SessionID: "s1", Source: "startup"}, notFocused)
	a := tb.Get("%1")
	if a == nil || a.State != model.Idle || a.Unseen || a.SessionID != "s1" || a.Kind != "claude" {
		t.Fatalf("agent = %+v", a)
	}
	if !ch.Started || ch.Notify != model.StatusNone {
		t.Errorf("change = %+v", ch)
	}
}

func TestPromptMakesWorkingAndKeepsPrompt(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "start", Pane: "%1", SessionID: "s1"}, notFocused)
	apply(tb, Event{Agent: "claude", Name: "prompt", Pane: "%1", SessionID: "s1", Prompt: "add pagination"}, notFocused)
	a := tb.Get("%1")
	if a.State != model.Working || a.Prompt != "add pagination" || a.Activity != "thinking" {
		t.Fatalf("agent = %+v", a)
	}
}

func TestPermissionThenNotificationNotifiesOnce(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "prompt", Pane: "%1", SessionID: "s1", Prompt: "x"}, notFocused)
	ch1 := apply(tb, Event{Agent: "claude", Name: "input", Pane: "%1", SessionID: "s1", Detail: "rm hello.txt"}, notFocused)
	ch2 := apply(tb, Event{Agent: "claude", Name: "input", Pane: "%1", SessionID: "s1"}, notFocused)
	a := tb.Get("%1")
	if a.State != model.Input || a.Detail != "rm hello.txt" {
		t.Fatalf("agent = %+v", a)
	}
	if ch1.Notify != model.StatusInput || ch2.Notify != model.StatusNone {
		t.Errorf("changes = %+v / %+v", ch1, ch2)
	}
}

func TestToolAfterInputGoesBackToWorking(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "input", Pane: "%1", SessionID: "s1", Detail: "rm x"}, notFocused)
	apply(tb, Event{Agent: "claude", Name: "tool", Pane: "%1", SessionID: "s1", Activity: "Bash rm x"}, notFocused)
	if a := tb.Get("%1"); a.State != model.Working || a.Activity != "Bash rm x" {
		t.Fatalf("agent = %+v", a)
	}
}

func TestStopUnfocusedIsUnseenAndNotifies(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "pi", Name: "prompt", Pane: "%2", SessionID: "p1", Prompt: "x"}, notFocused)
	ch := apply(tb, Event{Agent: "pi", Name: "stop", Pane: "%2", SessionID: "p1", Detail: "All done."}, notFocused)
	a := tb.Get("%2")
	if a.State != model.Idle || !a.Unseen || a.Detail != "All done." {
		t.Fatalf("agent = %+v", a)
	}
	if ch.Notify != model.StatusDone {
		t.Errorf("change = %+v", ch)
	}
}

func TestStopWhileFocusedIsSeenAndSilent(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "prompt", Pane: "%1", SessionID: "s1"}, focused)
	ch := apply(tb, Event{Agent: "claude", Name: "stop", Pane: "%1", SessionID: "s1"}, focused)
	if a := tb.Get("%1"); a.Unseen {
		t.Fatal("focused agent should not be unseen")
	}
	if ch.Notify != model.StatusNone {
		t.Errorf("change = %+v", ch)
	}
}

func TestInputWhileFocusedStillNotifies(t *testing.T) {
	// a permission prompt blocks the agent; flock still raises urgency/notification
	// so the state is consistent, the daemon decides whether to show a popup.
	tb := New()
	ch := apply(tb, Event{Agent: "claude", Name: "input", Pane: "%1", SessionID: "s1"}, focused)
	if ch.Notify != model.StatusInput {
		t.Errorf("change = %+v", ch)
	}
}

func TestSeenClearsUnseen(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "stop", Pane: "%1", SessionID: "s1"}, notFocused)
	if !tb.Seen("%1") {
		t.Fatal("Seen should report a change")
	}
	if tb.Get("%1").Unseen || tb.Seen("%1") {
		t.Fatal("second Seen should be a no-op")
	}
}

func TestEndRemovesAgent(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "start", Pane: "%1", SessionID: "s1"}, notFocused)
	ch := apply(tb, Event{Agent: "claude", Name: "end", Pane: "%1", SessionID: "s1"}, notFocused)
	if tb.Get("%1") != nil || !ch.Ended {
		t.Fatalf("change = %+v", ch)
	}
}

func TestEventWithoutStartCreatesAgent(t *testing.T) {
	tb := New()
	ch := apply(tb, Event{Agent: "claude", Name: "prompt", Pane: "%1", SessionID: "s1", Prompt: "hi"}, notFocused)
	if tb.Get("%1") == nil || !ch.Started {
		t.Fatalf("change = %+v", ch)
	}
}

func TestNewSessionInSamePaneReplacesAgent(t *testing.T) {
	tb := New()
	apply(tb, Event{Agent: "claude", Name: "start", Pane: "%1", SessionID: "s1"}, notFocused)
	tb.Get("%1").Label = "old"
	ch := apply(tb, Event{Agent: "claude", Name: "start", Pane: "%1", SessionID: "s2", Source: "clear"}, notFocused)
	a := tb.Get("%1")
	if a.SessionID != "s2" || a.Label != "" || !ch.Started {
		t.Fatalf("agent = %+v change = %+v", a, ch)
	}
}

func TestResumeKeepsSessionAndSince(t *testing.T) {
	tb := New()
	ch := apply(tb, Event{Agent: "claude", Name: "start", Pane: "%3", SessionID: "s1", Source: "resume"}, notFocused)
	if !ch.Started || !ch.Resumed {
		t.Fatalf("change = %+v", ch)
	}
}

func TestEmptyPaneIsIgnored(t *testing.T) {
	tb := New()
	ch := apply(tb, Event{Agent: "claude", Name: "start", SessionID: "s1"}, notFocused)
	if ch.Started || len(tb.Panes()) != 0 {
		t.Fatal("events without a pane cannot be placed and must be ignored")
	}
}
