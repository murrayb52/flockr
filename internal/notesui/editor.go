// Package notesui is the notes drawer: a small checklist editor that runs in a
// tmux popup under an agent's header and saves through the daemon.
package notesui

import (
	"strings"

	"github.com/murrayb52/flockr/internal/model"
)

// Editor is the key model. Rows are the notes plus a final "new note" row.
//
// Select mode: tab/down next, shift+tab/up previous (both wrap), space ticks,
// typing or enter edits, delete removes, esc closes.
// Edit mode: typing (spaces included) edits; enter/esc finish; tab finishes
// and moves to the next row. A note left empty is removed.
type Editor struct {
	Notes   []model.Note
	Sel     int
	Editing bool
}

func NewEditor(notes []model.Note) *Editor {
	e := &Editor{Notes: append([]model.Note(nil), notes...)}
	e.Sel = len(e.Notes)
	for i, n := range e.Notes {
		if !n.Done {
			e.Sel = i
			break
		}
	}
	return e
}

func (e *Editor) rows() int { return len(e.Notes) + 1 }

// Key applies one key (bubbletea key names; printable keys are the character)
// and reports whether the drawer should close.
func (e *Editor) Key(k string) (close bool) {
	if e.Editing {
		e.editKey(k)
		return false
	}
	switch k {
	case "esc":
		return true
	case "tab", "down":
		e.Sel = (e.Sel + 1) % e.rows()
	case "shift+tab", "up":
		e.Sel = (e.Sel - 1 + e.rows()) % e.rows()
	case " ":
		if e.Sel < len(e.Notes) {
			e.Notes[e.Sel].Done = !e.Notes[e.Sel].Done
		}
	case "delete":
		if e.Sel < len(e.Notes) {
			e.Notes = append(e.Notes[:e.Sel], e.Notes[e.Sel+1:]...)
		}
	case "enter":
		e.startEdit()
	case "backspace":
		if e.Sel < len(e.Notes) {
			e.startEdit()
			e.backspace()
		}
	default:
		if printable(k) {
			e.startEdit()
			e.Notes[e.Sel].Text += k
		}
	}
	return false
}

func (e *Editor) editKey(k string) {
	switch k {
	case "enter", "esc":
		e.finish()
	case "tab":
		if !e.finish() {
			e.Sel = (e.Sel + 1) % e.rows()
		}
	case "backspace":
		e.backspace()
	default:
		if printable(k) {
			e.Notes[e.Sel].Text += k
		}
	}
}

func (e *Editor) startEdit() {
	if e.Sel >= len(e.Notes) {
		e.Notes = append(e.Notes, model.Note{})
		e.Sel = len(e.Notes) - 1
	}
	e.Editing = true
}

// finish ends editing; it reports whether the edited note was removed for being empty.
func (e *Editor) finish() bool {
	e.Editing = false
	if e.Sel < len(e.Notes) && strings.TrimSpace(e.Notes[e.Sel].Text) == "" {
		e.Notes = append(e.Notes[:e.Sel], e.Notes[e.Sel+1:]...)
		return true
	}
	return false
}

func (e *Editor) backspace() {
	r := []rune(e.Notes[e.Sel].Text)
	if len(r) > 0 {
		e.Notes[e.Sel].Text = string(r[:len(r)-1])
	}
}

func printable(k string) bool {
	r := []rune(k)
	return len(r) == 1 && r[0] >= ' ' && r[0] != 127
}

// Clean returns the notes without blank entries (what gets saved).
func (e *Editor) Clean() []model.Note {
	out := []model.Note{}
	for _, n := range e.Notes {
		if strings.TrimSpace(n.Text) != "" {
			out = append(out, n)
		}
	}
	return out
}
