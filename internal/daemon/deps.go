package daemon

import (
	"context"

	"github.com/murrayb52/flockr/internal/tmux"
	"go.i3wm.org/i3/v4"
)

// WM is the window manager (i3) as the daemon needs it.
type WM interface {
	Tree() (*i3.Node, []i3.Workspace, error)
	Command(cmds ...string) error
	// Events signals (without detail) whenever windows or workspaces change.
	Events(ctx context.Context) <-chan struct{}
}

// Mux is the private tmux server.
type Mux interface {
	ListPanes() ([]tmux.Pane, error)
	Set(session string, opts map[string]string) error
	Popup(session, title, shellCmd string, header, height int) error
	ClosePopup(session string) error
	Prompt(session, prompt, initial, template string) error
}

// Notifier shows desktop notifications, one per key (a later one replaces it).
type Notifier interface {
	Notify(key, title, body string, critical bool) error
	Close(key string)
	// Actions delivers the keys of notifications whose default action was invoked.
	Actions() <-chan string
}

// Urgency sets or clears the X11 urgency hint on a client window.
type Urgency interface {
	Set(xid int64, on bool) error
}

// Launcher starts a detached shell command (used to open the sidebar terminal).
type Launcher func(shellCmd string) error
