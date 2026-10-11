// Package i3tree turns i3's layout tree into the workspace/group/window view the
// sidebar shows, and plans the i3 commands for flock's window-moving helpers.
package i3tree

import (
	"fmt"
	"strings"

	"github.com/murrayb52/flock/internal/model"
	"go.i3wm.org/i3/v4"
)

// SidebarClass is the WM class of flock's own sidebar window; it is never listed.
const SidebarClass = "flock-sidebar"

// Build lists the real workspaces (scratchpad excluded) in tree order.
// infos supplies focus/visibility/urgency from GET_WORKSPACES; terms maps a
// flock tmux session id (found in the window title) to its terminal.
func Build(root *i3.Node, infos []i3.Workspace, terms map[string]*model.Term) []model.Workspace {
	byName := map[string]i3.Workspace{}
	for _, w := range infos {
		byName[w.Name] = w
	}
	var out []model.Workspace
	var walk func(n *i3.Node)
	walk = func(n *i3.Node) {
		if n.Type == i3.WorkspaceNode {
			if !strings.HasPrefix(n.Name, "__i3") {
				out = append(out, workspace(n, byName[n.Name], terms))
			}
			return
		}
		for _, c := range n.Nodes {
			walk(c)
		}
	}
	walk(root)
	return out
}

func workspace(n *i3.Node, info i3.Workspace, terms map[string]*model.Term) model.Workspace {
	w := model.Workspace{Num: info.Num, Name: DisplayName(n.Name), Focused: info.Focused,
		Visible: info.Visible, Urgent: info.Urgent, Output: info.Output}
	if info.Name == "" {
		w.Num = leadingNum(n.Name)
	}
	b := builder{terms: terms, wsVisible: info.Visible}
	if (n.Layout == i3.Stacked || n.Layout == i3.Tabbed) && len(n.Nodes) > 0 {
		w.Groups = append(w.Groups, b.group(n, containerLabel(n), true))
	} else {
		for i, c := range n.Nodes {
			pos := position(n.Layout, i, len(n.Nodes))
			label := pos
			if c.Window == 0 {
				label = join(pos, containerLabel(c))
			}
			g := b.group(c, label, isFocusChild(n, c))
			w.Groups = append(w.Groups, g)
		}
	}
	var floating []model.Window
	for _, f := range n.FloatingNodes {
		for _, leaf := range leaves(f) {
			if leaf.WindowProperties.Class == SidebarClass {
				continue
			}
			floating = append(floating, b.window(leaf, 0, true))
		}
	}
	if len(floating) > 0 {
		w.Groups = append(w.Groups, model.Group{Label: "floating", Layout: "floating", Windows: floating})
	}
	return w
}

type builder struct {
	terms     map[string]*model.Term
	wsVisible bool
}

// group collects the leaf windows under n. When n is a stack/tabs container its
// children get slot numbers and only its focused child is visible.
func (b builder) group(n *i3.Node, label string, onTop bool) model.Group {
	g := model.Group{Label: label, Layout: string(n.Layout)}
	if n.Window != 0 {
		g.Layout = "window"
	}
	var add func(n *i3.Node, slot int, visible bool)
	add = func(n *i3.Node, slot int, visible bool) {
		if n.Window != 0 {
			w := b.window(n, slot, visible && b.wsVisible)
			if w.Focused {
				g.Focused = true
			}
			g.Windows = append(g.Windows, w)
			return
		}
		stacked := n.Layout == i3.Stacked || n.Layout == i3.Tabbed
		for i, c := range n.Nodes {
			s, v := slot, visible
			if stacked {
				v = visible && isFocusChild(n, c)
				if slot == 0 && len(n.Nodes) > 1 {
					s = i + 1
				}
			}
			add(c, s, v)
		}
	}
	add(n, 0, true)
	if !g.Focused && onTop && n.Focused {
		g.Focused = true
	}
	return g
}

