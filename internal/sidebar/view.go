// Package sidebar is the TUI shown in the flock-sidebar terminal window: the
// focused workspace's windows grouped as i3 lays them out, or an icon rail
// when condensed.
package sidebar

import (
	"fmt"
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
	stFocus  = lipgloss.NewStyle().Background(c(ui.ColRowBG))
	stEmpty  = lipgloss.NewStyle().Foreground(c("#9fb8a8")).Italic(true)
	stLegend = lipgloss.NewStyle().Foreground(c(ui.ColDim))
)

// Line is one rendered row plus the window it belongs to (0 = none), so
// mouse clicks can be mapped back to windows.
type Line struct {
	Text  string
	ConID int64
}

type Frame struct {
	Width, Height int
	Tick          int
	Now           time.Time
	Home          string
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

// pad right-aligns right within width after left, truncating left if needed.
func pad(left, right string, width int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw > width {
		return left
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

func trunc(s string, n int) string {
	if n < 1 {
		return ""
	}
	return runewidth.Truncate(s, n, "…")
}

// Render draws the sidebar for the focused workspace.
func Render(snap model.Snapshot, f Frame) []Line {
	ws, ok := snap.Current()
	if !ok {
		return []Line{{Text: stEmpty.Render(" waiting for i3…")}}
	}
	if f.Width <= RailColumns || snap.Condensed {
		return rail(ws, f)
	}
	return expanded(ws, f)
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
	out = append(out, Line{})
	out = append(out, legend(f)...)
	return out
}

func row(w model.Window, f Frame) []Line {
	a := ui.AgentOf(w)
	status := ui.StatusOf(a)
	slot := "  "
	if w.Slot > 0 {
		slot = fmt.Sprintf("%2d", w.Slot)
	}
	right := ""
	if done, total := ui.NotesCount(a); total > 0 {
		right = stNotes.Render(fmt.Sprintf("[%d/%d]", done, total))
	}
	if w.Visible {
		right += " " + stEye.Render("◉")
	} else {
		right += "  "
	}
	right += " "
	name, auto := ui.Name(w)
	nameStyle := stName
	if auto {
		nameStyle = stAuto
	}
	prefix := stSlot.Render(slot) + " " + tile(w) + " "
	room := f.Width - lipgloss.Width(prefix) - lipgloss.Width(right) - 1
	line1 := pad(prefix+nameStyle.Render(trunc(name, room)), right, f.Width)

	lead := "      "
	if a != nil {
		lead = "    " + logo(status, f.Tick) + " "
	}
	room = f.Width - lipgloss.Width(lead) - 1
	sub := trunc(ui.Sub(w, f.Now, f.Home), room)
	if a != nil { // keep the elapsed time visible: shorten the activity instead
		el := " · " + ui.Elapsed(f.Now.Sub(a.Since))
		sub = trunc(ui.StateText(a), room-runewidth.StringWidth(el)) + el
	}
	subStyle := stSub
	switch status {
	case model.StatusInput:
		subStyle = stSub.Foreground(c("#ff8a80"))
	case model.StatusDone:
		subStyle = stSub.Foreground(c(ui.ColOK))
	}
	line2 := lead + subStyle.Render(sub)
	if w.Focused {
		line1 = stFocus.Width(f.Width).Render(line1)
		line2 = stFocus.Width(f.Width).Render(line2)
	}
	return []Line{{Text: line1, ConID: w.ConID}, {Text: line2, ConID: w.ConID}}
}

func legend(f Frame) []Line {
	l := func(s model.Status, text string) string { return logo(s, f.Tick) + stLegend.Render(text) }
	return []Line{
		{Text: " " + l(model.StatusInput, " needs you  ") + l(model.StatusDone, " done")},
		{Text: " " + l(model.StatusWorking, " working    ") + l(model.StatusIdle, " idle")},
	}
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
			if w.Focused {
				line = stFocus.Width(f.Width).Render(line)
			}
			out = append(out, Line{Text: line, ConID: w.ConID}, Line{})
		}
	}
	return out
}
