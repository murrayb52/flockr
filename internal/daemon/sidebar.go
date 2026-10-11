package daemon

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/murrayb52/flockr/internal/i3tree"
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
		d.launchSidebar(false)
	}
}

// launchSidebar opens the sidebar terminal; withMenu reopens it with the menu
// open (after a font change made from the menu).
func (d *Daemon) launchSidebar(withMenu bool) {
	args := " sidebar"
	if withMenu {
		args += " --menu"
	}
	cmd := d.cfg.SidebarCommand(i3tree.SidebarClass, shellQuote(d.d.Bin)+args, d.font)
	if err := d.d.Launch(cmd); err != nil {
		log.Printf("launching sidebar: %v", err)
	}
}

// Font sizes the sidebar's − + buttons step between.
const (
	minFont = 5.0
	maxFont = 30.0
)

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
	if err := d.saveSettings(); err != nil {
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

// rendered reports whether the sidebar window sits inside its floating
// container, which i3 only guarantees for windows it is currently drawing.
func rendered(root *i3.Node, n *i3.Node) bool {
	parent := root.FindChild(func(c *i3.Node) bool {
		for _, k := range c.Nodes {
			if k.ID == n.ID {
				return true
			}
		}
		return false
	})
	if parent == nil || parent.Type != i3.FloatingCon || parent.Rect.Width == 0 {
		return true
	}
	p, r := parent.Rect, n.Rect
	return r.X >= p.X && r.Y >= p.Y && r.X+r.Width <= p.X+p.Width && r.Y+r.Height <= p.Y+p.Height
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
			d.launchSidebar(true)
		}
		return
	}
	if n == nil {
		// Not open (or closed by the user): no margin. It only reopens on
		// daemon start or `flockr sidebar show`.
		if d.gap > 0 && d.d.WM.Command("gaps left all set 0") == nil {
			d.gap = 0
		}
		d.sidebarReq = [2]int64{}
		return
	}
	d.launched = false // the launched window has arrived
	if !rendered(d.tree, n) {
		// i3 has moved the floating container but not the window itself: the
		// window is hidden (e.g. behind a fullscreen one). Re-sending commands
		// now would only loop; place it once it is drawn again.
		return
	}
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
		// the same commands again within a second: i3 is not applying them
		// (yet); leave it to the next tick rather than spin
		key := strings.Join(cmds, ";")
		now := d.d.Now()
		if key == d.placed && now.Sub(d.placedAt) < time.Second {
			return
		}
		d.placed, d.placedAt = key, now
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
