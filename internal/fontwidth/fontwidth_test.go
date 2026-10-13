package fontwidth

import (
	"os/exec"
	"testing"
)

func TestParseFont(t *testing.T) {
	for line, want := range map[string]struct {
		families string
		px       float64
		ok       bool
	}{
		"font pango:Hack, FontAwesome 10":                                {"Hack, FontAwesome", 40.0 / 3, true},
		"  font pango:monospace 12px":                                    {"monospace", 12, true},
		"font -misc-fixed-medium-r-normal--13-120-75-75-C-70-iso10646-1": {},
		"# font pango:Hack 10":                                           {},
	} {
		f, px, ok := parseFont(line)
		if ok != want.ok || f != want.families || px < want.px-0.01 || px > want.px+0.01 {
			t.Errorf("%q: %q %v %v", line, f, px, ok)
		}
	}
}

func TestMeasuresARealFont(t *testing.T) {
	if _, err := exec.LookPath("fc-match"); err != nil {
		t.Skip("no fontconfig")
	}
	m, err := FromI3Config("font pango:monospace 10\n")
	if err != nil {
		t.Skip(err)
	}
	one, ten := m("x", false), m("xxxxxxxxxx", false)
	if one <= 0 || ten < 9*one || ten > 11*one {
		t.Errorf("x = %v, 10x = %v", one, ten)
	}
	if m("●", false) <= 0 {
		t.Error("a character the font lacks still takes room")
	}
}
