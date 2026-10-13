package i3tree

import (
	"reflect"
	"testing"

	"github.com/murrayb52/flockr/internal/model"
	"go.i3wm.org/i3/v4"
)

var nextID i3.NodeID = 100

func win(class, title string) *i3.Node {
	nextID++
	return &i3.Node{ID: nextID, Type: i3.Con, Name: title, Window: int64(nextID) * 10,
		WindowProperties: i3.WindowProperties{Class: class, Title: title}}
}

func con(layout i3.Layout, kids ...*i3.Node) *i3.Node {
	nextID++
	n := &i3.Node{ID: nextID, Type: i3.Con, Layout: layout, Nodes: kids}
	if len(kids) > 0 {
		for _, k := range kids {
			n.Focus = append(n.Focus, k.ID)
		}
	}
	return n
}

func focusFirst(n *i3.Node, kid *i3.Node) *i3.Node {
	n.Focus = append([]i3.NodeID{kid.ID}, n.Focus...)
	return n
}

// ws builds a workspace node; the number lives in GET_WORKSPACES, so it is unused here.
func ws(_ int64, name string, layout i3.Layout, kids ...*i3.Node) *i3.Node {
	n := con(layout, kids...)
	n.Type, n.Name = i3.WorkspaceNode, name
	return n
}

func root(wss ...*i3.Node) *i3.Node {
	content := &i3.Node{Type: i3.Con, Name: "content", Nodes: wss}
	out := &i3.Node{Type: i3.OutputNode, Name: "eDP-1", Nodes: []*i3.Node{content}}
	scratch := &i3.Node{Type: i3.OutputNode, Name: "__i3", Nodes: []*i3.Node{{Type: i3.Con, Name: "content",
		Nodes: []*i3.Node{ws(-1, "__i3_scratch", i3.SplitH, win("Firefox", "hidden"))}}}}
	return &i3.Node{Type: i3.Root, Nodes: []*i3.Node{scratch, out}}
}

func names(g model.Group) []string {
	var out []string
	for _, w := range g.Windows {
		out = append(out, w.App)
	}
	return out
}

func TestSplitOfTwoStacks(t *testing.T) {
	api, mob, sh := win("Alacritty", "claude [flockr:a1]"), win("Alacritty", "pi [flockr:a2]"), win("Alacritty", "zsh [flockr:a3]")
	ff, code := win("firefox", "Bookshelf — Mozilla Firefox"), win("Code", "x - Visual Studio Code")
	mob.Focused = true
	left := focusFirst(con(i3.Stacked, api, mob, sh), mob)
	right := con(i3.Stacked, ff, code)
	w := ws(1, "1:Bookshelf", i3.SplitH, left, right)
	terms := map[string]*model.Term{"a1": {Session: "a1"}, "a2": {Session: "a2"}, "a3": {Session: "a3"}}
	got := Build(root(w), []i3.Workspace{{Num: 1, Name: "1:Bookshelf", Focused: true, Visible: true, Output: "eDP-1"}}, terms)

	if len(got) != 1 {
		t.Fatalf("workspaces = %d (scratchpad must be skipped)", len(got))
	}
	g := got[0]
	if g.Name != "Bookshelf" || g.Num != 1 || !g.Focused || len(g.Groups) != 2 {
		t.Fatalf("workspace = %+v", g)
	}
	if g.Groups[0].Label != "left · stack of 3" || g.Groups[1].Label != "right · stack of 2" {
		t.Errorf("labels = %q, %q", g.Groups[0].Label, g.Groups[1].Label)
	}
	if !g.Groups[0].Focused || g.Groups[1].Focused {
		t.Error("left group should be the focused one")
	}
	if !reflect.DeepEqual(names(g.Groups[0]), []string{"term", "term", "term"}) ||
		!reflect.DeepEqual(names(g.Groups[1]), []string{"firefox", "code"}) {
		t.Errorf("apps = %v %v", names(g.Groups[0]), names(g.Groups[1]))
	}
	l := g.Groups[0].Windows
	if l[0].Term != terms["a1"] {
		t.Errorf("terms wrong: %+v", l)
	}
	if l[0].Visible || !l[1].Visible || !l[1].Focused || l[2].Visible {
		t.Errorf("only the focused stack member is visible: %+v", l)
	}
	if r := g.Groups[1].Windows; !r[0].Visible || r[1].Visible {
		t.Errorf("right stack shows its first focus child: %+v", r)
	}
}

