package daemon

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/murrayb52/flockr/internal/model"
	"github.com/murrayb52/flockr/internal/protocol"
	"github.com/murrayb52/flockr/internal/tmux"
	"github.com/murrayb52/flockr/internal/ui"
)

const notesHeight = 16 // most lines the notes drawer takes

// hintKey is the notification that reports why a key binding did nothing.
const hintKey = "flockr-hint"

// Command runs one of the commands i3 bindings call (`flockr <name> …`). Key
// bindings have nowhere to print, so a failure of one is also shown as a
// desktop notification.
func (d *Daemon) Command(args []string) protocol.Response {
	r := d.command(args)
	if !r.OK && len(args) > 0 && d.d.Notifier != nil {
		switch args[0] {
		case "label", "notes", "next":
			if err := d.d.Notifier.Notify(hintKey, "flockr", r.Error, false); err != nil {
				log.Printf("notify: %v", err)
			}
		}
	}
	return r
}

func (d *Daemon) command(args []string) protocol.Response {
	if len(args) == 0 {
		return fail("no command")
	}
	switch args[0] {
	case "next":
		return d.next()
	case "label":
		return d.label()
	case "wslabel":
		return d.wslabel(args[1:])
	case "notes":
		return d.notes()
	case "notifications":
		v := "toggle"
		if len(args) > 1 {
			v = args[1]
		}
		if err := d.setNotifications(v); err != nil {
			return fail(err.Error())
		}
		return ok("")
	case "refresh":
		d.markDirty()
		return ok("")
	}
	return fail("unknown command " + args[0])
}

func ok(msg string) protocol.Response   { return protocol.Response{OK: true, Message: msg} }
func fail(msg string) protocol.Response { return protocol.Response{Error: msg} }

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

// focusedTerm returns the focused window if it is a flockr terminal.
func (d *Daemon) focusedTerm() (model.Window, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, found := d.focusedWindow()
	return w, found && w.Term != nil
}

func (d *Daemon) label() protocol.Response {
	w, found := d.focusedTerm()
	if !found {
		return fail("labels work in flockr terminals (start one with `flockr term`)")
	}
	current := w.Term.Label
	if a := w.Term.Agent; a != nil {
		current = a.Label
	}
	// The answer goes straight into a tmux option the daemon consumes: no shell
	// ever sees the text. The "=" prefix lets an empty answer clear the label.
	tpl := fmt.Sprintf(`set-option -t %s @flockr_label_input "=%%%%%%"`, w.Term.Session)
	if err := d.d.Mux.Prompt(w.Term.Session, "label › ", current, tpl); err != nil {
		return fail(err.Error())
	}
	return ok("")
}

func (d *Daemon) wslabel(args []string) protocol.Response {
	d.mu.Lock()
	ws, found := d.snap.Current()
	d.mu.Unlock()
	if !found {
		return fail("no focused workspace")
	}
	if len(args) > 0 {
		label := strings.TrimSpace(strings.Join(args, " "))
		d.mu.Lock()
		if label == "" {
			delete(d.wsLabels, ws.Num)
		} else {
			d.wsLabels[ws.Num] = label
		}
		d.mu.Unlock()
		if err := d.saveWSLabels(); err != nil {
			log.Printf("wslabel save: %v", err)
		}
		d.markDirty()
		return ok("")
	}
	// No args: run rofi directly (no shell) in a goroutine so the daemon stays responsive.
	current := d.wsLabels[ws.Num]
	go func() {
		args := []string{"-dmenu", "-l", "0", "-p", "label ›  "}
		if current != "" {
			args = append(args, "-filter", current)
		}
		out, err := exec.Command("rofi", args...).Output()
		if err != nil {
			return // Escape pressed or rofi not found
		}
		label := strings.TrimSpace(string(out))
		d.mu.Lock()
		if label == "" {
			delete(d.wsLabels, ws.Num)
		} else {
			d.wsLabels[ws.Num] = label
		}
		d.mu.Unlock()
		if err := d.saveWSLabels(); err != nil {
			log.Printf("wslabel save: %v", err)
		}
		d.markDirty()
	}()
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
	header := map[string]int{"on": 1, "2": 2}[d.headers[s]["status"]]
	go func() {
		d.d.Mux.Popup(s, title, cmd, header, height)
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
