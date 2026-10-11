package daemon

import (
	"fmt"
	"log"

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
		cmd := d.cfg.TerminalCommand(i3tree.SidebarClass, shellQuote(d.d.Bin)+" sidebar")
		if err := d.d.Launch(cmd); err != nil {
			log.Printf("launching sidebar: %v", err)
		}
	}
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
	if out := d.tree.FindChild(func(n *i3.Node) bool { return n.Type == i3.OutputNode && n.Name == output }); out != nil {
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
	if gap := int(n.Rect.Width); gap != d.gap {
		if err := d.d.WM.Command(fmt.Sprintf("gaps left all set %d", gap)); err == nil {
			d.gap = gap
		}
	}
}
