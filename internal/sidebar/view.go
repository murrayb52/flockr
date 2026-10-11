// Package sidebar is the TUI shown in the flock-sidebar terminal window: the
// focused workspace's windows grouped as i3 lays them out, or an icon rail
// when condensed.
package sidebar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/ui"
)

// RailColumns: at or below this width the sidebar draws the icon rail.
const RailColumns = 12

var (
	c        = func(s string) lipgloss.Color { return lipgloss.Color(s) }
	stGroup  = lipgloss.NewStyle().Foreground(c("#596172"))
	stName   = lipgloss.NewStyle().Foreground(c(ui.ColText))
	stAuto   = lipgloss.NewStyle().Foreground(c("#b4bccb")).Italic(true)
	stSub    = lipgloss.NewStyle().Foreground(c(ui.ColDim))
	stSlot   = lipgloss.NewStyle().Foreground(c(ui.ColFaint))
	stNotes  = lipgloss.NewStyle().Foreground(c(ui.ColBlue))
	stEye    = lipgloss.NewStyle().Foreground(c("#7d8799"))
	stEmpty  = lipgloss.NewStyle().Foreground(c("#9fb8a8")).Italic(true)
	stLegend = lipgloss.NewStyle().Foreground(c(ui.ColDim))
	stButton = lipgloss.NewStyle().Foreground(c(ui.ColText)).Background(c("#3a3f4b")).Bold(true)
)

// Line is one rendered row plus the window it belongs to (0 = none), so
// mouse clicks can be mapped back to windows, and any buttons on it.
type Line struct {
	Text    string
	ConID   int64
	Buttons []Button
}

// Button is a clickable cell range [From, To) that runs a daemon command.
type Button struct {
	From, To int
	Args     []string
}

// At returns the command of the button covering column x, if any.
func (l Line) At(x int) []string {
	for _, b := range l.Buttons {
		if x >= b.From && x < b.To {
			return b.Args
		}
	}
	return nil
}

type Frame struct {
	Width, Height int
	Tick          int
	Now           time.Time
	Home          string
	Menu          bool // the menu is open
}

func tile(w model.Window) string {
	t := ui.Tile(w)
	g := t.Glyph
	if runewidth.StringWidth(g) == 1 {
		g = " " + g + " "
	} else {
		g = g + " "
	}
	return lipgloss.NewStyle().Background(c(t.BG)).Foreground(c(t.FG)).Bold(true).Render(g)
}

func logo(s model.Status, tick int) string {
	g, col := ui.Logo(s, tick)
	if g == "" {
		return " "
	}
	return lipgloss.NewStyle().Foreground(c(col)).Bold(true).Render(g)
}

func trunc(s string, n int) string {
	if n < 1 {
		return ""
	}
	return runewidth.Truncate(s, n, "…")
}

// Render draws the sidebar for the focused workspace, with the menu at the
// foot of the window (or after a blank line when the height is unknown).
func Render(snap model.Snapshot, f Frame) []Line {
	ws, ok := snap.Current()
	if !ok {
		return []Line{{Text: stEmpty.Render(" waiting for i3…")}}
	}
	railMode := f.Width <= RailColumns || snap.Condensed
	var body, foot []Line
	if railMode {
		body = rail(ws, f)
		foot = []Line{railMenu()}
	} else {
		body = expanded(ws, f)
		foot = menu(snap, f)
	}
	if f.Height <= len(foot) {
		return append(append(body, Line{}), foot...)
	}
	room := f.Height - len(foot)
	if len(body) > room {
		body = body[:room]
	}
	for len(body) < room {
		body = append(body, Line{})
	}
	return append(body, foot...)
}

// Local actions the sidebar handles itself rather than sending to the daemon.
const (
	ActMenu       = ":menu"        // open or close the menu
	ActMenuExpand = ":menu-expand" // expand the rail and open the menu
)

// lineBuilder tracks columns so buttons know which cells they cover.
type lineBuilder struct {
	Line
	col int
}

func (b *lineBuilder) text(s string) *lineBuilder {
	b.Text += s
	b.col += lipgloss.Width(s)
	return b
}

func (b *lineBuilder) button(label string, args ...string) *lineBuilder {
	w := lipgloss.Width(label)
	b.Buttons = append(b.Buttons, Button{From: b.col, To: b.col + w, Args: args})
	b.Text += stButton.Render(label)
	b.col += w
	return b
}

// pad fills the label column of a menu row.
func pad(s string, n int) string { return stLegend.Render(fmt.Sprintf(" %-*s", n, s)) }

func rule(width int) Line {
	return Line{Text: stSlot.Render(" " + strings.Repeat("─", max(1, width-2)))}
}

