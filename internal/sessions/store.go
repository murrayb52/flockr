// Package sessions persists labels and notes per agent session id, so they come
// back when a session is resumed in any terminal.
package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murrayb52/flockr/internal/model"
)

type Record struct {
	Label   string       `json:"label,omitempty"`
	Notes   []model.Note `json:"notes,omitempty"`
	Updated time.Time    `json:"updated"`
}

type Store struct{ Dir string }

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

func (s Store) path(kind, id string) string {
	return filepath.Join(s.Dir, safe(kind)+"-"+safe(id)+".json")
}

func (s Store) Load(kind, id string) Record {
	var r Record
	b, err := os.ReadFile(s.path(kind, id))
	if err == nil {
		_ = json.Unmarshal(b, &r)
	}
	return r
}

// Save writes the record atomically; an empty record deletes the file.
func (s Store) Save(kind, id string, r Record) error {
	p := s.path(kind, id)
	if r.Label == "" && len(r.Notes) == 0 {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	r.Updated = time.Now().UTC()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
