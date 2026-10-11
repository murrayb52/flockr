package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murrayb52/flockr/internal/model"
)

func TestConfigNeverBindsKeys(t *testing.T) {
	c := Config("/home/u/.config/flockr/tmux.conf")
	for _, want := range []string{"set -g prefix None", "unbind-key -T prefix -a", "destroy-unattached on",
		"[flockr:#{session_name}]", "status-position top", "source-file -q /home/u/.config/flockr/tmux.conf"} {
		if !strings.Contains(c, want) {
			t.Errorf("config lacks %q", want)
		}
	}
	if strings.Contains(c, "\nbind-key ") || strings.Contains(c, "\nbind ") {
		t.Error("config must not bind keys")
	}
}

func TestHeaderEscapesUserText(t *testing.T) {
	w := model.Window{App: "term", Term: &model.Term{Cwd: "/x/#tag", Command: "zsh",
		Agent: &model.Agent{Kind: "claude", State: model.Input, Label: `fix #[fg=red] "quotes" 100%`,
			Notes: []model.Note{{Text: "a", Done: true}, {Text: "b"}}}}}
	h1, h2, lines := Header(w, time.Now(), "/home/u", Hints{Label: "Alt+/", Notes: "Alt+."})
	if !strings.Contains(h1, "fix ##[fg=red] \"quotes\" 100%") {
		t.Errorf("label not escaped: %s", h1)
	}
	if !strings.Contains(h1, "/x/##tag") || !strings.Contains(h1, "needs input") {
		t.Errorf("h1 = %s", h1)
	}
	if lines != 2 || !strings.Contains(h2, "notes [1/2]") || !strings.Contains(h2, "Alt+. open") {
		t.Errorf("h2 = %s (%d lines)", h2, lines)
	}
}

func TestHeaderPlainTerminal(t *testing.T) {
	w := model.Window{App: "term", Term: &model.Term{Cwd: "/x/app", Command: "zsh"}}
	h1, h2, lines := Header(w, time.Now(), "", Hints{Label: "Alt+/", Notes: "Alt+."})
	if lines != 1 || h2 != "" || !strings.Contains(h1, "zsh — app") || !strings.Contains(h1, "Alt+/ to name") {
		t.Errorf("h1=%s h2=%s lines=%d", h1, h2, lines)
	}
}

func TestParsePanesKeepsFirstPanePerSession(t *testing.T) {
	out := "a1\t%1\t/x\tzsh\t\t100\na1\t%2\t/y\tvim\t\t101\nb2\t%3\t/z\tclaude\tapi\t102\n"
	got := parsePanes(out)
	if len(got) != 2 || got[0].ID != "%1" || got[1].Label != "api" || got[1].PID != 102 {
		t.Fatalf("got %+v", got)
	}
}

// Integration: a real private tmux server.
func TestRealServer(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sock := fmt.Sprintf("flockrtest%d", os.Getpid())
	c := Client{Socket: sock}
	defer c.cmd("kill-server").Run()
	conf := filepath.Join(t.TempDir(), "tmux.conf")
	// a detached test session must survive, so drop destroy-unattached
	os.WriteFile(conf, []byte(strings.Replace(Config("/nonexistent"), "set -g destroy-unattached on", "", 1)), 0o644)
	if err := c.cmd("-f", conf, "new-session", "-d", "-s", "t1", "-c", "/tmp", "sh").Run(); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("t1", map[string]string{"@flockr_label": "hello #1", "@flockr_h1": "x"}); err != nil {
		t.Fatal(err)
	}
	panes, err := c.ListPanes()
	if err != nil || len(panes) != 1 || panes[0].Label != "hello #1" || panes[0].Cwd != "/tmp" {
		t.Fatalf("panes = %+v, %v", panes, err)
	}
	if _, err := (Client{Socket: sock + "x"}).ListPanes(); err != ErrNoServer {
		t.Errorf("missing server → %v", err)
	}
}
