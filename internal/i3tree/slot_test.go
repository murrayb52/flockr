package i3tree

import (
	"reflect"
	"testing"

	"go.i3wm.org/i3/v4"
)

func TestPlanSlot(t *testing.T) {
	a, b, c := win("A", "a"), win("B", "b"), win("C", "c")
	stack := con(i3.Stacked, a, b, c)
	tree := root(ws(1, "1", i3.SplitH, stack))
	cid := func(n *i3.Node) string { return "[con_id=" + itoa(int64(n.ID)) + "] " }

	cases := []struct {
		node *i3.Node
		slot int
		want []string
	}{
		{c, 1, []string{cid(c) + "move up", cid(c) + "move up"}},
		{a, 3, []string{cid(a) + "move down", cid(a) + "move down"}},
		{a, 9, []string{cid(a) + "move down", cid(a) + "move down"}}, // clamped to the last slot
		{b, 2, nil},
		{b, 0, nil},
	}
	for _, tc := range cases {
		got := PlanSlot(tree, tc.node.ID, tc.slot)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s → slot %d: got %v want %v", tc.node.Name, tc.slot, got, tc.want)
		}
	}
}

func TestPlanSlotHorizontal(t *testing.T) {
	a, b := win("A", "a"), win("B", "b")
	tree := root(ws(1, "1", i3.Tabbed, a, b))
	got := PlanSlot(tree, b.ID, 1)
	want := []string{"[con_id=" + itoa(int64(b.ID)) + "] move left"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestPlanSlotUnknownWindow(t *testing.T) {
	if got := PlanSlot(root(ws(1, "1", i3.SplitH)), 999999, 1); got != nil {
		t.Errorf("got %v", got)
	}
}