func (b builder) window(n *i3.Node, slot int, visible bool) model.Window {
	class := n.WindowProperties.Class
	w := model.Window{ConID: int64(n.ID), XID: n.Window, Class: class, Title: n.Name,
		App: AppKey(class), Slot: slot, Focused: n.Focused, Visible: visible}
	if id := SessionFromTitle(n.Name); id != "" {
		if t, ok := b.terms[id]; ok {
			w.App, w.Term = "term", t
		}
	}
	return w
}

func leaves(n *i3.Node) []*i3.Node {
	if n.Window != 0 {
		return []*i3.Node{n}
	}
	var out []*i3.Node
	for _, c := range n.Nodes {
		out = append(out, leaves(c)...)
	}
	return out
}

// isFocusChild reports whether c is the child of n that i3 would show/focus.
func isFocusChild(n, c *i3.Node) bool {
	if len(n.Focus) == 0 {
		return len(n.Nodes) > 0 && n.Nodes[0] == c
	}
	return n.Focus[0] == c.ID
}

func position(layout i3.Layout, i, n int) string {
	if n == 1 {
		return ""
	}
	var names []string
	switch layout {
	case i3.SplitH:
		names = map[int][]string{2: {"left", "right"}, 3: {"left", "middle", "right"}}[n]
		if names == nil {
			return fmt.Sprintf("column %d", i+1)
		}
	case i3.SplitV:
		names = map[int][]string{2: {"top", "bottom"}, 3: {"top", "middle", "bottom"}}[n]
		if names == nil {
			return fmt.Sprintf("row %d", i+1)
		}
	default:
		return fmt.Sprintf("%d", i+1)
	}
	return names[i]
}

func containerLabel(n *i3.Node) string {
	k := len(leaves(n))
	switch n.Layout {
	case i3.Stacked:
		return fmt.Sprintf("stack of %d", k)
	case i3.Tabbed:
		return fmt.Sprintf("tabs of %d", k)
	case i3.SplitV:
		return fmt.Sprintf("%d above each other", k)
	default:
		return fmt.Sprintf("%d side by side", k)
	}
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " · " + b
}

// DisplayName strips the "N:" prefix i3 users put in workspace names; a plain
// number has no display name.
func DisplayName(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return strings.TrimSpace(name[i+1:])
	}
	for _, r := range name {
		if r < '0' || r > '9' {
			return name
		}
	}
	return ""
}

func leadingNum(name string) int64 {
	var n int64 = -1
	for _, r := range name {
		if r < '0' || r > '9' {
			break
		}
		if n < 0 {
			n = 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

// SessionFromTitle extracts the flock session id from a "… [flock:<id>]" title.
func SessionFromTitle(title string) string {
	i := strings.LastIndex(title, "[flock:")
	if i < 0 || !strings.HasSuffix(title, "]") {
		return ""
	}
	return title[i+len("[flock:") : len(title)-1]
}

var appKeys = map[string]string{
	"firefox": "firefox", "firefox-esr": "firefox", "librewolf": "firefox", "navigator": "firefox",
	"google-chrome": "chrome", "chromium": "chrome", "chromium-browser": "chrome", "brave-browser": "chrome",
	"code": "code", "code-oss": "code", "vscodium": "code", "cursor": "code",
	"obsidian":   "obsidian",
	"tidal-hifi": "music", "spotify": "music", "rhythmbox": "music",
	"thunderbird": "mail", "tutanota-desktop": "mail", "evolution": "mail", "geary": "mail",
	"slack": "chat", "discord": "chat", "signal": "chat", "telegramdesktop": "chat",
	"alacritty": "terminal", "kitty": "terminal", "com.mitchellh.ghostty": "terminal", "ghostty": "terminal",
	"org.wezfurlong.wezterm": "terminal", "foot": "terminal", "xterm": "terminal", "urxvt": "terminal",
	"gnome-terminal-server": "terminal", "konsole": "terminal", "st-256color": "terminal",
	"nautilus": "files", "thunar": "files", "pcmanfm": "files", "dolphin": "files",
}

// AppKey normalises a WM class to the key used for icons and names.
func AppKey(class string) string {
	c := strings.ToLower(class)
	if k, ok := appKeys[c]; ok {
		return k
	}
	if c == "" {
		return "window"
	}
	return c
}
