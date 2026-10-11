// Package config loads flock's optional config file and resolves its paths.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
	c.Socket = filepath.Join(RuntimeDir(), "flock.sock")
	c.DataDir = filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "flock")
	return c, nil
}

// RuntimeDir is flock's private runtime directory: $XDG_RUNTIME_DIR/flock, or
// /tmp/flock-<uid> when that is unset (never a directory shared between users).
func RuntimeDir() string {
	if r := os.Getenv("XDG_RUNTIME_DIR"); r != "" {
		return filepath.Join(r, "flock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("flock-%d", os.Getuid()))
}

// PrivateDir creates dir (0700) and refuses to use it unless it is a real
// directory owned by the current user and inaccessible to others, so another
// local user cannot pre-create it to plant a socket or config.
func PrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return CheckPrivateDir(dir)
}

// CheckPrivateDir is PrivateDir without creating anything: clients use it
// before trusting a socket in dir.
func CheckPrivateDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !fi.IsDir():
		return fmt.Errorf("%s is not a directory", dir)
	case ok && int(st.Uid) != os.Getuid():
		return fmt.Errorf("%s is owned by another user", dir)
	case fi.Mode().Perm()&0o077 != 0:
		return fmt.Errorf("%s is accessible to other users (mode %o); fix with chmod 700", dir, fi.Mode().Perm())
	}
	return nil
}

// WriteFileAtomic writes data via a fresh temporary file renamed into place,
// so a pre-existing file or symlink at path is replaced, never followed.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
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