// menu is the ≡ button, and when open the settings and the legend above it.
func menu(snap model.Snapshot, f Frame) []Line {
	if !f.Menu {
		var b lineBuilder
		b.text(" ").button(" ≡ menu ", ActMenu)
		return []Line{b.Line}
	}
	const col = 14
	size := "–"
	if snap.Font > 0 {
		size = strconv.FormatFloat(snap.Font, 'f', -1, 64)
	}
	var text, side, notes, close lineBuilder
	text.text(pad("text size", col)).button(" − ", "sidebar", "font", "-1").
		text(stName.Render(fmt.Sprintf(" %s ", size))).button(" + ", "sidebar", "font", "+1")
	side.text(pad("sidebar", col)).button(" condense ", "sidebar", "condense")
	state := " on "
	if snap.Muted {
		state = " off "
	}
	notes.text(pad("notifications", col)).button(state, "notifications", "toggle")
	close.text(" ").button(" ≡ close ", ActMenu)
	l := func(st model.Status, s string) string { return logo(st, 0) + stLegend.Render(s) }
	return []Line{
		rule(f.Width),
		text.Line, side.Line, notes.Line,
		rule(f.Width),
		{Text: " " + l(model.StatusInput, " needs you  ") + l(model.StatusDone, " done")},
		{Text: " " + l(model.StatusWorking, " working    ") + l(model.StatusIdle, " idle")},
		rule(f.Width),
		close.Line,
	}
}

// railMenu: the rail is too narrow for the menu, so its button expands the
// sidebar and opens the menu there.
func railMenu() Line {
	var b lineBuilder
	b.text(" ").button(" ≡ ", ActMenuExpand)
	return b.Line
}

func expanded(ws model.Workspace, f Frame) []Line {
	var out []Line
	if len(ws.Groups) == 0 {
		return []Line{{}, {Text: stEmpty.Render(" empty workspace")}}
	}
	for gi, g := range ws.Groups {
		if gi > 0 || g.Label != "" {
			out = append(out, Line{})
		}
		if g.Label != "" {
			out = append(out, Line{Text: stGroup.Render(" " + strings.ToUpper(trunc(g.Label, f.Width-2)))})
		}
		for _, w := range g.Windows {
			out = append(out, row(w, f)...)
		}
	}
	return out
}

func row(w model.Window, f Frame) []Line {
	// Every segment carries the row background itself: wrapping the finished
	// line in a background style does not survive the segments' own resets.
	base := lipgloss.NewStyle()
	if w.Focused {
		base = base.Background(c(ui.ColRowBG))
	}
	on := func(st lipgloss.Style) lipgloss.Style { return st.Inherit(base) }
	sp := func(n int) string { return base.Render(strings.Repeat(" ", max(n, 0))) }
	a := ui.AgentOf(w)
	status := ui.StatusOf(a)
	slot := "  "
	if w.Slot > 0 {
		slot = fmt.Sprintf("%2d", w.Slot)
	}
	right := ""
	if done, total := ui.NotesCount(a); total > 0 {
		right = on(stNotes).Render(fmt.Sprintf("[%d/%d]", done, total))
	}
	if w.Visible {
		right += sp(1) + on(stEye).Render("◉")
	} else {
		right += sp(2)
	}
	right += sp(1)
	name, auto := ui.Name(w)
	nameStyle := on(stName)
	if auto {
		nameStyle = on(stAuto)
	}
	prefix := on(stSlot).Render(slot) + sp(1) + tile(w) + sp(1)
	room := f.Width - lipgloss.Width(prefix) - lipgloss.Width(right) - 1
	left := prefix + nameStyle.Render(trunc(name, room))
	line1 := left + sp(f.Width-lipgloss.Width(left)-lipgloss.Width(right)) + right

	lead := sp(6)
	if a != nil {
		g, col := ui.Logo(status, f.Tick)
		lead = sp(4) + on(lipgloss.NewStyle().Foreground(c(col)).Bold(true)).Render(g) + sp(1)
	}
	room = f.Width - lipgloss.Width(lead) - 1
	sub := trunc(ui.Sub(w, f.Now, f.Home), room)
	if a != nil { // keep the elapsed time visible: shorten the activity instead
		el := " · " + ui.Elapsed(f.Now.Sub(a.Since))
		sub = trunc(ui.StateText(a), room-runewidth.StringWidth(el)) + el
	}
	subStyle := on(stSub)
	switch status {
	case model.StatusInput:
		subStyle = subStyle.Foreground(c("#ff8a80"))
	case model.StatusDone:
		subStyle = subStyle.Foreground(c(ui.ColOK))
	}
	line2 := lead + subStyle.Render(sub)
	line2 += sp(f.Width - lipgloss.Width(line2))
	return []Line{{Text: line1, ConID: w.ConID}, {Text: line2, ConID: w.ConID}}
}

func rail(ws model.Workspace, f Frame) []Line {
	out := []Line{{}}
	for gi, g := range ws.Groups {
		if gi > 0 {
			out = append(out, Line{Text: stSlot.Render(" " + strings.Repeat("─", max(1, min(f.Width-2, 5))))})
		}
		for _, w := range g.Windows {
			mark := " "
			if w.Focused {
				mark = lipgloss.NewStyle().Foreground(c(ui.ColOK)).Render("▌")
			} else if w.Visible {
				mark = stSlot.Render("▏")
			}
			line := mark + tile(w) + logo(ui.StatusOf(ui.AgentOf(w)), f.Tick)
			out = append(out, Line{Text: line, ConID: w.ConID}, Line{})
		}
	}
	return out
}
