package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/protocol"
)

// ErrRunning means another daemon already owns the socket.
var ErrRunning = errors.New("flockr daemon is already running")

// Listen opens the Unix socket, refusing if a live daemon answers on it and
// replacing a stale socket file otherwise.
func Listen(path string) (net.Listener, error) {
	if err := config.PrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		c.Close()
		return nil, ErrRunning
	}
	os.Remove(path)
	return net.Listen("unix", path)
}

// Serve answers requests until ctx ends.
func (d *Daemon) Serve(ctx context.Context, l net.Listener) {
	go func() {
		<-ctx.Done()
		l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("accept: %v", err)
			}
			return
		}
		go d.handle(ctx, c)
	}
}

func (d *Daemon) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var req protocol.Request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		return
	}
	c.SetReadDeadline(time.Time{})
	enc := json.NewEncoder(c)
	if req.Op == protocol.OpSubscribe {
		d.stream(ctx, c)
		return
	}
	enc.Encode(d.Handle(req))
}

// Handle answers a single request (everything except subscribe).
func (d *Daemon) Handle(req protocol.Request) protocol.Response {
	switch req.Op {
	case protocol.OpEvent:
		if req.Event == nil {
			return fail("missing event")
		}
		d.HandleEvent(*req.Event)
		return ok("")
	case protocol.OpCommand:
		return d.Command(req.Args)
	case protocol.OpGetNotes:
		notes, name, found := d.Notes(req.Pane)
		if !found {
			return fail("no agent in pane " + req.Pane)
		}
		return protocol.Response{OK: true, Notes: notes, Title: name}
	case protocol.OpSetNotes:
		if !d.SetNotes(req.Pane, req.Notes) {
			return fail("no agent in pane " + req.Pane)
		}
		return ok("")
	}
	return fail("unknown op " + req.Op)
}

func (d *Daemon) stream(ctx context.Context, c net.Conn) {
	ch, first := d.subscribe()
	defer d.unsubscribe(ch)
	write := func(b []byte) bool {
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := c.Write(append(append([]byte(`{"ok":true,"snapshot":`), b...), '}', '\n'))
		return err == nil
	}
	if !write(first) {
		return
	}
	// a closed client shows up as a read error
	gone := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		c.Read(buf)
		close(gone)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-gone:
			return
		case b := <-ch:
			if !write(b) {
				return
			}
		}
	}
}
