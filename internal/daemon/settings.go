package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/murrayb52/flockr/internal/config"
)

// settings are choices made from the sidebar's menu, kept apart from the
// user's config file so flockr never rewrites it.
type settings struct {
	SidebarFont   float64 `json:"sidebar_font_size,omitempty"`
	Notifications *bool   `json:"notifications,omitempty"`
	TermsOnly     bool    `json:"terms_only,omitempty"` // sidebar lists only flockr terminals
}

func settingsPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "settings.json") }

// loadSettings returns the menu's saved choices, falling back to the config
// file's font size and notification switch.
func loadSettings(cfg config.Config) settings {
	var s settings
	if b, err := os.ReadFile(settingsPath(cfg)); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.SidebarFont <= 0 {
		s.SidebarFont = cfg.SidebarFont
	}
	if s.Notifications == nil {
		on := cfg.Notifications
		s.Notifications = &on
	}
	return s
}

// saveSettings writes the current menu choices (called with d.mu held).
func (d *Daemon) saveSettings() error {
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return err
	}
	on := d.notifyOn
	b, _ := json.Marshal(settings{SidebarFont: d.font, Notifications: &on, TermsOnly: d.termsOnly})
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

// setFilter chooses what the sidebar lists: "all" windows, flockr "terms"
// only, or "toggle".
func (d *Daemon) setFilter(v string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch v {
	case "all":
		d.termsOnly = false
	case "terms":
		d.termsOnly = true
	case "toggle":
		d.termsOnly = !d.termsOnly
	default:
		return errUsage("sidebar filter all|terms|toggle")
	}
	if err := d.saveSettings(); err != nil {
		return err
	}
	d.rebuild()
	return nil
}

type errUsage string

func (e errUsage) Error() string { return "usage: " + string(e) }
