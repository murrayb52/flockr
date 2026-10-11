package bar

import (
	"strings"
	"testing"

	"github.com/murrayb52/flockr/internal/model"
)

func snap() model.Snapshot {
	agent := func(s model.AgentState, unseen bool) model.Window {
		return model.Window{Term: &model.Term{Agent: &model.Agent{State: s, Unseen: unseen}}}
	}
	return model.Snapshot{Workspaces: []model.Workspace{
		{Num: 2, Name: "Website", Groups: []model.Group{{Windows: []model.Window{agent(model.Idle, true)}}}},
		{Num: 1, Name: "Bookshelf", Urgent: true, Groups: []model.Group{{Windows: []model.Window{agent(model.Input, false), agent(model.Working, false)}}}},
		{Num: 4, Name: "", Focused: true, Groups: []model.Group{{Windows: []model.Window{agent(model.Working, false)}}}},
		{Num: 3, Name: "Life admin", Groups: []model.Group{{Windows: []model.Window{{App: "firefox"}}}}},
	}}
}

func TestPlain(t *testing.T) {
	got := Format(snap(), Plain, 0)
	want := "1 Bookshelf ! · 2 Website ✓ · 3 Life admin · [4 ⠋]"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestPolybar(t *testing.T) {
	got := Format(snap(), Polybar, 0)
	for _, want := range []string{
		"%{A1:i3-msg workspace number 1:}",
		"%{F#ff9a94} 1 Bookshelf %{F#E53935}!%{F-}",
		"%{B#262b33}%{u#40916c}%{+u}%{F#f3f4f5} 4 %{F#f0b429}⠋%{F-}",
		"%{F#52b788}✓%{F-}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Count(got, "%{A1:") != strings.Count(got, "%{A}") {
		t.Error("unbalanced click actions")
	}
}

func TestPolybarEscapesNames(t *testing.T) {
	s := model.Snapshot{Workspaces: []model.Workspace{{Num: 5, Name: "50% done: %{F#fff}"}}}
	got := Format(s, Polybar, 0)
	if strings.Contains(got, "%{F#fff}") {
		t.Errorf("polybar markup in a name must be neutralised: %s", got)
	}
}
