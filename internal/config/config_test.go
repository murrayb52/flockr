package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestSidebarCommand(t *testing.T) {
	cases := map[string]string{
		"alacritty":              "alacritty --class flock-sidebar,flock-sidebar -o font.size=9.5 -e flock sidebar",
		"kitty":                  "kitty --class flock-sidebar -o font_size=9.5 -e flock sidebar",
		"ghostty":                "ghostty --class=flock-sidebar --font-size=9.5 -e flock sidebar",
		"wezterm":                "wezterm --config font_size=9.5 start --class flock-sidebar -- flock sidebar",
		"foot":                   "foot --app-id flock-sidebar --font=monospace:size=9.5 flock sidebar",
		"xterm":                  "xterm -class flock-sidebar -fa monospace -fs 9.5 -e flock sidebar",
		"st -c {class} -e {cmd}": "st -c flock-sidebar -e flock sidebar",
		"st -c {class} -f mono:size={font} -e {cmd}": "st -c flock-sidebar -f mono:size=9.5 -e flock sidebar",
	}
	for term, want := range cases {
		got := Config{SidebarTerminal: term}.SidebarCommand("flock-sidebar", "flock sidebar", 9.5)
		if got != want {
			t.Errorf("%s: got %q want %q", term, got, want)
		}
	}
}

func TestOpenCommand(t *testing.T) {
	cases := map[string]string{
		"gnome-terminal":          "gnome-terminal -- '/b/flock' term",
		"/usr/bin/kitty":          "kitty '/b/flock' term",
		"alacritty":               "alacritty -e '/b/flock' term",
		"sakura":                  "sakura -e '/b/flock' term",
		"my-term --run {cmd} --x": "my-term --run '/b/flock' term --x",
	}
	for term, want := range cases {
		if got := (Config{Terminal: term}).OpenCommand("'/b/flock' term"); got != want {
			t.Errorf("%s: got %q want %q", term, got, want)
		}
	}
}

func TestDefaultTerminalPrefersEnv(t *testing.T) {
	t.Setenv("TERMINAL", "foot")
	if got := defaultTerminal(); got != "foot" {
		t.Errorf("got %q", got)
	}
}

func TestRuntimeFallbackIsPerUser(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := RuntimeDir(); !strings.HasSuffix(got, fmt.Sprintf("flock-%d", os.Getuid())) {
		t.Errorf("fallback = %q", got)
	}
}

func TestPrivateDirRefusesOpenDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rt")
	if err := PrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o777)
	if err := PrivateDir(dir); err == nil {
		t.Error("a world-writable runtime dir must be refused")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(t.TempDir(), link)
	if err := PrivateDir(link); err == nil {
		t.Error("a symlinked runtime dir must be refused")
	}
}

func TestWriteFileAtomicReplacesSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "victim")
	os.WriteFile(target, []byte("keep"), 0o600)
	p := filepath.Join(dir, "tmux.conf")
	os.Symlink(target, p)
	if err := WriteFileAtomic(p, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Error("the symlink target must not be written through")
	}
	if fi, _ := os.Lstat(p); fi.Mode()&os.ModeSymlink != 0 {
		t.Error("the symlink should have been replaced by a file")
	}
}
