package hook

import (
	"testing"

	"github.com/murrayb52/flock/internal/agents"
)

// payloads captured from Claude Code 2.1.296 (paths shortened)
const (
	sessionStart = `{"session_id":"fa8e","transcript_path":"/t.jsonl","cwd":"/w","hook_event_name":"SessionStart","source":"resume","model":"claude-opus-5-5"}`
	promptSubmit = `{"session_id":"fa8e","cwd":"/w","prompt_id":"c0","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"Use the Bash tool to run exactly: rm hello.txt"}`
	preTool      = `{"session_id":"fa8e","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm hello.txt","description":"Delete hello.txt"}}`
	preToolEdit  = `{"session_id":"fa8e","hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"/w/src/routes/lists.ts","old_string":"a","new_string":"b"}}`
	permission   = `{"session_id":"fa8e","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm hello.txt","description":"Delete hello.txt"},"permission_suggestions":[]}`
	notifyPerm   = `{"session_id":"fa8e","hook_event_name":"Notification","message":"Claude needs your permission","notification_type":"permission_prompt"}`
	notifyIdle   = `{"session_id":"fa8e","hook_event_name":"Notification","message":"Claude is waiting for your input","notification_type":"idle_prompt"}`
	stop         = `{"session_id":"fa8e","hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Done.\n\nAll tests pass."}`
	sessionEnd   = `{"session_id":"fa8e","hook_event_name":"SessionEnd","reason":"prompt_input_exit"}`
)

func TestClaudeEvents(t *testing.T) {
	cases := []struct {
		event, payload string
		want           agents.Event
	}{
		{"SessionStart", sessionStart, agents.Event{Name: "start", Source: "resume"}},
		{"UserPromptSubmit", promptSubmit, agents.Event{Name: "prompt", Prompt: "Use the Bash tool to run exactly: rm hello.txt"}},
		{"PreToolUse", preTool, agents.Event{Name: "tool", Activity: "Bash rm hello.txt"}},
		{"PreToolUse", preToolEdit, agents.Event{Name: "tool", Activity: "Edit lists.ts"}},
		{"PostToolUse", preTool, agents.Event{Name: "tool", Activity: "Bash rm hello.txt"}},
		{"PermissionRequest", permission, agents.Event{Name: "input", Detail: "rm hello.txt"}},
		{"Notification", notifyPerm, agents.Event{Name: "input"}},
		{"Stop", stop, agents.Event{Name: "stop", Detail: "Done. All tests pass."}},
		{"SessionEnd", sessionEnd, agents.Event{Name: "end"}},
	}
	for _, c := range cases {
		got, ok := FromClaude(c.event, []byte(c.payload), "%7")
		if !ok {
			t.Errorf("%s: not ok", c.event)
			continue
		}
		c.want.Agent, c.want.Pane, c.want.SessionID = "claude", "%7", "fa8e"
		if got != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", c.event, got, c.want)
		}
	}
}

func TestClaudeIgnores(t *testing.T) {
	for _, c := range []struct{ event, payload string }{
		{"Notification", notifyIdle},
		{"SubagentStop", stop},
		{"Stop", `{not json`},
		{"Stop", `{"hook_event_name":"Stop"}`}, // no session id
	} {
		if _, ok := FromClaude(c.event, []byte(c.payload), "%7"); ok {
			t.Errorf("%s %s should be ignored", c.event, c.payload)
		}
	}
}

func TestLongMessagesAreTrimmed(t *testing.T) {
	long := `{"session_id":"x","last_assistant_message":"` + string(make([]byte, 0)) + repeat("word ", 200) + `"}`
	ev, _ := FromClaude("Stop", []byte(long), "%1")
	if n := len([]rune(ev.Detail)); n > 160 {
		t.Errorf("detail is %d runes", n)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
