package ui

import (
	"testing"
	"time"

	"github.com/murrayb52/flockr/internal/model"
)

func term(label, cwd, cmd string, a *model.Agent) model.Window {
	return model.Window{App: "term", Term: &model.Term{Label: label, Cwd: cwd, Command: cmd, Agent: a}}
}

func TestName(t *testing.T) {
	cases := []struct {
		w    model.Window
		want string
		auto bool
	}{
		{term("", "/home/u/code/api", "zsh", &model.Agent{Kind: "claude", Label: "reading-list API", Prompt: "add pagination"}), "reading-list API", false},
		{term("", "/home/u/code/api", "zsh", &model.Agent{Kind: "claude", Prompt: "add pagination"}), "add pagination", true},
		{term("", "/home/u/code/api", "claude", &model.Agent{Kind: "claude"}), "Claude Code — api", true},
		{term("dev server", "/x/app", "zsh", nil), "dev server", false},
		{term("", "/x/app", "zsh", nil), "zsh — app", true},
		{model.Window{App: "firefox", Class: "firefox", Title: "x"}, "Firefox", false},
		{model.Window{App: "music", Class: "tidal-hifi"}, "Tidal", false},
		{model.Window{App: "gimp", Class: "Gimp"}, "Gimp", false},
	}
	for _, c := range cases {
		got, auto := Name(c.w)
		if got != c.want || auto != c.auto {
			t.Errorf("got (%q,%v) want (%q,%v)", got, auto, c.want, c.auto)
		}
	}
}

func TestStatus(t *testing.T) {
	if StatusOf(nil) != model.StatusNone {
		t.Error("no agent → none")
	}
	if StatusOf(&model.Agent{State: model.Idle, Unseen: true}) != model.StatusDone {
		t.Error("unseen idle → done")
	}
	ws := model.Workspace{Groups: []model.Group{{Windows: []model.Window{
		term("", "/", "", &model.Agent{State: model.Working}),
		term("", "/", "", &model.Agent{State: model.Idle, Unseen: true}),
		term("", "/", "", &model.Agent{State: model.Input}),
	}}}}
	if WorkspaceStatus(ws) != model.StatusInput {
		t.Error("input wins")
	}
	if WorkspaceStatus(model.Workspace{}) != model.StatusNone {
		t.Error("no agents → none")
	}
}

func TestElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{5 * time.Second: "5s", 3 * time.Minute: "3m", 2 * time.Hour: "2h", -time.Second: "0s"} {
		if got := Elapsed(d); got != want {
			t.Errorf("%v → %q", d, got)
		}
	}
}

func TestSub(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	a := &model.Agent{Kind: "pi", State: model.Input, Since: now.Add(-6 * time.Second)}
	if got := Sub(term("", "/x", "pi", a), now, "/home/u"); got != "needs input · 6s" {
		t.Errorf("got %q", got)
	}
	if got := Sub(term("", "/home/u/code", "zsh", nil), now, "/home/u"); got != "zsh · ~/code" {
		t.Errorf("got %q", got)
	}
	if got := Sub(model.Window{App: "firefox", Title: "Bookshelf — Mozilla Firefox"}, now, "/home/u"); got != "Bookshelf" {
		t.Errorf("got %q", got)
	}
}

func TestTiles(t *testing.T) {
	if Tile(term("", "/", "zsh", &model.Agent{Kind: "claude"})).Glyph != "✻" {
		t.Error("claude tile")
	}
	if Tile(term("", "/", "zsh", &model.Agent{Kind: "pi"})).Glyph != "π" {
		t.Error("pi tile")
	}
	if Tile(term("", "/", "zsh", nil)).Glyph != "›_" {
		t.Error("shell tile")
	}
	if Tile(term("", "/", "claude", nil)).Glyph != "✻" {
		t.Error("a running claude gets its icon before any hook arrives")
	}
	if n, auto := Name(term("", "/x/api", "claude", nil)); n != "Claude Code — api" || !auto {
		t.Errorf("name = %q", n)
	}
	if Tile(model.Window{App: "gimp", Class: "Gimp"}).Glyph != "G" {
		t.Error("fallback tile uses first letter")
	}
}
