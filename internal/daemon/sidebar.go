package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/murrayb52/flock/internal/config"

	"github.com/murrayb52/flock/internal/i3tree"
	"go.i3wm.org/i3/v4"
)

func (d *Daemon) sidebarNode() *i3.Node {
	if d.tree == nil {
		return nil
	}
	return d.tree.FindChild(func(n *i3.Node) bool {
		return n.Window != 0 && n.WindowProperties.Class == i3tree.SidebarClass
	})
}

// ensureSidebar opens the sidebar terminal if it is not on screen.
func (d *Daemon) ensureSidebar() {
	d.mu.Lock()
	missing := d.sidebarNode() == nil && !d.launched && d.d.Launch != nil
	if missing {
		d.launched = true
	}
	d.mu.Unlock()
	if missing {
		d.launchSidebar()
	}
}

func (d *Daemon) launchSidebar() {
	cmd := d.cfg.TerminalCommand(i3tree.SidebarClass, shellQuote(d.d.Bin)+" sidebar", d.font)
	if err := d.d.Launch(cmd); err != nil {
		log.Printf("launching sidebar: %v", err)
	}
}

// Font sizes the sidebar's − + buttons step between.
const (
	minFont = 5.0
	maxFont = 30.0
)

// settings are choices made from the sidebar itself, kept apart from the
// user's config file so flock never rewrites it.
type settings struct {
	SidebarFont float64 `json:"sidebar_font_size,omitempty"`
}

func settingsPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "settings.json") }

func loadFont(cfg config.Config) float64 {
	var s settings
	if b, err := os.ReadFile(settingsPath(cfg)); err == nil && json.Unmarshal(b, &s) == nil && s.SidebarFont > 0 {
		return s.SidebarFont
	}
	return cfg.SidebarFont
}

// setFont changes the sidebar's font size by step (or to size when step is 0),
// saves it and reopens the sidebar terminal, the one way that works for every
// terminal. The left gap stays put meanwhile so windows do not jump.
func (d *Daemon) setFont(size, step float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if step != 0 {
		size = d.font + step
	}
	size = math.Max(minFont, math.Min(maxFont, math.Round(size*2)/2))
	if size == d.font {
		return nil
	}
	d.font = size
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(settings{SidebarFont: size})
	if err := config.WriteFileAtomic(settingsPath(d.cfg), b, 0o600); err != nil {
		return err
	}
	if n := d.sidebarNode(); n != nil && !d.relaunch {
		d.relaunch = true
		if err := d.d.WM.Command(fmt.Sprintf("[con_id=%d] kill", n.ID)); err != nil {
			d.relaunch = false
			return err
		}
	}
	d.rebuild()
	return nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// placeSidebar keeps the sidebar docked at the left edge of the focused
// workspace's output and the left gap equal to its real width (terminals snap
// to whole cells, so the width i3 reports can differ from the one requested).
func (d *Daemon) placeSidebar() {
	n := d.sidebarNode()
	if n == nil && d.relaunch {
		// closed by setFont: reopen it at the new size, keeping the margin
		d.sidebarReq = [2]int64{}
		if !d.launched && d.d.Launch != nil {
			d.launched = true
			d.launchSidebar()
		}
		return
	}
	if n == nil {
		// Not open (or closed by the user): no margin. It only reopens on
		// daemon start or `flock sidebar show`.
		if d.gap > 0 && d.d.WM.Command("gaps left all set 0") == nil {
			d.gap = 0
		}
		d.sidebarReq = [2]int64{}
		return
	}
	d.launched = false // the launched window has arrived
	if d.relaunch && n.ID != d.sidebarStyled {
		d.relaunch = false
	}
	var area i3.Rect
	var output string
	for _, ws := range d.infos {
		if ws.Focused {
			area, output = ws.Rect, ws.Output
		}
	}
	if area.Width == 0 {
		return
	}
	// The workspace rect already excludes the gap we add, so the left edge comes
	// from the output itself; top and height still come from the workspace (bars).
	gapNow := int64(-1)
	if out := d.tree.FindChild(func(n *i3.Node) bool { return n.Type == i3.OutputNode && n.Name == output }); out != nil {
		gapNow = area.X - out.Rect.X
		area.X = out.Rect.X
	}
	want := int64(d.cfg.SidebarWidth)
	if d.condensed {
		want = int64(d.cfg.RailWidth)
	}
	var cmds []string
	con := fmt.Sprintf("[con_id=%d] ", n.ID)
	// style each new sidebar window once: catch-all for_window border rules in
	// user configs would otherwise win over the example's rule
	if d.sidebarStyled != n.ID {
		d.sidebarStyled = n.ID
		cmds = append(cmds, con+"floating enable", con+"sticky enable", con+"border none")
	}
	if abs(n.Rect.X-area.X) > 1 || abs(n.Rect.Y-area.Y) > 1 {
		cmds = append(cmds, con+fmt.Sprintf("move absolute position %d px %d px", area.X, area.Y))
	}
	if req := [2]int64{want, area.Height}; req != d.sidebarReq {
		d.sidebarReq = req
		cmds = append(cmds, con+fmt.Sprintf("resize set %d px %d px", want, area.Height))
	}
	if len(cmds) > 0 {
		if err := d.d.WM.Command(cmds...); err != nil {
			log.Printf("placing sidebar: %v", err)
		}
		d.markDirty() // read the real size back on the next refresh
		return
	}
	// compare with the gap i3 actually has, not the one we last set: anything
	// (a config reload, another tool) may have reset it
	if gap := n.Rect.Width; gap != gapNow {
		if err := d.d.WM.Command(fmt.Sprintf("gaps left all set %d", gap)); err == nil {
			d.gap = int(gap)
		}
	}
}
