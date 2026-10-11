// Package tmux drives flock's private tmux server: every flock terminal is one
// session on it, its top status lines are the label header and notes strip,
// and display-popup hosts the notes editor.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type Client struct {
	Socket string // tmux -L name
}

type Pane struct {
	Session string
	ID      string // %N
	Cwd     string
	Command string
	Label   string // @flock_label on the session
	PID     int
	// LabelInput is what the label prompt wrote ("=" + text); the daemon
	// consumes and clears it. Labels never pass through a shell.
	LabelInput string
}

func (c Client) cmd(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-L", c.Socket}, args...)...)
}

func (c Client) run(args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := c.cmd(args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// ErrNoServer means the flock tmux server is not running (no flock terminals).
var ErrNoServer = errors.New("no flock tmux server")

const paneFormat = "#{session_name}\t#{pane_id}\t#{pane_current_path}\t#{pane_current_command}\t#{@flock_label}\t#{pane_pid}\t#{@flock_label_input}"

// ListPanes returns the first pane of every session (flock terminals have one).
func (c Client) ListPanes() ([]Pane, error) {
	out, err := c.run("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "error connecting") {
			return nil, ErrNoServer
		}
		return nil, err
	}
	return parsePanes(out), nil
}

func parsePanes(out string) []Pane {
	seen := map[string]bool{}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		var pid int
		fmt.Sscan(f[5], &pid)
		p := Pane{Session: f[0], ID: f[1], Cwd: f[2], Command: f[3], Label: f[4], PID: pid}
		if len(f) > 6 {
			p.LabelInput = f[6]
		}
		panes = append(panes, p)
	}
	return panes
}

// Set sets session options in one tmux invocation. Keys are applied in sorted
// order so batches are deterministic.
func (c Client) Set(session string, opts map[string]string) error {
	if len(opts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var args []string
	for i, k := range keys {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, "set-option", "-t", session, k, opts[k])
	}
	_, err := c.run(args...)
	return err
}

// Client returns a tmux client attached to session (popups and prompts need one).
func (c Client) Client(session string) (string, error) {
	out, err := c.run("list-clients", "-t", session, "-F", "#{client_name}")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	if name == "" {
		return "", fmt.Errorf("session %s has no client", session)
	}
	return name, nil
}

// Popup opens a popup running shellCmd just under the header and blocks until
// it closes.
func (c Client) Popup(session, title, shellCmd string, header, height int) error {
	client, err := c.Client(session)
	if err != nil {
		return err
	}
	_, err = c.run("display-popup", "-c", client, "-E", "-B",
		"-x", "0", "-y", fmt.Sprint(header+height), "-w", "100%", "-h", fmt.Sprint(height),
		"-T", title, shellCmd)
	return err
}

// ClosePopup closes any popup on the session's client.
func (c Client) ClosePopup(session string) error {
	client, err := c.Client(session)
	if err != nil {
		return err
	}
	_, err = c.run("display-popup", "-C", "-c", client)
	return err
}

// Prompt shows tmux's command prompt in the session's status line; tmux runs
// template with %% replaced by the (quote-escaped) answer.
func (c Client) Prompt(session, prompt, initial, template string) error {
	client, err := c.Client(session)
	if err != nil {
		return err
	}
	_, err = c.run("command-prompt", "-t", client, "-p", prompt, "-I", initial, template)
	return err
}

// Source reloads a config file into a running server (ignored if none runs).
func (c Client) Source(path string) error {
	_, err := c.run("source-file", path)
	return err
}

// Running reports whether the flock tmux server is up.
func (c Client) Running() bool {
	return c.cmd("has-session").Run() == nil
}
