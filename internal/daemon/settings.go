package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/murrayb52/flock/internal/config"
)

// settings are choices made from the sidebar's menu, kept apart from the
// user's config file so flock never rewrites it.
type settings struct {
	SidebarFont   float64 `json:"sidebar_font_size,omitempty"`
	Notifications *bool   `json:"notifications,omitempty"`
}

func settingsPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "settings.json") }

// loadSettings returns the font size and notification switch: the menu's
// saved choices, else the config file's.
func loadSettings(cfg config.Config) (font float64, notify bool) {
	font, notify = cfg.SidebarFont, cfg.Notifications
	var s settings
	if b, err := os.ReadFile(settingsPath(cfg)); err == nil && json.Unmarshal(b, &s) == nil {
		if s.SidebarFont > 0 {
			font = s.SidebarFont
		}
		if s.Notifications != nil {
			notify = *s.Notifications
		}
	}
	return font, notify
}

// saveSettings writes the current menu choices (called with d.mu held).
func (d *Daemon) saveSettings() error {
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return err
	}
	on := d.notifyOn
	b, _ := json.Marshal(settings{SidebarFont: d.font, Notifications: &on})
	return config.WriteFileAtomic(settingsPath(d.cfg), b, 0o600)
}

// setNotifications switches desktop notifications on or off ("toggle" flips).
func (d *Daemon) setNotifications(v string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch v {
	case "on":
		d.notifyOn = true
	case "off":
		d.notifyOn = false
	case "toggle":
		d.notifyOn = !d.notifyOn
	default:
		return errUsage("notifications on|off|toggle")
	}
	if err := d.saveSettings(); err != nil {
		return err
	}
	d.rebuild()
	return nil
}

type errUsage string

func (e errUsage) Error() string { return "usage: " + string(e) }
