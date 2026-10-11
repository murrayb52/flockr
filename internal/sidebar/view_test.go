package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/murrayb52/flock/internal/model"
)

var now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func snapshot() model.Snapshot {
	api := model.Window{ConID: 11, App: "term", Slot: 1, Focused: true, Visible: true, Term: &model.Term{Cwd: "/w/api", Command: "claude",
		Agent: &model.Agent{Kind: "claude", State: model.Working, Label: "reading-list API", Activity: "Update lists.ts", Since: now.Add(-42 * time.Second),
			Notes: []model.Note{{Text: "a", Done: true}, {Text: "b"}, {Text: "c"}}}}}
	mob := model.Window{ConID: 12, App: "term", Slot: 2, Term: &model.Term{Cwd: "/w/app", Command: "node",
		Agent: &model.Agent{Kind: "pi", State: model.Input, Prompt: "add offline caching to the book detail screen", Since: now.Add(-6 * time.Second)}}}
	ff := model.Window{ConID: 13, App: "firefox", Class: "firefox", Title: "Bookshelf — Mozilla Firefox", Visible: true}
	return model.Snapshot{Workspaces: []model.Workspace{
		{Num: 2, Name: "Other"},
		{Num: 1, Name: "Bookshelf", Focused: true, Groups: []model.Group{
			{Label: "left · stack of 2", Windows: []model.Window{api, mob}},
			{Label: "right", Windows: []model.Window{ff}},
		}},
	}}
}

func plain(lines []Line) []string {
	var out []string
	for _, l := range lines {
		out = append(out, ansi.Strip(l.Text))
	}
	return out
}

func TestExpanded(t *testing.T) {
	lines := Render(snapshot(), Frame{Width: 36, Now: now})
	text := strings.Join(plain(lines), "\n")
	for _, want := range []string{"LEFT · STACK OF 2", " 1  ✻  reading-list API", "[1/3] ◉", "Update lists.ts · 42s",
		" 2  π  add offline caching", "needs input · 6s", "RIGHT", "Firefox", "Bookshelf", "needs you"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Other") {
		t.Error("only the focused workspace is listed (and no workspace heading)")
	}
	for i, l := range plain(lines) {
		if w := ansi.StringWidth(l); w > 36 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
	var con []int64
	for _, l := range lines {
		if l.ConID != 0 && (len(con) == 0 || con[len(con)-1] != l.ConID) {
			con = append(con, l.ConID)
		}
	}
	if len(con) != 3 || con[0] != 11 || con[2] != 13 {
		t.Errorf("click targets = %v", con)
	}
}

func TestRail(t *testing.T) {
	lines := plain(Render(snapshot(), Frame{Width: 8, Now: now}))
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "▌ ✻ ") || !strings.Contains(text, " π !") || !strings.Contains(text, "─") {
		t.Errorf("rail:\n%s", text)
	}
	s := snapshot()
	s.Condensed = true
	if got := plain(Render(s, Frame{Width: 40, Now: now})); strings.Contains(strings.Join(got, ""), "reading-list") {
		t.Error("condensed snapshot draws the rail even before the window narrows")
	}
}

func TestFooterFontButtons(t *testing.T) {
	s := snapshot()
	s.Font = 9.5
	lines := Render(s, Frame{Width: 36, Height: 30, Now: now})
	if len(lines) != 30 {
		t.Fatalf("%d lines, want the full height", len(lines))
	}
	foot := lines[29]
	if got := ansi.Strip(foot.Text); got != " text  −  9.5  + " {
		t.Errorf("footer = %q", got)
	}
	if a := foot.At(7); strings.Join(a, " ") != "sidebar font -1" {
		t.Errorf("click on − = %v", a)
	}
	if a := foot.At(15); strings.Join(a, " ") != "sidebar font +1" {
		t.Errorf("click on + = %v", a)
	}
	if a := foot.At(11); a != nil {
		t.Errorf("the size itself is not a button: %v", a)
	}
	// too short for everything: the footer still gets the last row
	lines = Render(s, Frame{Width: 36, Height: 4, Now: now})
	if len(lines) != 4 || lines[3].At(7) == nil {
		t.Errorf("short sidebar: %v", plain(lines))
	}
	rail := Render(s, Frame{Width: 8, Height: 20, Now: now})
	if f := rail[19]; ansi.Strip(f.Text) != " − +" || f.At(1) == nil || f.At(3) == nil || f.At(2) != nil {
		t.Errorf("rail footer = %q", ansi.Strip(f.Text))
	}
}

func TestEmptyAndMissing(t *testing.T) {
	s := model.Snapshot{Workspaces: []model.Workspace{{Num: 3, Focused: true}}}
	if got := strings.Join(plain(Render(s, Frame{Width: 30})), ""); !strings.Contains(got, "empty workspace") {
		t.Errorf("got %q", got)
	}
	if got := strings.Join(plain(Render(model.Snapshot{}, Frame{Width: 30})), ""); !strings.Contains(got, "waiting") {
		t.Errorf("got %q", got)
	}
}
