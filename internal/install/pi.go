package install

import (
	"os"
	"path/filepath"

	"github.com/murrayb52/flock/integrations"
)

// PiPath is where Pi loads personal extensions from.
func PiPath() string {
	dir := os.Getenv("PI_CODING_AGENT_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".pi", "agent")
	}
	return filepath.Join(dir, "extensions", "flock.ts")
}

// Pi writes (or removes) the flock extension for Pi.
func Pi(install bool) (string, error) {
	p := PiPath()
	if !install {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			err = nil
		}
		return p, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return p, err
	}
	return p, os.WriteFile(p, integrations.PiExtension, 0o644)
}
