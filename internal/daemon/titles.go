package daemon

import (
	"fmt"
	"log"
	"strings"

	"github.com/murrayb52/flockr/internal/fontwidth"
	"github.com/murrayb52/flockr/internal/model"
	"github.com/murrayb52/flockr/internal/ui"
)

// titlePad is the room i3 leaves around a title's text, in pixels, plus a
// little for glyphs drawn from fallback fonts.
const titlePad = 10

// syncTitles gives every flockr terminal its i3 title bar: logo, number and
// state glyph, then its name, what the agent is doing and, at the right edge,
// its working directory. The agent's activity starts at the same place in
// every title of a stack, so they read as a column.
func (d *Daemon) syncTitles() {
	seen := map[int64]bool{}
	var cmds []string
	for _, ws := range d.snap.Workspaces {
		for _, g := range ws.Groups {
			col := d.titleColumn(g)
			for _, w := range g.Windows {
				if w.Term == nil {
					continue
				}
				seen[w.ConID] = true
				// The title bar is the header, so a terminal outside a stack
				// needs i3's normal border to have one. Only once: a later
				// choice of border for the window is the user's.
				if !d.bordered[w.ConID] {
					d.bordered[w.ConID] = true
					if w.Border != "normal" {
						cmds = append(cmds, borderCmd(w))
					}
				}
				f := titleParts(w, d.home, d.frame).render(w.TitleWidth, d.d.Measure, col)
				// A change of the window's own title can make i3 rerun a
				// for_window title_format rule over ours, so send it again then.
				key := f + "\x00" + w.Title
				if d.titles[w.ConID] == key {
					continue
				}
				d.titles[w.ConID] = key
				cmds = append(cmds, fmt.Sprintf(`[con_id=%d] title_format "%s"`, w.ConID, i3Quote(f)))
			}
		}
	}
	for id := range d.titles {
		if !seen[id] {
			delete(d.titles, id)
		}
	}
	for id := range d.bordered {
		if !seen[id] {
			delete(d.bordered, id)
		}
	}
	if len(cmds) == 0 {
		return
	}
	if err := d.d.WM.Command(cmds...); err != nil {
		log.Printf("i3 titles: %v", err)
	}
}

// borderCmd gives a window i3's normal border (with a title bar), keeping the
// width of the border it has.
func borderCmd(w model.Window) string {
	if w.BorderWidth > 0 {
		return fmt.Sprintf("[con_id=%d] border normal %d", w.ConID, w.BorderWidth)
	}
	return fmt.Sprintf("[con_id=%d] border normal", w.ConID)
}

// animate advances the spinner of working agents' titles (as the bar does).
func (d *Daemon) animate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.windows() {
		if ui.StatusOf(ui.AgentOf(w)) == model.StatusWorking {
			d.frame++
			d.syncTitles()
			return
		}
	}
}

// titleColumn is where the activity starts in the group's titles: past the
// longest name, but no further than half way across.
func (d *Daemon) titleColumn(g model.Group) float64 {
	m := d.d.Measure
	if m == nil {
		return 0
	}
	var col float64
	for _, w := range g.Windows {
		if w.Term == nil || w.TitleWidth == 0 {
			continue
		}
		p := titleParts(w, d.home, d.frame)
		col = max(col, min(m(p.leadText(), false)+m(p.name, true), float64(w.TitleWidth-titlePad)/2))
	}
	return col
}

type title struct {
	lead   []span
	name   string
	status span
	cwd    string
}

// titleParts splits a title up; frame animates a working agent's spinner.
func titleParts(w model.Window, home string, frame int) title {
	t := ui.Tile(w)
	logoCol := t.BG
	if t.Glyph == "›_" {
		logoCol = t.FG // a shell's tile is dark: its glyph colour reads better
	}
	p := title{lead: []span{{t.Glyph, logoCol}}, cwd: ui.Home(w.Term.Cwd, home)}
	if w.Num > 0 {
		p.lead = append(p.lead, span{fmt.Sprint(w.Num), ""})
	}
	if a := ui.AgentOf(w); a != nil {
		s := ui.StatusOf(a)
		glyph, col := ui.Logo(s, frame)
		p.lead = append(p.lead, span{glyph, col})
		p.status = span{ui.StateText(a), col}
	}
	p.name, _ = ui.Name(w)
	return p
}

func (p title) leadText() string {
	var b strings.Builder
	for _, s := range p.lead {
		b.WriteString(s.text + " ")
	}
	return b.String()
}

// render lays the title out as Pango markup. Given a measure of the title font
// and the bar's width, the activity starts at col and the directory ends at
// the right edge; the activity, then the name, are trimmed to make room.
func (p title) render(width int, m fontwidth.Measure, col float64) string {
	gap1, gap2 := "  ", "   "
	if m != nil && width > 0 {
		space := m(" ", false)
		avail := float64(width - titlePad)
		lead := m(p.leadText(), false)
		col = max(col, lead+m("…", true)*8)
		p.name = fit(p.name, col-lead, true, m)
		statusX := col + 2*space
		gap1 = spaces(statusX-lead-m(p.name, true), space)
		p.status.text = fit(p.status.text, avail-statusX-m(p.cwd, false)-2*space, false, m)
		gap2 = spaces(avail-statusX-m(p.status.text, false)-m(p.cwd, false), space)
	}
	var b strings.Builder
	for _, s := range p.lead {
		b.WriteString(s.markup() + " ")
	}
	fmt.Fprintf(&b, "<b>%s</b>%s%s%s<span foreground='%s'>%s</span>", markup(p.name), gap1, p.status.markup(), gap2, ui.ColDim, markup(p.cwd))
	return b.String()
}

// fit trims s with an ellipsis until it is at most room pixels wide.
func fit(s string, room float64, bold bool, m fontwidth.Measure) string {
	for r := []rune(s); len(r) > 0 && m(s, bold) > room; {
		r = r[:len(r)-1]
		s = string(r) + "…"
		if len(r) == 0 {
			return ""
		}
	}
	return s
}

// spaces is the run of spaces closest to px wide (at least two).
func spaces(px, space float64) string {
	return strings.Repeat(" ", max(2, int(px/space+0.5)))
}

type span struct{ text, col string } // col "" keeps i3's own text colour

func (s span) markup() string {
	if s.col == "" || s.text == "" {
		return markup(s.text)
	}
	return fmt.Sprintf("<span foreground='%s'>%s</span>", s.col, markup(s.text))
}

// markup escapes text for Pango. "%" becomes an entity so i3 cannot mistake
// user text for a placeholder such as %title.
func markup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "&#39;", `"`, "&quot;", "%", "&#37;").Replace(s)
}

// i3Quote escapes s for a double-quoted i3 command argument.
func i3Quote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
