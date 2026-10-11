package i3tree

import (
	"strconv"

	"go.i3wm.org/i3/v4"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// PlanSlot returns the i3 commands that move window con to position slot
// (1-based, clamped) inside its parent container. i3 has no "move to index",
// so this is a run of single moves along the container's axis.
func PlanSlot(root *i3.Node, con i3.NodeID, slot int) []string {
	if slot < 1 {
		return nil
	}
	var parent *i3.Node
	idx := -1
	root.FindChild(func(n *i3.Node) bool {
		for i, c := range n.Nodes {
			if c.ID == con {
				parent, idx = n, i
				return true
			}
		}
		return false
	})
	if parent == nil {
		return nil
	}
	target := slot - 1
	if target > len(parent.Nodes)-1 {
		target = len(parent.Nodes) - 1
	}
	back, fwd := "move up", "move down"
	if parent.Layout == i3.SplitH || parent.Layout == i3.Tabbed {
		back, fwd = "move left", "move right"
	}
	prefix := "[con_id=" + itoa(int64(con)) + "] "
	var cmds []string
	for ; idx > target; idx-- {
		cmds = append(cmds, prefix+back)
	}
	for ; idx < target; idx++ {
		cmds = append(cmds, prefix+fwd)
	}
	return cmds
}
