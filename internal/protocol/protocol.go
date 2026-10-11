// Package protocol is the newline-delimited JSON protocol on flockr's Unix socket.
// Every connection sends one Request; the daemon answers with one Response, or
// for "subscribe" keeps streaming Responses carrying snapshots.
package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/murrayb52/flockr/internal/agents"
	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/model"
)

const (
	OpEvent     = "event"     // Event: an agent harness event
	OpSubscribe = "subscribe" // stream snapshots
	OpCommand   = "cmd"       // Args[0] = command name, rest = arguments
	OpGetNotes  = "get-notes" // Pane
	OpSetNotes  = "set-notes" // Pane, Notes
	OpSetLabel  = "set-label" // Session, Text
)

type Request struct {
	Op      string        `json:"op"`
	Event   *agents.Event `json:"event,omitempty"`
	Args    []string      `json:"args,omitempty"`
	Pane    string        `json:"pane,omitempty"`
	Session string        `json:"session,omitempty"`
	Notes   []model.Note  `json:"notes,omitempty"`
	Text    string        `json:"text,omitempty"`
}

type Response struct {
	OK       bool            `json:"ok"`
	Error    string          `json:"error,omitempty"`
	Message  string          `json:"message,omitempty"`
	Title    string          `json:"title,omitempty"`
	Notes    []model.Note    `json:"notes,omitempty"`
	Snapshot *model.Snapshot `json:"snapshot,omitempty"`
}

// ErrNoDaemon is returned when nothing listens on the socket.
var ErrNoDaemon = errors.New("flockr daemon is not running (start it with `flockr daemon`)")

// ErrUntrusted means the socket's directory could have been planted by
// another user, so flockr refuses to talk to it.
var ErrUntrusted = errors.New("flockr socket directory is not private to you")

func dial(socket string, timeout time.Duration) (net.Conn, error) {
	if err := config.CheckPrivateDir(filepath.Dir(socket)); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoDaemon
		}
		return nil, fmt.Errorf("%w: %v", ErrUntrusted, err)
	}
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return nil, ErrNoDaemon
	}
	return c, nil
}

// Call sends one request and waits for its response.
func Call(socket string, req Request, timeout time.Duration) (Response, error) {
	c, err := dial(socket, timeout)
	if err != nil {
		return Response{}, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("reading reply: %w", err)
	}
	if !resp.OK && resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// Subscribe streams snapshots until the connection closes; the returned
// channel is closed then.
func Subscribe(socket string) (<-chan model.Snapshot, func(), error) {
	c, err := dial(socket, time.Second)
	if err != nil {
		return nil, nil, err
	}
	if err := json.NewEncoder(c).Encode(Request{Op: OpSubscribe}); err != nil {
		c.Close()
		return nil, nil, err
	}
	ch := make(chan model.Snapshot, 4)
	go func() {
		defer close(ch)
		dec := json.NewDecoder(bufio.NewReader(c))
		for {
			var r Response
			if err := dec.Decode(&r); err != nil {
				return
			}
			if r.Snapshot != nil {
				ch <- *r.Snapshot
			}
		}
	}()
	return ch, func() { c.Close() }, nil
}
