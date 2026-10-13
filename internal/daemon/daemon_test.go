package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/murrayb52/flockr/internal/agents"
	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/model"
	"github.com/murrayb52/flockr/internal/protocol"
	"github.com/murrayb52/flockr/internal/sessions"
	"github.com/murrayb52/flockr/internal/tmux"
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

// ---- fixture: workspace 1 "Bookshelf" with a stack of two flockr terminals + firefox ----

func fixture(t *testing.T) (*Daemon, *fakeWM, *fakeMux, *fakeNotifier, fakeUrg) {
	t.Helper()
	api := &i3.Node{ID: 11, Window: 1100, Name: "claude [flockr:s1]", WindowProperties: i3.WindowProperties{Class: "Alacritty"}}
	mob := &i3.Node{ID: 12, Window: 1200, Name: "pi [flockr:s2]", WindowProperties: i3.WindowProperties{Class: "Alacritty"}}
	ff := &i3.Node{ID: 13, Window: 1300, Name: "x — Mozilla Firefox", WindowProperties: i3.WindowProperties{Class: "firefox"}, Focused: true}
	stack := &i3.Node{ID: 10, Layout: i3.Stacked, Nodes: []*i3.Node{api, mob}, Focus: []i3.NodeID{11, 12}}
	ws1 := &i3.Node{ID: 2, Type: i3.WorkspaceNode, Name: "1:Bookshelf", Layout: i3.SplitH, Nodes: []*i3.Node{stack, ff}, Focus: []i3.NodeID{13, 10}}
	root := &i3.Node{Type: i3.Root, Nodes: []*i3.Node{{Type: i3.OutputNode, Name: "eDP-1", Rect: i3.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
		Nodes: []*i3.Node{{Name: "content", Nodes: []*i3.Node{ws1}}}}}}
	// i3 reports the workspace rect inside the gap flockr adds (x=280 here)
	wm := &fakeWM{root: root, infos: []i3.Workspace{{Num: 1, Name: "1:Bookshelf", Focused: true, Visible: true, Output: "eDP-1",
		Rect: i3.Rect{X: 280, Y: 28, Width: 1640, Height: 1052}}}}
	mux := &fakeMux{panes: []tmux.Pane{
		{Session: "s1", ID: "%1", Cwd: "/home/u/code/api", Command: "claude"},
		{Session: "s2", ID: "%2", Cwd: "/home/u/code/app", Command: "node"},
	}}
	n := &fakeNotifier{}
	u := fakeUrg{}
	cfg := config.Config{DataDir: t.TempDir(), Notifications: true, LabelKey: "Alt+/", NotesKey: "Alt+."}
	d := New(cfg, Deps{WM: wm, Mux: mux, Notifier: n, Urgency: u, Bin: "/usr/bin/flockr",
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
	d, wm, mux, n, u := fixture(t)
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
	if mux.opt("s1", "status") != "on" || !strings.Contains(mux.opt("s1", "@flockr_h1"), "notes [0/0]") {
		t.Errorf("notes strip = %q", mux.opt("s1", "@flockr_h1"))
	}
	if mux.opt("s2", "status") != "off" {
		t.Error("a plain shell has no tmux header")
	}
	titles := wm.took("[con_id=11] title_format")
	if len(titles) == 0 || !strings.Contains(titles[len(titles)-1], "</span> 1 <span") ||
		!strings.Contains(titles[len(titles)-1], "add pagination</b>  <span foreground='#E53935'>needs input") || !strings.Contains(titles[len(titles)-1], "code/api</span>") {
		t.Errorf("i3 title = %v", titles)
	}
	if mux.opt("s1", "@flockr_title") != "! add pagination" {
		t.Errorf("title = %q", mux.opt("s1", "@flockr_title"))
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

func TestTerminalsGetATitleBarOnce(t *testing.T) {
	d, wm, _, _, _ := fixture(t) // the fixture's nodes have no border style yet
	d.Refresh()
	d.Refresh()
	for _, con := range []string{"11", "12"} {
		if got := wm.took("[con_id=" + con + "] border normal"); len(got) != 1 {
			t.Errorf("terminal %s: border commands = %v", con, got)
		}
	}
	if got := wm.took("[con_id=13] border"); len(got) != 0 {
		t.Errorf("only flockr terminals: %v", got)
	}
	w := model.Window{ConID: 7, Border: "pixel", BorderWidth: 5}
	if got := borderCmd(w); got != "[con_id=7] border normal 5" {
		t.Errorf("keeps the width: %s", got)
	}
}

func TestFocusByNumber(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	if r := d.Command([]string{"focus", "2"}); !r.OK {
		t.Fatal(r.Error)
	}
	if got := wm.took("focus"); len(got) == 0 || !strings.Contains(got[len(got)-1], "con_id=12") {
		t.Errorf("commands = %v", got)
	}
	if r := d.Command([]string{"focus", "3"}); r.OK {
		t.Error("there is no terminal 3 (firefox is not numbered)")
	}
}

func TestMarkUnread(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	wm.focus(11)
	d.Refresh()
	d.HandleEvent(ev("start", "%1", "a"))
	d.HandleEvent(ev("stop", "%1", "a"))
	if a := agentOf(d, "%1"); a.Unseen {
		t.Fatal("finished in view: already seen")
	}
	if r := d.Command([]string{"unread"}); !r.OK {
		t.Fatal(r.Error)
	}
	d.Refresh()
	if a := agentOf(d, "%1"); !a.Unseen {
		t.Error("unread marks the focused agent unseen, and it stays so while focused")
	}
	wm.focus(12)
	d.Refresh()
	wm.focus(11)
	d.Refresh()
	if a := agentOf(d, "%1"); a.Unseen {
		t.Error("coming back to it marks it seen")
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
	if mux.opt("s1", "@flockr_label_input") != "" || mux.opt("s2", "@flockr_label") != "dev server" {
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
	if len(mux.prompts) != 1 || !strings.Contains(mux.prompts[0], `set-option -t s1 @flockr_label_input "=%%%"`) {
		t.Errorf("prompt = %v", mux.prompts)
	}
	wm.focus(13)
	d.Refresh()
	if r := d.Command([]string{"label"}); r.OK {
		t.Error("labelling a non-flockr window should fail politely")
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

func TestNotificationsSwitchIsSavedAndMutes(t *testing.T) {
	d, _, _, n, _ := fixture(t)
	if r := d.Command([]string{"notifications", "off"}); !r.OK || !d.Snapshot().Muted {
		t.Fatalf("off: %+v muted=%v", r, d.Snapshot().Muted)
	}
	d.HandleEvent(ev("start", "%1", "a"))
	d.HandleEvent(ev("input", "%1", "a"))
	if len(n.shown) != 0 {
		t.Errorf("muted, yet notified: %+v", n.shown)
	}
	if d2 := New(d.cfg, d.d); d2.notifyOn {
		t.Error("the switch must survive a daemon restart")
	}
	d.Command([]string{"notifications", "toggle"})
	if d.Snapshot().Muted {
		t.Error("toggle turns them back on")
	}
	if r := d.Command([]string{"notifications", "loud"}); r.OK {
		t.Error("bad value must fail")
	}
}

func TestFailedKeyCommandsSayWhy(t *testing.T) {
	d, _, _, n, _ := fixture(t) // firefox has focus
	if r := d.Command([]string{"label"}); r.OK {
		t.Fatal("labelling firefox must fail")
	}
	if len(n.shown) != 1 || n.shown[0].key != hintKey || !strings.Contains(n.shown[0].body, "flockr terminals") {
		t.Errorf("hint = %+v", n.shown)
	}
	d.Command([]string{"notifications", "loud"}) // not a key binding: no notification
	if len(n.shown) != 1 {
		t.Errorf("only key commands notify: %+v", n.shown)
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
