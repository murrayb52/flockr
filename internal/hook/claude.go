// Package hook adapts agent harness payloads to agents.Event.
package hook

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/murrayb52/flockr/internal/agents"
)

type claudePayload struct {
	SessionID        string          `json:"session_id"`
	Source           string          `json:"source"`
	Prompt           string          `json:"prompt"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	NotificationType string          `json:"notification_type"`
	Message          string          `json:"message"`
	LastMessage      string          `json:"last_assistant_message"`
	AgentID          string          `json:"agent_id"` // set only inside a subagent
}

// FromClaude maps a Claude Code hook (event name as configured in settings.json,
// payload from stdin) to an agents.Event. ok is false for events flockr ignores.
func FromClaude(event string, payload []byte, pane string) (agents.Event, bool) {
	var p claudePayload
	if err := json.Unmarshal(payload, &p); err != nil || p.SessionID == "" {
		return agents.Event{}, false
	}
	ev := agents.Event{Agent: "claude", Pane: pane, SessionID: p.SessionID}
	switch event {
	case "SessionStart":
		ev.Name, ev.Source = "start", p.Source
	case "UserPromptSubmit":
		ev.Name, ev.Prompt = "prompt", Trim(p.Prompt, 200)
	case "PreToolUse", "PostToolUse":
		// A background subagent keeps using tools after the session's turn
		// ends, and never sends Stop: its tools would leave the agent
		// "working" for good. (Its permission requests still count.)
		if p.AgentID != "" {
			return agents.Event{}, false
		}
		ev.Name, ev.Activity = "tool", Trim(strings.TrimSpace(p.ToolName+" "+toolSummary(p.ToolName, p.ToolInput)), 80)
	case "PermissionRequest":
		ev.Name, ev.Detail = "input", Trim(permissionSummary(p.ToolName, p.ToolInput), 160)
	case "Notification":
		if p.NotificationType != "permission_prompt" &&
			!(p.NotificationType == "" && strings.Contains(strings.ToLower(p.Message), "permission")) {
			return ev, false
		}
		ev.Name = "input"
	case "Stop":
		ev.Name, ev.Detail = "stop", Trim(p.LastMessage, 160)
	case "SessionEnd":
		ev.Name = "end"
	default:
		return ev, false
	}
	return ev, true
}

type toolInput struct {
	Command  string `json:"command"`
	FilePath string `json:"file_path"`
	Pattern  string `json:"pattern"`
	URL      string `json:"url"`
	Query    string `json:"query"`
}

func toolSummary(tool string, raw json.RawMessage) string {
	var in toolInput
	_ = json.Unmarshal(raw, &in)
	switch {
	case in.Command != "":
		return firstLine(in.Command)
	case in.FilePath != "":
		return filepath.Base(in.FilePath)
	case in.Pattern != "":
		return in.Pattern
	case in.URL != "":
		return in.URL
	case in.Query != "":
		return in.Query
	}
	return ""
}

func permissionSummary(tool string, raw json.RawMessage) string {
	var in toolInput
	_ = json.Unmarshal(raw, &in)
	if in.Command != "" {
		return firstLine(in.Command)
	}
	return strings.TrimSpace(tool + " " + toolSummary(tool, raw))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

// Trim collapses whitespace and cuts s to at most n runes, adding an ellipsis.
func Trim(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
