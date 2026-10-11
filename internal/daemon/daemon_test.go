package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murrayb52/flock/internal/agents"
	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/protocol"
	"github.com/murrayb52/flock/internal/sessions"
	"github.com/murrayb52/flock/internal/tmux"
	"go.i3wm.org/i3/v4"
)

// ---- fakes ----

type fakeWM struct {
	mu    sync.Mutex
	root  *i3.Node
	infos []i3.Workspace
	cmds  []string
}

func (f *fakeWM) Tree() (*i3.Node, []i3.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.root, f.infos, nil
}
func (f *fakeWM) Command(c ...string) error {
	f.mu.Lock()
	f.cmds = append(f.cmds, c...)
	f.mu.Unlock()
	return nil
}
func (f *fakeWM) Events(context.Context) <-chan struct{} { return nil }
func (f *fakeWM) took(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.cmds {
		if strings.Contains(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

type fakeMux struct {
	mu      sync.Mutex
	panes   []tmux.Pane
	sets    map[string]map[string]string
	prompts []string
	popups  []string
}

func (f *fakeMux) ListPanes() ([]tmux.Pane, error) { return f.panes, nil }
func (f *fakeMux) Set(s string, o map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sets == nil {
		f.sets = map[string]map[string]string{}
	}
	if f.sets[s] == nil {
		f.sets[s] = map[string]string{}
	}
	for k, v := range o {
		f.sets[s][k] = v
	}
	return nil
}
func (f *fakeMux) Popup(s, title, cmd string, h, ht int) error {
	f.mu.Lock()
	f.popups = append(f.popups, s+" "+cmd)
	f.mu.Unlock()
	return nil
}
func (f *fakeMux) ClosePopup(string) error { return nil }
func (f *fakeMux) Prompt(s, p, initial, tpl string) error {
	f.prompts = append(f.prompts, s+"|"+initial+"|"+tpl)
	return nil
}
func (f *fakeMux) opt(s, k string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sets[s][k]
}

type note struct {
	key, title, body string
	critical         bool
}
type fakeNotifier struct {
	shown  []note
	closed []string
}

func (f *fakeNotifier) Notify(k, t, b string, c bool) error {
	f.shown = append(f.shown, note{k, t, b, c})
	return nil
}
func (f *fakeNotifier) Close(k string)         { f.closed = append(f.closed, k) }
func (f *fakeNotifier) Actions() <-chan string { return nil }

type fakeUrg map[int64]bool

func (f fakeUrg) Set(x int64, on bool) error { f[x] = on; return nil }

// ---- fixture: workspace 1 "Bookshelf" with a stack of two flock terminals + firefox ----

func fixture(t *testing.T) (*Daemon, *fakeWM, *fakeMux, *fakeNotifier, fakeUrg) {
	t.Helper()
	api := &i3.Node{ID: 11, Window: 1100, Name: "claude [flock:s1]", WindowProperties: i3.WindowProperties{Class: "Alacritty"}}
	mob := &i3.Node{ID: 12, Window: 1200, Name: "pi [flock:s2]", WindowProperties: i3.WindowProperties{Class: "Alacritty"}}
	ff := &i3.Node{ID: 13, Window: 1300, Name: "x — Mozilla Firefox", WindowProperties: i3.WindowProperties{Class: "firefox"}, Focused: true}
	stack := &i3.Node{ID: 10, Layout: i3.Stacked, Nodes: []*i3.Node{api, mob}, Focus: []i3.NodeID{11, 12}}
	ws1 := &i3.Node{ID: 2, Type: i3.WorkspaceNode, Name: "1:Bookshelf", Layout: i3.SplitH, Nodes: []*i3.Node{stack, ff}, Focus: []i3.NodeID{13, 10}}
	root := &i3.Node{Type: i3.Root, Nodes: []*i3.Node{{Type: i3.OutputNode, Name: "eDP-1", Rect: i3.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
		Nodes: []*i3.Node{{Name: "content", Nodes: []*i3.Node{ws1}}}}}}
	// i3 reports the workspace rect inside the gap flock adds (x=280 here)
	wm := &fakeWM{root: root, infos: []i3.Workspace{{Num: 1, Name: "1:Bookshelf", Focused: true, Visible: true, Output: "eDP-1",
		Rect: i3.Rect{X: 280, Y: 28, Width: 1640, Height: 1052}}}}
	mux := &fakeMux{panes: []tmux.Pane{
		{Session: "s1", ID: "%1", Cwd: "/home/u/code/api", Command: "claude"},
		{Session: "s2", ID: "%2", Cwd: "/home/u/code/app", Command: "node"},
	}}
	n := &fakeNotifier{}
	u := fakeUrg{}
	cfg := config.Config{DataDir: t.TempDir(), Notifications: true, SidebarWidth: 280, RailWidth: 64, SidebarFont: 11, LabelKey: "Alt+/", NotesKey: "Alt+."}
	d := New(cfg, Deps{WM: wm, Mux: mux, Notifier: n, Urgency: u, Bin: "/usr/bin/flock",
		Now: func() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) }})
	d.Refresh()
	return d, wm, mux, n, u
}

func (f *fakeWM) focus(id i3.NodeID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.root.FindChild(func(n *i3.Node) bool { n.Focused = n.ID == id; return false })
}

func ev(name, pane, sid string) agents.Event {
	return agents.Event{Agent: "claude", Name: name, Pane: pane, SessionID: sid}
}

func agentOf(d *Daemon, pane string) *model.Agent {
	for _, ws := range d.Snapshot().Workspaces {
		for _, w := range ws.Windows() {
			if w.Term != nil && w.Term.Pane == pane {
				return w.Term.Agent
			}
		}
	}
	return nil
}

// ---- tests ----

func TestInputNotifiesAndRaisesUrgency(t *testing.T) {
	d, _, mux, n, u := fixture(t)
	d.HandleEvent(ev("start", "%1", "s-claude"))
	d.HandleEvent(agents.Event{Agent: "claude", Name: "prompt", Pane: "%1", SessionID: "s-claude", Prompt: "add pagination"})
	d.HandleEvent(agents.Event{Agent: "claude", Name: "input", Pane: "%1", SessionID: "s-claude", Detail: "npm test"})

	if len(n.shown) != 1 || !n.shown[0].critical || n.shown[0].title != "add pagination needs your input" ||
		!strings.Contains(n.shown[0].body, "Wants to run: npm test") || !strings.Contains(n.shown[0].body, "Bookshelf") {
		t.Fatalf("notifications = %+v", n.shown)
	}
	if !u[1100] {
		t.Error("urgency hint should be set on the agent's window")
	}
	if h := mux.opt("s1", "@flock_h1"); !strings.Contains(h, "needs input") {
		t.Errorf("header = %q", h)
	}
	if mux.opt("s1", "status") != "2" || !strings.Contains(mux.opt("s1", "@flock_h2"), "notes [0/0]") {
		t.Errorf("notes strip = %q", mux.opt("s1", "@flock_h2"))
	}
	if mux.opt("s1", "@flock_title") != "add pagination" {
		t.Errorf("title = %q", mux.opt("s1", "@flock_title"))
	}
}

func TestFocusClearsDoneAndUrgency(t *testing.T) {
	d, wm, _, n, u := fixture(t)
	d.HandleEvent(ev("start", "%1", "a"))
	d.HandleEvent(ev("input", "%1", "a"))
	d.HandleEvent(ev("stop", "%1", "a"))
	if a := agentOf(d, "%1"); a == nil || !a.Unseen {
		t.Fatalf("agent = %+v", a)
	}
	if u[1100] {
		t.Error("urgency must clear when the agent stops needing input")
	}
	wm.focus(11)
	d.Refresh()
	if a := agentOf(d, "%1"); a.Unseen {
		t.Error("focusing the agent marks it seen")
	}
	if len(n.closed) == 0 || n.closed[len(n.closed)-1] != "%1" {
		t.Errorf("notification should close on focus: %v", n.closed)
	}
}

func TestNoPopupForFocusedInput(t *testing.T) {
	d, wm, _, n, _ := fixture(t)
	wm.focus(11)
	d.Refresh()
	d.HandleEvent(ev("input", "%1", "a"))
	if len(n.shown) != 0 {
		t.Errorf("focused agent needs no notification: %+v", n.shown)
	}
}

func TestNotesFollowTheSession(t *testing.T) {
	d, _, _, _, _ := fixture(t)
	d.HandleEvent(ev("start", "%1", "sess-1"))
	if !d.SetNotes("%1", []model.Note{{Text: "rate-limit"}, {Text: "index", Done: true}}) {
		t.Fatal("SetNotes failed")
	}
	rec := d.store.Load("claude", "sess-1")
	if len(rec.Notes) != 2 {
		t.Fatalf("not persisted: %+v", rec)
	}
	// the session ends and is resumed in the other terminal
	d.HandleEvent(ev("end", "%1", "sess-1"))
	d.HandleEvent(agents.Event{Agent: "claude", Name: "start", Pane: "%2", SessionID: "sess-1", Source: "resume"})
	notes, _, ok := d.Notes("%2")
	if !ok || len(notes) != 2 || !notes[1].Done {
		t.Fatalf("notes after resume = %+v", notes)
	}
}

func TestLabelPromptAnswerIsConsumed(t *testing.T) {
	d, _, mux, _, _ := fixture(t)
	d.HandleEvent(ev("start", "%1", "sess-9"))
	mux.panes[0].LabelInput = `=reading-list "API" #1`
	mux.panes[1].LabelInput = "=dev server"
	d.Refresh()
	if a := agentOf(d, "%1"); a.Label != `reading-list "API" #1` {
		t.Errorf("agent label = %q", a.Label)
	}
	if d.store.Load("claude", "sess-9").Label != `reading-list "API" #1` {
		t.Error("agent label must be saved with the session")
	}
	if mux.opt("s1", "@flock_label_input") != "" || mux.opt("s2", "@flock_label") != "dev server" {
		t.Errorf("tmux options: %+v", mux.sets)
	}
}

func TestLabelCommandUsesNoShell(t *testing.T) {
	d, wm, mux, _, _ := fixture(t)
	wm.focus(11)
	d.Refresh()
	if r := d.Command([]string{"label"}); !r.OK {
		t.Fatal(r.Error)
	}
	if len(mux.prompts) != 1 || !strings.Contains(mux.prompts[0], `set-option -t s1 @flock_label_input "=%%%"`) {
		t.Errorf("prompt = %v", mux.prompts)
	}
	wm.focus(13)
	d.Refresh()
	if r := d.Command([]string{"label"}); r.OK {
		t.Error("labelling a non-flock window should fail politely")
	}
}

func TestNextPrefersInputThenOldestDone(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	d.HandleEvent(ev("stop", "%1", "a"))
	d.HandleEvent(agents.Event{Agent: "pi", Name: "input", Pane: "%2", SessionID: "b"})
	d.Command([]string{"next"})
	if got := wm.took("focus"); len(got) == 0 || got[len(got)-1] != "[con_id=12] focus" {
		t.Errorf("focus commands = %v", got)
	}
}

func TestSlotCommand(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	wm.focus(12)
	d.Refresh()
	d.Command([]string{"slot", "1"})
	if got := wm.took("move up"); len(got) != 1 || got[0] != "[con_id=12] move up" {
		t.Errorf("cmds = %v", got)
	}
	if r := d.Command([]string{"slot", "x"}); r.OK {
		t.Error("bad slot should fail")
	}
}

func TestAgentClearedWhenPaneReturnsToShell(t *testing.T) {
	d, _, mux, _, _ := fixture(t)
	d.HandleEvent(ev("start", "%1", "a"))
	d.Refresh() // records "claude" as the running command
	mux.panes[0].Command = "zsh"
	d.Refresh()
	if agentOf(d, "%1") != nil {
		t.Error("agent should be cleared once its pane is back at a shell")
	}
}

func TestAgentRunByAShellScriptIsKept(t *testing.T) {
	d, _, mux, _, _ := fixture(t)
	mux.panes[0].Command = "bash" // tmux reports a script by its interpreter
	d.HandleEvent(ev("start", "%1", "a"))
	d.Refresh()
	d.Refresh()
	if agentOf(d, "%1") == nil {
		t.Error("an agent whose process looks like a shell from the start must not be cleared")
	}
}

func TestSidebarDockedAndGapMatchesRealWidth(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	side := &i3.Node{ID: 99, Window: 9900, Name: "sidebar", WindowProperties: i3.WindowProperties{Class: "flock-sidebar"},
		Rect: i3.Rect{X: 500, Y: 500, Width: 300, Height: 300}}
	wm.root.Nodes[0].Nodes[0].Nodes[0].FloatingNodes = []*i3.Node{{Type: i3.FloatingCon, Nodes: []*i3.Node{side}}}
	d.Refresh()
	if got := wm.took("con_id=99"); len(got) != 5 || got[2] != "[con_id=99] border none" ||
		got[3] != "[con_id=99] move absolute position 0 px 28 px" || got[4] != "[con_id=99] resize set 280 px 1052 px" {
		t.Fatalf("placement = %v", got)
	}
	side.Rect = i3.Rect{X: 0, Y: 28, Width: 283, Height: 1050} // snapped by the terminal
	d.Refresh()
	if got := wm.took("gaps left"); len(got) != 1 || got[0] != "gaps left all set 283" {
		t.Fatalf("gaps = %v", got)
	}
	wm.infos[0].Rect.X = 283 // i3 applied it
	d.Refresh()
	if got := wm.took("gaps left"); len(got) != 1 {
		t.Fatalf("gap already right, nothing to do: %v", got)
	}
	wm.infos[0].Rect.X = 0 // something reset the gaps behind flock's back
	d.Refresh()
	if got := wm.took("gaps left"); len(got) != 2 {
		t.Fatalf("a reset gap must be restored: %v", got)
	}
	wm.infos[0].Rect.X = 283
	d.Refresh()
	if got := wm.took("con_id=99"); len(got) != 5 {
		t.Errorf("no repeated placement once settled: %v", got)
	}
	d.Command([]string{"sidebar", "toggle"})
	if got := wm.took("resize set 64 px"); len(got) != 1 || !d.Snapshot().Condensed {
		t.Errorf("condense = %v", got)
	}
}

func TestSidebarHiddenBehindFullscreenIsLeftAlone(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	side := &i3.Node{ID: 99, Window: 9900, Name: "sidebar", WindowProperties: i3.WindowProperties{Class: "flock-sidebar"},
		Rect: i3.Rect{X: 597, Y: 295, Width: 800, Height: 600}} // stale: not drawn
	wm.root.Nodes[0].Nodes[0].Nodes[0].FloatingNodes = []*i3.Node{{Type: i3.FloatingCon,
		Rect: i3.Rect{X: 0, Y: 28, Width: 280, Height: 1052}, Nodes: []*i3.Node{side}}}
	for i := 0; i < 3; i++ {
		d.Refresh()
	}
	if got := wm.took("con_id=99"); len(got) != 0 {
		t.Errorf("no commands for a window i3 is not drawing: %v", got)
	}
	if got := wm.took("gaps left"); len(got) != 0 {
		t.Errorf("gap from a stale width: %v", got)
	}
	select {
	case <-d.dirty:
		t.Error("must not ask for an immediate refresh (that loops)")
	default:
	}
}

func TestSidebarFontReopensAtNewSizeKeepingGap(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	var launched []string
	d.d.Launch = func(c string) error { launched = append(launched, c); return nil }
	d.cfg.Terminal = "alacritty"
	side := &i3.Node{ID: 99, Window: 9900, Name: "sidebar", WindowProperties: i3.WindowProperties{Class: "flock-sidebar"},
		Rect: i3.Rect{X: 0, Y: 28, Width: 280, Height: 1052}}
	floats := &wm.root.Nodes[0].Nodes[0].Nodes[0].FloatingNodes
	*floats = []*i3.Node{{Type: i3.FloatingCon, Nodes: []*i3.Node{side}}}
	d.Refresh()
	d.Refresh()
	if d.Snapshot().Font != 11 {
		t.Fatalf("default font = %v", d.Snapshot().Font)
	}
	if r := d.Command([]string{"sidebar", "font", "-1"}); !r.OK {
		t.Fatal(r.Error)
	}
	if got := wm.took("[con_id=99] kill"); len(got) != 1 || d.Snapshot().Font != 10 {
		t.Fatalf("kill = %v, font = %v", got, d.Snapshot().Font)
	}
	*floats = nil // i3 closed it
	d.Refresh()
	d.Refresh()
	if len(launched) != 1 || !strings.Contains(launched[0], "-o font.size=10 ") {
		t.Fatalf("launched = %v", launched)
	}
	if got := wm.took("gaps left all set 0"); len(got) != 0 {
		t.Errorf("the gap must survive the reopen: %v", got)
	}
	side2 := *side
	side2.ID = 100
	side2.Rect = i3.Rect{X: 0, Y: 28, Width: 800, Height: 600} // a fresh terminal's own size
	*floats = []*i3.Node{{Type: i3.FloatingCon, Nodes: []*i3.Node{&side2}}}
	d.Refresh()
	if d.relaunch || d.launched {
		t.Error("the new window has arrived")
	}
	if got := wm.took("[con_id=100] resize set 280 px"); len(got) != 1 {
		t.Errorf("the reopened sidebar must be sized: %v", wm.took("con_id=100"))
	}
	// remembered across daemons, and clamped
	d2 := New(d.cfg, d.d)
	if d2.font != 10 {
		t.Errorf("saved font = %v", d2.font)
	}
	d.Command([]string{"sidebar", "font", "2"})
	if d.Snapshot().Font != minFont {
		t.Errorf("clamped = %v", d.Snapshot().Font)
	}
	for _, bad := range [][]string{{"sidebar", "font"}, {"sidebar", "font", "big"}, {"sidebar", "font", "0"}} {
		if r := d.Command(bad); r.OK {
			t.Errorf("%v must fail", bad)
		}
	}
}

func TestHandleProtocol(t *testing.T) {
	d, _, _, _, _ := fixture(t)
	e := ev("start", "%1", "z")
	if r := d.Handle(protocol.Request{Op: protocol.OpEvent, Event: &e}); !r.OK {
		t.Fatal(r.Error)
	}
	if r := d.Handle(protocol.Request{Op: protocol.OpSetNotes, Pane: "%9"}); r.OK {
		t.Error("notes for an unknown pane must fail")
	}
	if r := d.Handle(protocol.Request{Op: "bogus"}); r.OK {
		t.Error("unknown op must fail")
	}
}

func TestStoreUsesDataDir(t *testing.T) {
	d, _, _, _, _ := fixture(t)
	if !strings.HasSuffix(d.store.Dir, "/sessions") {
		t.Errorf("store dir = %s", d.store.Dir)
	}
	_ = sessions.Record{}
}
