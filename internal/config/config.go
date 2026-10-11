// Package config loads flock's optional config file and resolves its paths.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	// Terminal used to open the sidebar: a known name (alacritty, kitty, ghostty,
	// wezterm, foot, xterm) or a template containing {class} and {cmd}.
	Terminal      string `toml:"terminal"`
	SidebarWidth  int    `toml:"sidebar_width"` // pixels, expanded
	RailWidth     int    `toml:"rail_width"`    // pixels, condensed
	Notifications bool   `toml:"notifications"`
	Shell         string `toml:"shell"` // shell started by `flock term`; defaults to $SHELL
	// Key names shown in terminal headers; match them to your i3 bindings.
	LabelKey string `toml:"label_key"`
	NotesKey string `toml:"notes_key"`

	Socket     string `toml:"-"`
	DataDir    string `toml:"-"`
	TmuxSocket string `toml:"-"`
}

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

// Path of the config file (it does not have to exist).
func Path() string {
	return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "flock", "config.toml")
}

func Load() (Config, error) {
	c := Config{
		Terminal:      detectTerminal(),
		SidebarWidth:  280,
		RailWidth:     64,
		Notifications: true,
		Shell:         os.Getenv("SHELL"),
		LabelKey:      "Alt+/",
		NotesKey:      "Alt+.",
		TmuxSocket:    "flock",
	}
	if c.Shell == "" {
		c.Shell = "/bin/sh"
	}
	if b, err := os.ReadFile(Path()); err == nil {
		if err := toml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("%s: %w", Path(), err)
		}
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		runtime = os.TempDir()
	}
	c.Socket = filepath.Join(runtime, "flock", "flock.sock")
	c.DataDir = filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "flock")
	return c, nil
}

var templates = map[string]string{
	"alacritty": "alacritty --class {class},{class} -e {cmd}",
	"kitty":     "kitty --class {class} -e {cmd}",
	"ghostty":   "ghostty --class={class} -e {cmd}",
	"wezterm":   "wezterm start --class {class} -- {cmd}",
	"foot":      "foot --app-id {class} {cmd}",
	"xterm":     "xterm -class {class} -e {cmd}",
}

func detectTerminal() string {
	for _, name := range []string{"alacritty", "kitty", "ghostty", "wezterm", "foot", "xterm"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "xterm"
}

// TerminalCommand returns a shell command that opens the configured terminal
// with the given WM class running cmd.
func (c Config) TerminalCommand(class, cmd string) string {
	tpl, ok := templates[c.Terminal]
	if !ok {
		tpl = c.Terminal
	}
	return strings.NewReplacer("{class}", class, "{cmd}", cmd).Replace(tpl)
}
