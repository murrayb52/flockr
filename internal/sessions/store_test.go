package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/murrayb52/flock/internal/model"
)

func TestRoundTrip(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	rec := Record{Label: `hero "fix" #2 — 100%`, Notes: []model.Note{{Text: "check OG image ✓", Done: true}, {Text: "swap photo"}}}
	if err := s.Save("claude", "fa8e-1", rec); err != nil {
		t.Fatal(err)
	}
	got := s.Load("claude", "fa8e-1")
	if got.Label != rec.Label || len(got.Notes) != 2 || got.Notes[0] != rec.Notes[0] || got.Updated.IsZero() {
		t.Fatalf("got %+v", got)
	}
}

func TestMissingIsEmpty(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if got := s.Load("pi", "nope"); got.Label != "" || got.Notes != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestIDsCannotEscapeTheDirectory(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "sessions")}
	if err := s.Save("claude", "../../etc/passwd", Record{Label: "x"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "sessions"))
	if len(entries) != 1 {
		t.Fatalf("expected one file inside the store, got %v", entries)
	}
	if s.Load("claude", "../../etc/passwd").Label != "x" {
		t.Fatal("sanitised id must round trip")
	}
}

func TestEmptyRecordRemovesFile(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	s.Save("pi", "a", Record{Label: "x"})
	s.Save("pi", "a", Record{})
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 0 {
		t.Fatalf("empty record should not leave a file: %v", entries)
	}
}
