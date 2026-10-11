// Package daemon is flock's long-running process. It owns all state: the i3
// layout, the flock terminals on the private tmux server, agent states, labels
// and notes. Everything else (sidebar, bar, notes editor, bindings, hooks) is
// a client on its Unix socket.
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/murrayb52/flock/internal/agents"
	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/i3tree"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/sessions"
	"github.com/murrayb52/flock/internal/tmux"
	"github.com/murrayb52/flock/internal/ui"
	"go.i3wm.org/i3/v4"
)

type Deps struct {
	WM       WM
	Mux      Mux
	Notifier Notifier // may be nil
	Urgency  Urgency  // may be nil
	Launch   Launcher // may be nil
	Bin      string   // absolute path of the flock binary
	Now      func() time.Time
}

type Daemon struct {
	cfg   config.Config
	d     Deps
	store sessions.Store
	home  string
	hints tmux.Hints

	mu            sync.Mutex
	agents        *agents.Table
	panes         map[string]tmux.Pane // by session
	tree          *i3.Node
	infos         []i3.Workspace
	snap          model.Snapshot
	snapJSON      []byte
	subs          map[chan []byte]struct{}
	condensed     bool
	focusedID     int64
	headers       map[string]map[string]string // session → last options pushed
	urgent        map[int64]bool               // xid → urgency hint set
	popups        map[string]bool              // session → notes popup open
	agentCmd      map[string]string            // pane → its foreground command while the agent runs
	gap           int                          // current left gap (px), -1 unknown
	sidebarReq    [2]int64                     // last requested sidebar width/height
	sidebarStyled i3.NodeID                    // sidebar con already given border/sticky
	launched      bool
	placed        string    // last sidebar placement commands sent…
	placedAt      time.Time // …and when
	relaunch      bool      // sidebar closed to reopen at a new font size
	font          float64   // sidebar font size (points)
	notifyOn      bool      // desktop notifications (menu switch)
	dirty         chan struct{}
}

func New(cfg config.Config, deps Deps) *Daemon {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	home, _ := os.UserHomeDir()
	font, notifyOn := loadSettings(cfg)
	return &Daemon{
		font: font, notifyOn: notifyOn,
		cfg: cfg, d: deps, home: home,
		store:    sessions.Store{Dir: filepath.Join(cfg.DataDir, "sessions")},
		hints:    tmux.Hints{Label: cfg.LabelKey, Notes: cfg.NotesKey},
		agents:   agents.New(),
		panes:    map[string]tmux.Pane{},
		subs:     map[chan []byte]struct{}{},
		headers:  map[string]map[string]string{},
		urgent:   map[int64]bool{},
		popups:   map[string]bool{},
		agentCmd: map[string]string{},
		gap:      -1,
		dirty:    make(chan struct{}, 1),
	}
}

// markDirty asks the run loop to refresh soon (coalesced).
func (d *Daemon) markDirty() {
	select {
	case d.dirty <- struct{}{}:
	default:
	}
}

