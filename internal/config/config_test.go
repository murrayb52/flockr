package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsFollowXDG(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/42")
	t.Setenv("XDG_DATA_HOME", "/data")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Socket != "/run/user/42/flock/flock.sock" {
		t.Errorf("socket = %q", c.Socket)
	}
	if c.DataDir != "/data/flock" {
		t.Errorf("data dir = %q", c.DataDir)
	}
	if c.SidebarWidth != 280 || c.RailWidth != 64 {
		t.Errorf("widths = %d/%d", c.SidebarWidth, c.RailWidth)
	}
	if c.TmuxSocket != "flock" {
		t.Errorf("tmux socket = %q", c.TmuxSocket)
	}
}

func TestFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "flock"), 0o755)
	os.WriteFile(filepath.Join(dir, "flock", "config.toml"), []byte(`
terminal = "kitty"
sidebar_width = 300
notifications = false
`), 0o644)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Terminal != "kitty" || c.SidebarWidth != 300 || c.Notifications {
		t.Errorf("got %+v", c)
	}
}

func TestBadFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "flock"), 0o755)
	os.WriteFile(filepath.Join(dir, "flock", "config.toml"), []byte(`terminal = `), 0o644)
	if _, err := Load(); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestTerminalCommand(t *testing.T) {
	cases := map[string]string{
		"alacritty":              "alacritty --class flock-sidebar,flock-sidebar -e flock sidebar",
		"kitty":                  "kitty --class flock-sidebar -e flock sidebar",
		"ghostty":                "ghostty --class=flock-sidebar -e flock sidebar",
		"wezterm":                "wezterm start --class flock-sidebar -- flock sidebar",
		"foot":                   "foot --app-id flock-sidebar flock sidebar",
		"xterm":                  "xterm -class flock-sidebar -e flock sidebar",
		"st -c {class} -e {cmd}": "st -c flock-sidebar -e flock sidebar",
	}
	for term, want := range cases {
		got := Config{Terminal: term}.TerminalCommand("flock-sidebar", "flock sidebar")
		if got != want {
			t.Errorf("%s: got %q want %q", term, got, want)
		}
	}
}
