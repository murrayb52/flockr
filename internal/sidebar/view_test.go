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
		" 2  π  add offline caching", "needs input · 6s", "RIGHT", "Firefox", "Bookshelf", "≡ menu"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "needs you") {
		t.Error("the legend lives in the menu")
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

func button(l Line, label string) []string {
	text := ansi.Strip(l.Text)
	i := strings.Index(text, label)
	if i < 0 {
		return nil
	}
	return l.At(ansi.StringWidth(text[:i]))
}

func TestMenu(t *testing.T) {
	s := snapshot()
	s.Font = 9.5
	closed := Render(s, Frame{Width: 30, Height: 30, Now: now})
	if len(closed) != 30 || strings.Join(button(closed[29], "≡ menu"), " ") != ActMenu {
		t.Fatalf("closed menu: %q", ansi.Strip(closed[29].Text))
	}
	open := Render(s, Frame{Width: 30, Height: 30, Now: now, Menu: true})
	if len(open) != 30 {
		t.Fatalf("%d lines", len(open))
	}
	text := strings.Join(plain(open), "\n")
	for _, want := range []string{"text size", " 9.5 ", "condense", "notifications", " on ", "needs you", "idle", "≡ close"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	find := func(label string) []string {
		for _, l := range open {
			if a := button(l, label); a != nil {
				return a
			}
		}
		return nil
	}
	for label, want := range map[string]string{
		" − ": "sidebar font -1", " + ": "sidebar font +1", "condense": "sidebar condense",
		" on ": "notifications toggle", "≡ close": ActMenu,
	} {
		if got := strings.Join(find(label), " "); got != want {
			t.Errorf("%q runs %q, want %q", label, got, want)
		}
	}
	if !strings.Contains(text, "reading-list") {
		t.Error("the windows stay listed above the menu")
	}
	for i, l := range plain(open) {
		if w := ansi.StringWidth(l); w > 30 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
	s.Muted = true
	if !strings.Contains(strings.Join(plain(Render(s, Frame{Width: 30, Height: 30, Menu: true})), ""), " off ") {
		t.Error("muted shows off")
	}
	rail := Render(s, Frame{Width: 8, Height: 20, Now: now})
	if got := strings.Join(button(rail[19], "≡"), " "); got != ActMenuExpand {
		t.Errorf("rail menu button = %q", got)
	}
}

func TestShortSidebarKeepsMenu(t *testing.T) {
	lines := Render(snapshot(), Frame{Width: 30, Height: 4, Now: now})
	if len(lines) != 4 || button(lines[3], "≡ menu") == nil {
		t.Errorf("short sidebar: %q", plain(lines))
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
