package daemon

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/murrayb52/flockr/internal/fontwidth"
	"github.com/murrayb52/flockr/internal/model"
)

// plain strips the markup from a title.
func plain(f string) string {
	var b strings.Builder
	in := false
	for _, r := range f {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&quot;", `"`, "&#37;", "%", "&#39;", "'").Replace(b.String())
}

func agentWin(num int, label, activity string) model.Window {
	return model.Window{App: "term", Num: num, TitleWidth: 810, Term: &model.Term{Cwd: "/home/u/code/api", Command: "claude",
		Agent: &model.Agent{Kind: "claude", State: model.Working, Label: label, Activity: activity}}}
}

func TestTitleLayout(t *testing.T) {
	m := fontwidth.Mono(8)
	w := agentWin(2, "api", "Bash npm test")
	// (810 - 10)px / 8 = 100 columns; activity starts at column 20 + 2
	got := plain(titleParts(w, "/home/u", 0).render(w.TitleWidth, m, 20*8))
	if utf8.RuneCountInString(got) != 100 {
		t.Errorf("title is %d columns, want 100: %q", utf8.RuneCountInString(got), got)
	}
	if r := []rune(got); !strings.HasPrefix(got, "✻ 2 ⠋ api ") || string(r[22:35]) != "Bash npm test" ||
		!strings.HasSuffix(got, " ~/code/api") {
		t.Errorf("title = %q", got)
	}
}

func TestActivityColumnIsSharedAcrossAStack(t *testing.T) {
	d, _, _, _, _ := fixture(t)
	d.d.Measure = fontwidth.Mono(8)
	g := model.Group{Windows: []model.Window{agentWin(1, "api", ""), agentWin(2, "a much longer label", "")}}
	if col := d.titleColumn(g); col != float64(utf8.RuneCountInString("✻ 2 ⠋ a much longer label"))*8 {
		t.Errorf("col = %v", col)
	}
	g.Windows[1].Term.Agent.Label = strings.Repeat("x", 200)
	if col := d.titleColumn(g); col != 400 {
		t.Errorf("col is capped half way across, got %v", col)
	}
}

func TestTitleTrimsAndEscapes(t *testing.T) {
	m := fontwidth.Mono(8)
	w := agentWin(1, `<b> & "100%" `+strings.Repeat("x", 40), strings.Repeat("y", 80))
	w.TitleWidth = 410 // 50 columns
	p := titleParts(w, "/home/u", 0)
	f := p.render(w.TitleWidth, m, 30*8)
	if !strings.Contains(f, "&lt;b&gt; &amp; &quot;100&#37;&quot;") || !strings.HasSuffix(plain(f), "  ~/code/api") {
		t.Errorf("title = %s", f)
	}
	if n := utf8.RuneCountInString(plain(f)); n > 50 {
		t.Errorf("title is %d columns, more than the bar's 50: %q", n, plain(f))
	}
	if q := i3Quote(`a "b" \c`); q != `a \"b\" \\c` {
		t.Errorf("i3Quote = %s", q)
	}
}

func TestTitleWithoutMeasure(t *testing.T) {
	w := model.Window{App: "term", Num: 3, Term: &model.Term{Cwd: "/srv", Command: "zsh", Label: "dev"}}
	if got := plain(titleParts(w, "", 0).render(0, nil, 0)); got != "›_ 3 dev     /srv" {
		t.Errorf("title = %q", got)
	}
}

func TestWorkingTitlesSpin(t *testing.T) {
	d, wm, _, _, _ := fixture(t)
	d.HandleEvent(ev("prompt", "%1", "a"))
	before := len(wm.took("[con_id=11] title_format"))
	d.animate()
	d.animate()
	got := wm.took("[con_id=11] title_format")
	if len(got) != before+2 || !strings.Contains(got[len(got)-1], "⠹") {
		t.Errorf("titles = %v", got[before:])
	}
	if n := len(wm.took("[con_id=12] title_format")); n != 1 {
		t.Errorf("an idle terminal's title is left alone, sent %d times", n)
	}
}
