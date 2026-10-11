package notesui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/murrayb52/flockr/internal/protocol"
	"github.com/murrayb52/flockr/internal/ui"
)

var (
	stSel     = lipgloss.NewStyle().Background(lipgloss.Color("#22352b"))
	stEdit    = lipgloss.NewStyle().Background(lipgloss.Color("#1d2b25")).Foreground(lipgloss.Color(ui.ColText))
	stDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5f6b66")).Strikethrough(true)
	stTick    = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.ColOK))
	stBox     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8799"))
	stAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7a73")).Italic(true)
	stHelp    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7a73"))
	stHelpKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#b4c8bd"))
	stCursor  = lipgloss.NewStyle().Background(lipgloss.Color("#d0d4dc"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.ColRed))
)

type app struct {
	socket, pane string
	ed           *Editor
	width        int
	err          string
}

type savedMsg struct{ err error }

func (a *app) save() tea.Cmd {
	notes := a.ed.Clean()
	return func() tea.Msg {
		_, err := protocol.Call(a.socket, protocol.Request{Op: protocol.OpSetNotes, Pane: a.pane, Notes: notes}, time.Second)
		return savedMsg{err}
	}
}

func (a *app) Init() tea.Cmd { return nil }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
	case savedMsg:
		a.err = ""
		if m.err != nil {
			a.err = "not saved: " + m.err.Error()
		}
	case tea.KeyMsg:
		if m.String() == "ctrl+c" || m.String() == "alt+." {
			return a, tea.Sequence(a.save(), tea.Quit)
		}
		before := fmt.Sprint(a.ed.Notes)
		if a.ed.Key(m.String()) {
			return a, tea.Sequence(a.save(), tea.Quit)
		}
		if fmt.Sprint(a.ed.Notes) != before {
			return a, a.save() // save as you type: however the popup closes, nothing is lost
		}
	}
	return a, nil
}

func (a *app) View() string {
	w := a.width
	if w <= 0 {
		w = 80
	}
	var b strings.Builder
	for i, n := range a.ed.Notes {
		box, text := stBox.Render("☐"), n.Text
		if n.Done {
			box, text = stTick.Render("☑"), stDone.Render(n.Text)
		}
		line := " " + box + " " + text
		if i == a.ed.Sel && a.ed.Editing {
			line = " " + box + " " + n.Text + stCursor.Render(" ")
			line = stEdit.Width(w).Render(line)
		} else if i == a.ed.Sel {
			line = stSel.Width(w).Render(line)
		}
		b.WriteString(line + "\n")
	}
	add := " + new note"
	if len(a.ed.Notes) == 0 {
		add = " + new note — just start typing"
	}
	if a.ed.Sel == len(a.ed.Notes) {
		b.WriteString(stSel.Width(w).Render(stAdd.Render(add)) + "\n")
	} else {
		b.WriteString(stAdd.Render(add) + "\n")
	}
	k := stHelpKey.Render
	help := k("Tab") + "/" + k("↓") + " next · " + k("Space") + " tick · type or " + k("Enter") + " to edit · " + k("Del") + " remove · " + k("Esc") + " close"
	if a.ed.Editing {
		help = "editing · " + k("Enter") + "/" + k("Esc") + " done · " + k("Tab") + " done + next"
	}
	b.WriteString("\n" + stHelp.Render(help))
	if a.err != "" {
		b.WriteString("\n" + stErr.Render(a.err))
	}
	return b.String()
}

// Run loads the pane's notes from the daemon and edits them until closed.
func Run(socket, pane string) error {
	resp, err := protocol.Call(socket, protocol.Request{Op: protocol.OpGetNotes, Pane: pane}, time.Second)
	if err != nil {
		return err
	}
	a := &app{socket: socket, pane: pane, ed: NewEditor(resp.Notes)}
	_, err = tea.NewProgram(a).Run()
	return err
}
