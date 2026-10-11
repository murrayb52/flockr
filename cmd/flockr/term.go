package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/tmux"
)

func tmuxConfig() string { return tmux.Config(userTmuxConf()) }

// writeTmuxConf writes the generated config next to the socket and returns its path.
func writeTmuxConf(cfg config.Config) (string, error) {
	path := filepath.Join(filepath.Dir(cfg.Socket), "tmux.conf")
	if err := config.PrivateDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	return path, config.WriteFileAtomic(path, []byte(tmuxConfig()), 0o600)
}

func inFlockrTmux(cfg config.Config) bool {
	sock := strings.SplitN(os.Getenv("TMUX"), ",", 2)[0]
	return sock != "" && filepath.Base(sock) == cfg.TmuxSocket
}

// runTerm replaces this process with a new session on flockr's tmux server.
// Started without a terminal (from an i3 binding), it first opens the
// configured terminal running itself, by absolute path so that terminals
// started with a different PATH (gnome-terminal's server) still find it.
func runTerm(cfg config.Config, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) && !inFlockrTmux(cfg) {
		cmd := shellQuote(binPath()) + " term"
		if len(args) > 0 {
			cmd += " --"
			for _, a := range args {
				cmd += " " + shellQuote(a)
			}
		}
		return launch(cfg.OpenCommand(cmd))
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux is required for flockr terminals: %w", err)
	}
	if inFlockrTmux(cfg) { // already inside one: just run the command/shell
		if len(args) == 0 {
			args = []string{cfg.Shell}
		}
		bin, err := exec.LookPath(args[0])
		if err != nil {
			return err
		}
		return syscall.Exec(bin, args, os.Environ())
	}
	conf, err := writeTmuxConf(cfg)
	if err != nil {
		return err
	}
	b := make([]byte, 4)
	rand.Read(b)
	session := hex.EncodeToString(b)
	cwd, _ := os.Getwd()
	argv := []string{"tmux", "-L", cfg.TmuxSocket, "-f", conf, "new-session", "-s", session, "-c", cwd,
		"-e", "FLOCKR_SOCKET=" + cfg.Socket, "-e", "FLOCKR_TMUX=" + cfg.TmuxSocket}
	argv = append(argv, args...)
	env := os.Environ()
	if os.Getenv("SHELL") == "" {
		env = append(env, "SHELL="+cfg.Shell)
	}
	// a server started by an older flockr keeps its old config: reload it
	(tmux.Client{Socket: cfg.TmuxSocket}).Source(conf)
	return syscall.Exec(tmuxBin, argv, withoutTmux(env))
}

func withoutTmux(env []string) []string {
	out := env[:0:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "TMUX=") && !strings.HasPrefix(e, "TMUX_PANE=") {
			out = append(out, e)
		}
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