// Run refreshes on i3 events, agent events and a poll of tmux until ctx ends.
func (d *Daemon) Run(ctx context.Context) {
	events := d.d.WM.Events(ctx)
	var actions <-chan string
	if d.d.Notifier != nil {
		actions = d.d.Notifier.Actions()
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	lastOK := d.d.Now()
	refresh := func() bool {
		if d.Refresh() {
			lastOK = d.d.Now()
		}
		// i3 gone for good (logout, X server ended): exit rather than linger.
		// An i3 restart takes well under this.
		return d.d.Now().Sub(lastOK) < 15*time.Second
	}
	refresh()
	d.ensureSidebar()
	for {
		alive := true
		select {
		case <-ctx.Done():
			// give the margin back, but don't wait on an i3 that is already gone
			done := make(chan struct{})
			go func() { d.d.WM.Command("gaps left all set 0"); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		case <-events:
			// let a burst of i3 events settle into one refresh
			time.Sleep(15 * time.Millisecond)
			alive = refresh()
		case <-d.dirty:
			alive = refresh()
		case <-tick.C:
			alive = refresh()
		case key := <-actions:
			d.focusPane(key)
		}
		if !alive {
			log.Printf("i3 is unreachable; exiting")
			return
		}
	}
}

// Refresh rebuilds the snapshot from i3 + tmux and applies side effects:
// tmux headers, urgency hints, sidebar placement and subscriber broadcast.
// It reports whether i3 answered.
func (d *Daemon) Refresh() bool {
	tree, infos, err := d.d.WM.Tree()
	panes, perr := d.d.Mux.ListPanes()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err == nil {
		d.tree, d.infos = tree, infos
	} else {
		log.Printf("i3: %v", err)
	}
	if perr == nil || perr == tmux.ErrNoServer {
		d.syncPanes(panes)
	} else {
		log.Printf("tmux: %v", perr)
	}
	d.rebuild()
	return err == nil
}

// syncPanes records the tmux sessions, consumes label prompt answers and drops
// agents whose pane disappeared or went back to a shell.
func (d *Daemon) syncPanes(panes []tmux.Pane) {
	d.panes = map[string]tmux.Pane{}
	byID := map[string]tmux.Pane{}
	for _, p := range panes {
		d.panes[p.Session] = p
		byID[p.ID] = p
		if strings.HasPrefix(p.LabelInput, "=") {
			d.applyLabel(p, strings.TrimSpace(p.LabelInput[1:]))
		}
	}
	for _, pane := range d.agents.Panes() {
		p, ok := byID[pane]
		if !ok {
			d.endAgent(pane)
			continue
		}
		// An agent that died without saying so leaves its pane back at a shell.
		// Compare with the command seen while it ran, so agents that are
		// themselves started through a shell script are not mistaken for dead.
		started, known := d.agentCmd[pane]
		if !known {
			d.agentCmd[pane] = p.Command
		} else if p.Command != started && isShell(p.Command) {
			d.endAgent(pane)
		}
	}
	for s := range d.headers {
		if _, ok := d.panes[s]; !ok {
			delete(d.headers, s)
		}
	}
}

func isShell(cmd string) bool {
	switch cmd {
	case "zsh", "bash", "fish", "sh", "dash", "ksh", "nu", "elvish", "xonsh", "tcsh":
		return true
	}
	return false
}

func (d *Daemon) applyLabel(p tmux.Pane, label string) {
	opts := map[string]string{"@flock_label_input": ""}
	if a := d.agents.Get(p.ID); a != nil {
		a.Label = label
		d.saveRecord(a)
	} else {
		opts["@flock_label"] = label
		p.Label = label
		d.panes[p.Session] = p
	}
	if err := d.d.Mux.Set(p.Session, opts); err != nil {
		log.Printf("tmux label: %v", err)
	}
}

func (d *Daemon) saveRecord(a *model.Agent) {
	if a.SessionID == "" {
		return
	}
	if err := d.store.Save(a.Kind, a.SessionID, sessions.Record{Label: a.Label, Notes: a.Notes}); err != nil {
		log.Printf("saving session %s: %v", a.SessionID, err)
	}
}

func (d *Daemon) endAgent(pane string) {
	delete(d.agentCmd, pane)
	if !d.agents.Clear(pane) {
		return
	}
	if d.d.Notifier != nil {
		d.d.Notifier.Close(pane)
	}
}

// terms builds the flock terminals keyed by tmux session.
func (d *Daemon) terms() map[string]*model.Term {
	out := map[string]*model.Term{}
	for s, p := range d.panes {
		t := &model.Term{Session: s, Pane: p.ID, Cwd: p.Cwd, Command: p.Command, Label: p.Label}
		if a := d.agents.Get(p.ID); a != nil {
			cp := *a
			cp.Notes = append([]model.Note(nil), a.Notes...)
			t.Agent = &cp
		}
		out[s] = t
	}
	return out
}

func (d *Daemon) build() model.Snapshot {
	return model.Snapshot{Workspaces: i3tree.Build(d.tree, d.infos, d.terms()), Condensed: d.condensed, Font: d.font, Muted: !d.notifyOn}
}

func (d *Daemon) rebuild() {
	if d.tree == nil {
		return
	}
	d.snap = d.build()
	d.trackFocus() // may rebuild d.snap after marking an agent seen
	d.pushHeaders()
	d.syncUrgency()
	d.placeSidebar()
	snap := d.snap
	snap.Now = d.d.Now()
	b, _ := json.Marshal(snap)
	snap.Now = time.Time{}
	cmp, _ := json.Marshal(snap)
	if bytes.Equal(cmp, d.snapJSON) {
		return
	}
	d.snapJSON = cmp
	for ch := range d.subs {
		select {
		case ch <- b:
		default: // slow subscriber: it will catch up on the next change
		}
	}
}

// windows returns every window across workspaces.
func (d *Daemon) windows() []model.Window {
	var out []model.Window
	for _, ws := range d.snap.Workspaces {
		out = append(out, ws.Windows()...)
	}
	return out
}

func (d *Daemon) focusedWindow() (model.Window, bool) {
	for _, w := range d.windows() {
		if w.Focused {
			return w, true
		}
	}
	return model.Window{}, false
}

// trackFocus marks a newly focused agent as seen and dismisses its notification.
func (d *Daemon) trackFocus() {
	w, ok := d.focusedWindow()
	if !ok || w.ConID == d.focusedID {
		return
	}
	d.focusedID = w.ConID
	if w.Term == nil || w.Term.Agent == nil {
		return
	}
	pane := w.Term.Pane
	if d.agents.Seen(pane) {
		w.Term.Agent.Unseen = false
		d.snap = d.build()
	}
	if d.d.Notifier != nil {
		d.d.Notifier.Close(pane)
	}
}

func (d *Daemon) paneFocused(pane string) bool {
	w, ok := d.focusedWindow()
	return ok && w.Term != nil && w.Term.Pane == pane
}

func (d *Daemon) pushHeaders() {
	now := d.d.Now()
	for _, w := range d.windows() {
		if w.Term == nil {
			continue
		}
		h1, h2, lines := tmux.Header(w, now, d.home, d.hints)
		name, _ := ui.Name(w)
		opts := map[string]string{"@flock_h1": h1, "@flock_h2": h2, "status": map[int]string{1: "on", 2: "2"}[lines], "@flock_title": name}
		last := d.headers[w.Term.Session]
		changed := map[string]string{}
		for k, v := range opts {
			if last[k] != v {
				changed[k] = v
			}
		}
		if len(changed) == 0 {
			continue
		}
		if err := d.d.Mux.Set(w.Term.Session, changed); err != nil {
			log.Printf("tmux header: %v", err)
			continue
		}
		d.headers[w.Term.Session] = opts
	}
}

// syncUrgency keeps the X urgency hint set exactly on unfocused agents that
// need input (i3 then paints their title strip and workspace red).
func (d *Daemon) syncUrgency() {
	if d.d.Urgency == nil {
		return
	}
	want := map[int64]bool{}
	for _, w := range d.windows() {
		if a := ui.AgentOf(w); a != nil && a.State == model.Input && !w.Focused {
			want[w.XID] = true
		}
	}
	for xid := range d.urgent {
		if !want[xid] {
			d.d.Urgency.Set(xid, false)
			delete(d.urgent, xid)
		}
	}
	for xid := range want {
		if !d.urgent[xid] {
			if err := d.d.Urgency.Set(xid, true); err == nil {
				d.urgent[xid] = true
			}
		}
	}
}

// HandleEvent applies an agent event (from a hook or the Pi extension).
func (d *Daemon) HandleEvent(ev agents.Event) {
	d.mu.Lock()
	ch := d.agents.Apply(ev, d.d.Now(), d.paneFocused)
	a := d.agents.Get(ev.Pane)
	if ch.Started {
		delete(d.agentCmd, ev.Pane)
	}
	if ch.Ended {
		delete(d.agentCmd, ev.Pane)
	}
	if ch.Started && a != nil {
		rec := d.store.Load(a.Kind, a.SessionID)
		a.Label, a.Notes = rec.Label, rec.Notes
	}
	if ch.Ended && d.d.Notifier != nil {
		d.d.Notifier.Close(ev.Pane)
	}
	if ch.Notify != model.StatusNone && a != nil && d.notifyOn && d.d.Notifier != nil &&
		!(ch.Notify == model.StatusInput && d.paneFocused(ev.Pane)) {
		d.notify(ev.Pane, a, ch.Notify)
	}
	d.rebuild()
	d.mu.Unlock()
}

func (d *Daemon) notify(pane string, a *model.Agent, s model.Status) {
	name := a.Label
	if name == "" {
		name = a.Prompt
	}
	if name == "" {
		name = ui.KindName(a.Kind)
	}
	name = trim(name, 60)
	where := d.workspaceOfPane(pane)
	title, body := name+" is done", a.Detail
	if s == model.StatusInput {
		title = name + " needs your input"
		if a.Detail != "" && a.Kind == "claude" {
			body = "Wants to run: " + a.Detail
		}
	}
	if where != "" {
		body = strings.TrimSpace(body + "\n" + where)
	}
	if err := d.d.Notifier.Notify(pane, title, body, s == model.StatusInput); err != nil {
		log.Printf("notify: %v", err)
	}
}

func (d *Daemon) workspaceOfPane(pane string) string {
	for _, ws := range d.snap.Workspaces {
		for _, w := range ws.Windows() {
			if w.Term != nil && w.Term.Pane == pane {
				if ws.Name != "" {
					return ws.Name
				}
				return "workspace " + itoa(ws.Num)
			}
		}
	}
	return ""
}

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// focusPane focuses the window hosting pane (notification default action).
func (d *Daemon) focusPane(pane string) {
	d.mu.Lock()
	var con int64
	for _, w := range d.windows() {
		if w.Term != nil && w.Term.Pane == pane {
			con = w.ConID
		}
	}
	d.mu.Unlock()
	if con != 0 {
		d.d.WM.Command(focusCmd(con))
		d.markDirty()
	}
}

func focusCmd(con int64) string { return "[con_id=" + itoa(con) + "] focus" }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Notes returns a copy of the notes of the agent in pane.
func (d *Daemon) Notes(pane string) ([]model.Note, string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a := d.agents.Get(pane)
	if a == nil {
		return nil, "", false
	}
	name := a.Label
	if name == "" {
		name = a.Prompt
	}
	return append([]model.Note(nil), a.Notes...), name, true
}

// SetNotes replaces and persists the notes of the agent in pane.
func (d *Daemon) SetNotes(pane string, notes []model.Note) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	a := d.agents.Get(pane)
	if a == nil {
		return false
	}
	a.Notes = append([]model.Note(nil), notes...)
	d.saveRecord(a)
	d.rebuild()
	return true
}

// Snapshot returns the current snapshot (for status and tests).
func (d *Daemon) Snapshot() model.Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snap
}

func (d *Daemon) subscribe() (chan []byte, []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ch := make(chan []byte, 8)
	d.subs[ch] = struct{}{}
	snap := d.snap
	snap.Now = d.d.Now()
	b, _ := json.Marshal(snap)
	return ch, b
}

func (d *Daemon) unsubscribe(ch chan []byte) {
	d.mu.Lock()
	delete(d.subs, ch)
	d.mu.Unlock()
}

// attention lists agents that need the user: input first, then unseen done,
// oldest first within each.
func (d *Daemon) attention() []model.Window {
	var out []model.Window
	for _, w := range d.windows() {
		if s := ui.StatusOf(ui.AgentOf(w)); s == model.StatusInput || s == model.StatusDone {
			out = append(out, w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := ui.AgentOf(out[i]), ui.AgentOf(out[j])
		pi, pj := ai.State == model.Input, aj.State == model.Input
		if pi != pj {
			return pi
		}
		return ai.Since.Before(aj.Since)
	})
	return out
}
