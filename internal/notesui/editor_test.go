package notesui

import (
	"testing"

	"github.com/murrayb52/flock/internal/model"
)

func keys(e *Editor, ks ...string) {
	for _, k := range ks {
		e.Key(k)
	}
}

func typ(e *Editor, s string) {
	for _, r := range s {
		e.Key(string(r))
	}
}

func three() *Editor {
	return NewEditor([]model.Note{{Text: "rate-limit"}, {Text: "cursor format", Done: true}, {Text: "index"}})
}

func TestOpensOnFirstOpenNote(t *testing.T) {
	e := NewEditor([]model.Note{{Text: "a", Done: true}, {Text: "b"}})
	if e.Sel != 1 || e.Editing {
		t.Fatalf("sel=%d editing=%v", e.Sel, e.Editing)
	}
	if NewEditor(nil).Sel != 0 {
		t.Fatal("no notes → the new-note row")
	}
}

func TestSpaceTicksTabMoves(t *testing.T) {
	e := three()
	keys(e, " ")
	if !e.Notes[0].Done {
		t.Fatal("space ticks the selected note")
	}
	keys(e, "tab", "tab", " ")
	if e.Sel != 2 || !e.Notes[2].Done {
		t.Fatalf("sel=%d notes=%+v", e.Sel, e.Notes)
	}
	keys(e, "tab")
	if e.Sel != 3 {
		t.Fatal("tab reaches the new-note row")
	}
	keys(e, "tab")
	if e.Sel != 0 {
		t.Fatal("tab wraps to the first note")
	}
	keys(e, "shift+tab")
	if e.Sel != 3 {
		t.Fatal("shift+tab wraps backwards")
	}
	keys(e, "up", "up")
	if e.Sel != 1 {
		t.Fatalf("up moves back, sel=%d", e.Sel)
	}
}

func TestTypingEditsWithSpaces(t *testing.T) {
	e := three()
	keys(e, "tab", "tab", "tab") // new-note row
	typ(e, "buy milk")
	if !e.Editing || len(e.Notes) != 4 || e.Notes[3].Text != "buy milk" {
		t.Fatalf("notes=%+v editing=%v", e.Notes, e.Editing)
	}
	keys(e, "tab") // finish + next
	if e.Editing || e.Sel != 4 {
		t.Fatalf("sel=%d editing=%v", e.Sel, e.Editing)
	}
}

func TestTypingAppendsToExistingNote(t *testing.T) {
	e := three()
	typ(e, "!")
	keys(e, "backspace", "backspace", "enter")
	if e.Notes[0].Text != "rate-limi" || e.Editing {
		t.Fatalf("note=%q editing=%v", e.Notes[0].Text, e.Editing)
	}
}

func TestEnterEditsAndEscFinishes(t *testing.T) {
	e := three()
	keys(e, "enter")
	if !e.Editing {
		t.Fatal("enter starts editing")
	}
	typ(e, " now")
	if close := e.Key("esc"); close || e.Editing || e.Notes[0].Text != "rate-limit now" {
		t.Fatalf("esc should only finish the edit: %+v", e)
	}
	if close := e.Key("esc"); !close {
		t.Fatal("esc outside editing closes")
	}
}

func TestEmptyEditIsDropped(t *testing.T) {
	e := three()
	keys(e, "tab", "tab", "tab", "enter", "enter")
	if len(e.Notes) != 3 || e.Editing {
		t.Fatalf("an empty new note must vanish: %+v", e.Notes)
	}
	keys(e, "tab", "enter") // from the new-note row, wrap to the first note and edit it
	for range "rate-limit" {
		e.Key("backspace")
	}
	keys(e, "enter")
	if len(e.Notes) != 2 {
		t.Fatalf("clearing a note's text removes it: %+v", e.Notes)
	}
}

func TestDeleteRemoves(t *testing.T) {
	e := three()
	keys(e, "tab", "delete")
	if len(e.Notes) != 2 || e.Notes[1].Text != "index" || e.Sel != 1 {
		t.Fatalf("notes=%+v sel=%d", e.Notes, e.Sel)
	}
	keys(e, "tab", "delete") // on the new-note row: nothing to delete
	if len(e.Notes) != 2 {
		t.Fatal("delete on the new row is a no-op")
	}
}

func TestCleanDropsBlankNotes(t *testing.T) {
	e := NewEditor([]model.Note{{Text: " "}, {Text: "x"}})
	if got := e.Clean(); len(got) != 1 || got[0].Text != "x" {
		t.Fatalf("got %+v", got)
	}
}
