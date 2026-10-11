// Package install adds (and removes) flock's integrations with agent harnesses.
package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeEvents are the Claude Code hook events flock listens to.
var ClaudeEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
	"PermissionRequest", "Notification", "Stop", "SessionEnd"}

const marker = " hook claude "

// ordered is a JSON object that remembers its key order.
type ordered struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseOrdered(b []byte) (*ordered, error) {
	o := &ordered{vals: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(b)) == 0 {
		return o, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *ordered) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *ordered) del(k string) {
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *ordered) marshal(indent string) []byte {
	var b bytes.Buffer
	b.WriteString("{")
	for i, k := range o.keys {
		if i > 0 {
			b.WriteString(",")
		}
		kb, _ := json.Marshal(k)
		b.WriteString("\n" + indent + "  ")
		b.Write(kb)
		b.WriteString(": ")
		var v bytes.Buffer
		json.Indent(&v, o.vals[k], indent+"  ", "  ")
		b.Write(v.Bytes())
	}
	if len(o.keys) > 0 {
		b.WriteString("\n" + indent)
	}
	b.WriteString("}")
	return b.Bytes()
}

type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func isFlock(e json.RawMessage) bool { return bytes.Contains(e, []byte(marker)) }

// MergeClaude returns settings.json content with flock's hooks installed
// (install=true) or removed, leaving every other setting and hook untouched.
func MergeClaude(settings []byte, bin string, install bool) ([]byte, error) {
	top, err := parseOrdered(settings)
	if err != nil {
		return nil, fmt.Errorf("settings.json is not valid JSON, refusing to touch it: %w", err)
	}
	hooks, err := parseOrdered(top.vals["hooks"])
	if err != nil {
		return nil, fmt.Errorf("settings.json \"hooks\" is not an object: %w", err)
	}
	for _, ev := range ClaudeEvents {
		var entries []json.RawMessage
		if raw, ok := hooks.vals[ev]; ok {
			if err := json.Unmarshal(raw, &entries); err != nil {
				return nil, fmt.Errorf("hooks.%s is not a list: %w", ev, err)
			}
		}
		kept := entries[:0]
		for _, e := range entries {
			if !isFlock(e) {
				kept = append(kept, e)
			}
		}
		if install {
			entry := hookEntry{Hooks: []hookCommand{{Type: "command", Command: shellQuote(bin) + marker + ev, Timeout: 5}}}
			if ev == "PreToolUse" || ev == "PostToolUse" || ev == "PermissionRequest" {
				entry.Matcher = "*"
			}
			b, _ := json.Marshal(entry)
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			hooks.del(ev)
			continue
		}
		b, _ := json.Marshal(kept)
		hooks.set(ev, b)
	}
	if len(hooks.keys) == 0 {
		top.del("hooks")
	} else {
		top.set("hooks", hooks.marshal(""))
	}
	return append(top.marshal(""), '\n'), nil
}

// Claude installs or removes the hooks in ~/.claude/settings.json (or $CLAUDE_CONFIG_DIR).
func Claude(bin string, install bool) (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	path := filepath.Join(dir, "settings.json")
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, err
	}
	out, err := MergeClaude(old, bin, install)
	if err != nil {
		return path, err
	}
	if bytes.Equal(old, out) {
		return path, nil
	}
	if len(old) > 0 {
		if err := os.WriteFile(path+".flock-backup", old, 0o600); err != nil {
			return path, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return path, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return path, err
	}
	return path, os.Rename(tmp, path)
}
