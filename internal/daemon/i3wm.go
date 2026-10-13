package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/murrayb52/flockr/internal/fontwidth"
	"go.i3wm.org/i3/v4"
)

func init() {
	// Honour I3SOCK like i3-msg does (useful for nested i3 sessions and tests).
	i3.SocketPathHook = func() (string, error) {
		if p := os.Getenv("I3SOCK"); p != "" {
			return p, nil
		}
		out, err := exec.Command("i3", "--get-socketpath").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("getting i3 socket path: %v (output: %s)", err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// I3 is the real window manager.
type I3 struct{}

func (I3) Tree() (*i3.Node, []i3.Workspace, error) {
	t, err := i3.GetTree()
	if err != nil {
		return nil, nil, err
	}
	ws, err := i3.GetWorkspaces()
	if err != nil {
		return nil, nil, err
	}
	return t.Root, ws, nil
}

func (I3) Command(cmds ...string) error {
	if len(cmds) == 0 {
		return nil
	}
	_, err := i3.RunCommand(strings.Join(cmds, "; "))
	return err
}

// Events reconnects automatically when i3 restarts (the library handles it).
func (I3) Events(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for ctx.Err() == nil {
			recv := i3.Subscribe(i3.WindowEventType, i3.WorkspaceEventType, i3.OutputEventType)
			go func() {
				<-ctx.Done()
				recv.Close()
			}()
			for recv.Next() {
				// title changes include flockr's own spinner frames; the
				// once-a-second poll picks up a terminal's new title
				if e, ok := recv.Event().(*i3.WindowEvent); ok && e.Change == "title" {
					continue
				}
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			recv.Close()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}()
	return ch
}

// TitleMeasure measures text in the title font of the running i3's config.
func TitleMeasure() (fontwidth.Measure, error) {
	c, err := i3.GetConfig()
	if err != nil {
		return nil, err
	}
	return fontwidth.FromI3Config(c.Config)
}
