package daemon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/murrayb52/flock/internal/i3tree"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/protocol"
	"github.com/murrayb52/flock/internal/tmux"
	"github.com/murrayb52/flock/internal/ui"
	"go.i3wm.org/i3/v4"
)

const notesHeight = 16 // most lines the notes drawer takes

// Command runs one of the commands i3 bindings call (`flock <name> …`).
func (d *Daemon) Command(args []string) protocol.Response {
	if len(args) == 0 {
		return fail("no command")
	}
	switch args[0] {
	case "slot":
		if len(args) < 2 {
			return fail("usage: slot N")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 {
			return fail("slot must be a number from 1")
		}
		return d.slot(n)
	case "next":
		return d.next()
	case "jump":
		if len(args) < 2 {
			return fail("usage: jump CON_ID")
		}
		con, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fail("bad con id")
		}
		d.d.WM.Command(focusCmd(con))
		d.markDirty()
		return ok("")
	case "label":
		return d.label()
	case "notes":
		return d.notes()
	case "sidebar":
		sub := "toggle"
		if len(args) > 1 {
			sub = args[1]
		}
		return d.sidebar(sub)
	case "refresh":
		d.markDirty()
		return ok("")
	}
	return fail("unknown command " + args[0])
}

func ok(msg string) protocol.Response   { return protocol.Response{OK: true, Message: msg} }
func fail(msg string) protocol.Response { return protocol.Response{Error: msg} }

func (d *Daemon) focusedNode() *i3.Node {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tree == nil {
		return nil
	}
	return d.tree.FindChild(func(n *i3.Node) bool { return n.Focused && n.Window != 0 })
}

func (d *Daemon) slot(n int) protocol.Response {
	node := d.focusedNode()
	if node == nil {
		return fail("no focused window")
	}
	d.mu.Lock()
	cmds := i3tree.PlanSlot(d.tree, node.ID, n)
	d.mu.Unlock()
	if len(cmds) == 0 {
		return ok("")
	}
	if err := d.d.WM.Command(cmds...); err != nil {
		return fail(err.Error())
	}
	d.markDirty()
	return ok("")
}

func (d *Daemon) next() protocol.Response {
	d.mu.Lock()
	var target int64
	for _, w := range d.attention() {
		if !w.Focused {
			target = w.ConID
			break
		}
	}
	d.mu.Unlock()
	if target == 0 {
		return ok("no agent needs you")
	}
	d.d.WM.Command(focusCmd(target))
	d.markDirty()
	return ok("")
}

// focusedTerm returns the focused window if it is a flock terminal.
func (d *Daemon) focusedTerm() (model.Window, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, found := d.focusedWindow()
	return w, found && w.Term != nil
}

func (d *Daemon) label() protocol.Response {
	w, found := d.focusedTerm()
	if !found {
		return fail("labels work in flock terminals (start one with `flock term`)")
	}
	current := w.Term.Label
	if a := w.Term.Agent; a != nil {
		current = a.Label
	}
	// The answer goes straight into a tmux option the daemon consumes: no shell
	// ever sees the text. The "=" prefix lets an empty answer clear the label.
	tpl := fmt.Sprintf(`set-option -t %s @flock_label_input "=%%%%%%"`, w.Term.Session)
	if err := d.d.Mux.Prompt(w.Term.Session, "label › ", current, tpl); err != nil {
		return fail(err.Error())
	}
	return ok("")
}

func (d *Daemon) notes() protocol.Response {
	w, found := d.focusedTerm()
	if !found || w.Term.Agent == nil {
		return fail("notes belong to an agent session: focus a terminal running Claude Code or Pi")
	}
	s := w.Term.Session
	d.mu.Lock()
	open := d.popups[s]
	if !open {
		d.popups[s] = true
	}
	d.mu.Unlock()
	if open {
		d.d.Mux.ClosePopup(s)
		return ok("")
	}
	name, _ := ui.Name(w)
	title := " Notes · " + tmux.Escape(trim(name, 50)) + " "
	cmd := shellQuote(d.d.Bin) + " notes-ui --pane " + shellQuote(w.Term.Pane)
	// fit the drawer to its notes: rows + "new note" + blank + help, capped
	height := len(w.Term.Agent.Notes) + 4
	if height < 6 {
		height = 6
	}
	if height > notesHeight {
		height = notesHeight
	}
	go func() {
		d.d.Mux.Popup(s, title, cmd, 2, height)
		d.mu.Lock()
		delete(d.popups, s)
		d.mu.Unlock()
		d.markDirty()
	}()
	return ok("")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (d *Daemon) sidebar(sub string) protocol.Response {
	d.mu.Lock()
	switch sub {
	case "toggle":
		d.condensed = !d.condensed
	case "condense":
		d.condensed = true
	case "expand":
		d.condensed = false
	case "show":
		d.launched = false
	default:
		d.mu.Unlock()
		return fail("usage: sidebar toggle|condense|expand|show")
	}
	d.rebuild()
	d.mu.Unlock()
	d.ensureSidebar()
	return ok("")
}
