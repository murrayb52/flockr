package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/murrayb52/flockr/internal/config"
)

// settings are choices made at run time (`flockr notifications`), kept apart
// from the user's config file so flockr never rewrites it.
type settings struct {
	Notifications *bool `json:"notifications,omitempty"`
}

func settingsPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "settings.json") }

// loadSettings returns the saved choices, falling back to the config file's
// notification switch.
func loadSettings(cfg config.Config) settings {
	var s settings
	if b, err := os.ReadFile(settingsPath(cfg)); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.Notifications == nil {
		on := cfg.Notifications
		s.Notifications = &on
	}
	return s
}

// saveSettings writes the current choices (called with d.mu held).
func (d *Daemon) saveSettings() error {
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return err
	}
	on := d.notifyOn
	b, _ := json.Marshal(settings{Notifications: &on})
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

func wsLabelsPath(cfg config.Config) string { return filepath.Join(cfg.DataDir, "workspace-labels.json") }

func loadWSLabels(cfg config.Config) map[int64]string {
	m := map[int64]string{}
	b, err := os.ReadFile(wsLabelsPath(cfg))
	if err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (d *Daemon) saveWSLabels() error {
	if err := os.MkdirAll(d.cfg.DataDir, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(d.wsLabels)
	return config.WriteFileAtomic(wsLabelsPath(d.cfg), b, 0o600)
}

type errUsage string

func (e errUsage) Error() string { return "usage: " + string(e) }
