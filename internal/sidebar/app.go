package sidebar

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/murrayb52/flock/internal/model"
	"github.com/murrayb52/flock/internal/protocol"
)

type (
	snapMsg  model.Snapshot
	lostMsg  struct{}
	tickMsg  struct{}
	retryMsg struct{}
)

type app struct {
	socket string
	snap   *model.Snapshot
	ch     <-chan model.Snapshot
	frame  Frame
	lines  []Line
}

func (a *app) connect() tea.Msg {
	ch, _, err := protocol.Subscribe(a.socket)
	if err != nil {
		return lostMsg{}
	}
	a.ch = ch
	return a.wait()
}

func (a *app) wait() tea.Msg {
	s, ok := <-a.ch
	if !ok {
		return lostMsg{}
	}
	return snapMsg(s)
}

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (a *app) Init() tea.Cmd { return tea.Batch(a.connect, tick()) }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.frame.Width, a.frame.Height = m.Width, m.Height
	case snapMsg:
		s := model.Snapshot(m)
		a.snap = &s
		return a, a.wait
	case lostMsg:
		a.snap = nil
		return a, tea.Tick(time.Second, func(time.Time) tea.Msg { return retryMsg{} })
	case retryMsg:
		return a, a.connect
	case tickMsg:
		a.frame.Tick++
		return a, tick()
	case tea.MouseMsg:
		if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft && m.Y < len(a.lines) {
			if con := a.lines[m.Y].ConID; con != 0 {
				go protocol.Call(a.socket, protocol.Request{Op: protocol.OpCommand, Args: []string{"jump", strconv.FormatInt(con, 10)}}, time.Second)
			}
		}
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a *app) View() string {
	if a.snap == nil {
		a.lines = nil
		return "\n " + stEmpty.Render("waiting for the flock daemon…")
	}
	a.frame.Now = time.Now()
	a.lines = Render(*a.snap, a.frame)
	if a.frame.Height > 0 && len(a.lines) > a.frame.Height {
		a.lines = a.lines[:a.frame.Height]
	}
	out := make([]string, len(a.lines))
	for i, l := range a.lines {
		out[i] = l.Text
	}
	return strings.Join(out, "\n")
}

// Run shows the sidebar until the window closes.
func Run(socket string) error {
	home, _ := os.UserHomeDir()
	a := &app{socket: socket, frame: Frame{Home: home}}
	_, err := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
