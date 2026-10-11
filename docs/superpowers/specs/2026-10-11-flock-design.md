# flock — design

Date: 2026-10-11 · Status: approved in conversation (user authorised implementation without a further review gate)

## Purpose

flock is a local-only, keyboard-first companion for i3 that keeps an eye on CLI coding agents
(Claude Code and Pi). It is inspired by [herdr](https://herdr.dev/) but does not replace the
terminal or the window manager: windows stay ordinary i3 windows, every binding is an i3 binding
(so agents can never swallow it), and flock only adds:

- a sidebar listing the current workspace's windows, grouped the way i3 lays them out
  (side-by-side columns, stacks, tabs), with app icons, agent state and note counts;
  it condenses to an icon rail;
- agent state tracking (working / needs input / done / idle) from agent hooks, with desktop
  notifications, i3 urgency (red title strip + red workspace) and "jump to next agent that needs me";
- per-terminal labels and per-agent-session notes, shown in a header at the top of each terminal and
  restored when an agent session is resumed;
- a status feed for polybar (workspace name + one status logo per workspace);
- helpers i3 bindings call: move window to stack slot N, label, notes, next, sidebar toggle.

Success: someone with any i3 config can `go install` flock, add the example i3 snippet, run
`flock install claude` / `flock install pi`, and get the behaviour of `demo/flock-mockup.html`.

## Constraints and findings (spike, 2026-10-11)

- i3 ≥ 4.22 (gaps). With `font pango:<any> 1` stacked/tabbed title bars are 6 px "slim strips";
  i3 colours an urgent window's strip with `client.urgent`.
- The sidebar is a terminal window (`class flock-sidebar`), floating + sticky + `no_focus`, placed in
  a `gaps left` margin. Gaps can be changed at runtime. Terminals snap to cell sizes, so flock reads
  the real width back and sizes the gap to match.
- tmux ≥ 3.2: each flock terminal runs in its own session on a private server (`tmux -L flock`)
  with no prefix and no prefix bindings. A two-line top status bar is the label header + notes strip;
  `display-popup` is the notes drawer. `destroy-unattached on` so closing the window ends the shell.
- Claude Code hooks (verified on 2.1.296): `SessionStart{source: startup|resume|clear|compact}`,
  `UserPromptSubmit{prompt}`, `PreToolUse`, `PostToolUse`, `PermissionRequest{tool_name, tool_input}`,
  `Notification{notification_type: permission_prompt|idle_prompt, message}`,
  `Stop{last_assistant_message}`, `SessionEnd{reason}`. All carry `session_id`, `cwd`.
  Resume keeps the same `session_id`. `$TMUX_PANE` is inherited by hook processes.
- Pi (1.1.0) extension events: `session_start{reason}`, `before_agent_start{prompt}`, `agent_start`,
  `agent_end{messages}`, `agent_settled`, `ui_prompt_start/end`, `session_shutdown`;
  `ctx.sessionManager.getSessionId()`; guard on `ctx.mode === "tui"`.

## Architecture

One Go binary, `flock`, no cgo. Long-running pieces:

| Process | Role |
|---|---|
| `flock daemon` | Single instance. Subscribes to i3 IPC, polls the flock tmux server, receives agent events on `$XDG_RUNTIME_DIR/flock/flock.sock`, owns all state, persists labels/notes, sends notifications (D-Bus), sets/clears X11 urgency, keeps tmux header options up to date, places the sidebar, streams state to subscribers. |
| `flock sidebar` | Bubble Tea TUI inside a terminal window the daemon launches. Subscribes to the state stream. Renders expanded list or icon rail depending on its width. |
| `flock bar` | Subscribes to the stream and prints one polybar-formatted line per change (`custom/script`, `tail = true`). |
| tmux server `-L flock` | One session per `flock term` window. |

Short-lived invocations: `flock term`, `flock hook claude <event>` (reads stdin, forwards, always exits 0
quickly, never prints), `flock label`, `flock notes`, `flock notes-ui`, `flock slot N`, `flock next`,
`flock jump <con_id>`, `flock sidebar toggle|condense|expand`, `flock install claude|pi`,
`flock status`.

### Window ↔ terminal ↔ agent mapping

`flock term` starts `tmux -L flock new-session -s <id>`; tmux `set-titles` makes the X window title end
in `[flock:<id>]`, which the daemon matches in the i3 tree (terminal-agnostic, works with
single-process terminals such as Ghostty). Hooks send `$TMUX_PANE`; the daemon maps pane → session →
window. A pane whose foreground command returns to a shell clears its agent (crash safety).

### State

- Window (from i3): con id, X window id, class, title, workspace, ancestry/layout, focused, visible.
- Terminal (tmux session): id, pane, cwd, foreground command, terminal label.
- Agent (per pane): kind (claude|pi), session id, state (working|input|idle), unseen flag,
  since, last prompt, detail (permission command or last message).
- Session record, persisted at `$XDG_DATA_HOME/flock/sessions/<kind>-<session_id>.json`:
  `{label, notes:[{text, done}], updated}`. Loaded on agent session start/resume.
- Terminal labels live on the tmux session (`@flock_label`) so they survive daemon restarts.

State transitions: prompt submitted / tool use → working; permission prompt or Pi UI prompt or Pi
question → input; stop/settled → idle (unseen=true unless the window is focused); focusing a window
clears unseen and its notification. Input sets the X urgency hint; leaving input clears it.

### Display rules (from the approved mockup)

- Sidebar lists only the focused workspace. Groups follow i3: top-level children of the workspace in
  order; a split child is labelled left/middle/right (or top/bottom), stacks "stack of N", tabs
  "tabs of N". Slot numbers are shown inside stacks/tabs. "◉" marks windows on screen; the focused
  row is highlighted. No workspace heading (polybar shows it).
- Name: label, else for agents the latest prompt (italic), else `zsh — <dir>` for flock terminals,
  else the app name derived from WM class.
- Icons: coloured glyph tiles (Claude ✻ orange, Pi π purple, shell ›_ grey-green, Firefox, Chrome,
  VS Code, Obsidian, music, mail, generic first letter).
- Status logos: input `!` red, done `✓` green, working spinner amber, idle `○` grey. No pulsing.
- Header (tmux status line 1): icon, label/auto-name, process · cwd, state. Line 2 (agents only):
  `notes [done/total]` + `Alt+. open/add` hint.
- Notes drawer (popup under the header): Tab/↓ next, Shift+Tab/↑ previous, Space tick, typing or
  Enter edits, Enter/Esc finish edit, Tab in edit = finish + next, Del removes, Esc closes;
  last row "+ new note". Saved on every change.
- Bar: `N Name` per workspace with its most urgent logo; focused workspace highlighted; urgent
  workspaces red text only.

### Error handling

Hooks and the Pi extension never block or fail the agent: 300 ms connect timeout, errors swallowed.
If the daemon is down, commands print a short hint and exit 1 (bindings fail silently). The daemon
reconnects to i3 and D-Bus if they restart; missing tmux/D-Bus degrade features rather than crash.

### Testing

Unit tests for the state machine, i3 tree → sidebar groups, slot-move planning, notes editing model,
bar formatting, hook payload parsing, session store, tmux config/format generation.
Integration check in a nested Xephyr i3 with a fake agent script that emits real hook payloads;
the same harness records the README GIFs.

## Repository layout

```
cmd/flock/            main + subcommand dispatch
internal/protocol/    socket messages
internal/daemon/      state, i3, tmux, notify, x11 urgency, persistence
internal/sidebar/     Bubble Tea sidebar
internal/notesui/     Bubble Tea notes editor
internal/bar/         polybar formatter
internal/install/     claude hook merge, pi extension
internal/ui/          shared icons, colours, naming
integrations/pi/flock.ts
examples/i3.conf, examples/polybar.ini
demo/                 mockup, fake agent, GIF recording scripts
```

## Out of scope (for now)

Sway/Wayland, multi-monitor sidebars (sidebar follows the focused output), real app icon images,
remote/server mode, configurable keymaps inside the notes editor.