func TestWorkspaceThatIsItselfAStack(t *testing.T) {
	a, b := win("Alacritty", "x [flockr:z]"), win("tidal-hifi", "Tidal")
	w := ws(3, "3:Life admin", i3.Stacked, a, b)
	got := Build(root(w), []i3.Workspace{{Num: 3, Name: "3:Life admin"}}, nil)[0]
	if len(got.Groups) != 1 || got.Groups[0].Label != "stack of 2" || got.Groups[0].Windows[1].App != "music" {
		t.Fatalf("got %+v", got.Groups)
	}
	if got.Groups[0].Windows[0].Term != nil {
		t.Error("unknown flockr session must not invent a term")
	}
	if got.Visible {
		t.Error("visibility comes from GET_WORKSPACES")
	}
}

func TestLoneWindowsSideBySide(t *testing.T) {
	a, b, c := win("Alacritty", "a"), win("Alacritty", "b"), win("Alacritty", "c")
	got := Build(root(ws(4, "4", i3.SplitH, a, b, c)), []i3.Workspace{{Num: 4, Name: "4"}}, nil)[0]
	if got.Name != "" {
		t.Errorf("plain numbered workspace has no name, got %q", got.Name)
	}
	var labels []string
	for _, g := range got.Groups {
		labels = append(labels, g.Label)
	}
	if !reflect.DeepEqual(labels, []string{"left", "middle", "right"}) {
		t.Errorf("labels = %v", labels)
	}
}

func TestNestedSplitAndVertical(t *testing.T) {
	a, b, c := win("Alacritty", "a"), win("Alacritty", "b"), win("firefox", "f")
	inner := con(i3.SplitH, a, b)
	got := Build(root(ws(2, "2:x", i3.SplitV, inner, c)), []i3.Workspace{{Num: 2, Name: "2:x"}}, nil)[0]
	if len(got.Groups) != 2 || got.Groups[0].Label != "top · 2 side by side" || got.Groups[1].Label != "bottom" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if len(got.Groups[0].Windows) != 2 {
		t.Errorf("nested windows flattened in order: %+v", got.Groups[0].Windows)
	}
}

func TestFloatingAndEmpty(t *testing.T) {
	a := win("Alacritty", "a")
	w := ws(5, "5", i3.SplitH, a)
	fl := win("Pavucontrol", "Volume")
	w.FloatingNodes = []*i3.Node{{Type: i3.FloatingCon, Nodes: []*i3.Node{fl}}}
	empty := ws(6, "6:Empty", i3.SplitH)
	got := Build(root(w, empty), []i3.Workspace{{Num: 5, Name: "5"}, {Num: 6, Name: "6:Empty"}}, nil)
	if len(got[0].Groups) != 2 || got[0].Groups[1].Label != "floating" || len(got[0].Groups[1].Windows) != 1 {
		t.Fatalf("groups = %+v", got[0].Groups)
	}
	if len(got[1].Groups) != 0 || got[1].Name != "Empty" {
		t.Fatalf("empty = %+v", got[1])
	}
}

func TestAppKeys(t *testing.T) {
	cases := map[string]string{"firefox": "firefox", "Google-chrome": "chrome", "Code": "code", "obsidian": "obsidian",
		"tidal-hifi": "music", "Spotify": "music", "tutanota-desktop": "mail", "Alacritty": "terminal",
		"com.mitchellh.ghostty": "terminal", "Gimp": "gimp", "": "window"}
	for class, want := range cases {
		if got := AppKey(class); got != want {
			t.Errorf("%q → %q want %q", class, got, want)
		}
	}
}

func TestSessionFromTitle(t *testing.T) {
	cases := map[string]string{"claude — api [flockr:f3a9]": "f3a9", "[flockr:x]": "x", "zsh": "", "[flockr:]": "", "a [flockr:b": ""}
	for title, want := range cases {
		if got := SessionFromTitle(title); got != want {
			t.Errorf("%q → %q want %q", title, got, want)
		}
	}
}
