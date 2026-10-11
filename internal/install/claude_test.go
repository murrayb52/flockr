package install

import (
	"encoding/json"
	"strings"
	"testing"
)

const userSettings = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls:*)"]},
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}]
  },
  "zzz": 1
}`

func hooksOf(t *testing.T, b []byte) map[string][]map[string]any {
	t.Helper()
	var s struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, b)
	}
	return s.Hooks
}

func TestMergeAddsFlockHooksAndKeepsUserOnes(t *testing.T) {
	out, err := MergeClaude([]byte(userSettings), "/home/u/go/bin/flock", true)
	if err != nil {
		t.Fatal(err)
	}
	h := hooksOf(t, out)
	for _, ev := range ClaudeEvents {
		if len(h[ev]) == 0 {
			t.Errorf("no %s hook", ev)
		}
	}
	if len(h["Stop"]) != 2 || !strings.Contains(string(out), "notify-send done") {
		t.Errorf("user's Stop hook must survive: %v", h["Stop"])
	}
	if !strings.Contains(string(out), `"'/home/u/go/bin/flock' hook claude PreToolUse"`) {
		t.Errorf("hook command missing:\n%s", out)
	}
	s := string(out)
	if !(strings.Index(s, `"model"`) < strings.Index(s, `"permissions"`) && strings.Index(s, `"hooks"`) < strings.Index(s, `"zzz"`)) {
		t.Errorf("top-level key order must be preserved:\n%s", s)
	}
}

func TestMergeIsIdempotent(t *testing.T) {
	once, _ := MergeClaude([]byte(userSettings), "/bin/flock", true)
	twice, _ := MergeClaude(once, "/bin/flock", true)
	if string(once) != string(twice) {
		t.Errorf("second install changed the file:\n%s\n---\n%s", once, twice)
	}
}

func TestUninstallRemovesOnlyFlock(t *testing.T) {
	in, _ := MergeClaude([]byte(userSettings), "/bin/flock", true)
	out, err := MergeClaude(in, "/bin/flock", false)
	if err != nil {
		t.Fatal(err)
	}
	h := hooksOf(t, out)
	if len(h) != 1 || len(h["Stop"]) != 1 || strings.Contains(string(out), "hook claude") {
		t.Errorf("after uninstall:\n%s", out)
	}
}

func TestMergeEmptyAndInvalid(t *testing.T) {
	out, err := MergeClaude(nil, "/bin/flock", true)
	if err != nil || len(hooksOf(t, out)) != len(ClaudeEvents) {
		t.Fatalf("empty settings: %v\n%s", err, out)
	}
	if _, err := MergeClaude([]byte(`{"hooks": [`), "/bin/flock", true); err == nil {
		t.Error("invalid json must be refused, not overwritten")
	}
}
