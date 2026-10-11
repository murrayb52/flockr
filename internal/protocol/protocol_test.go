package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallRefusesUntrustedSocketDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rt")
	os.Mkdir(dir, 0o777)
	os.Chmod(dir, 0o777)
	_, err := Call(filepath.Join(dir, "flockr.sock"), Request{Op: OpCommand}, time.Second)
	if !errors.Is(err, ErrUntrusted) {
		t.Fatalf("err = %v, want ErrUntrusted", err)
	}
}

func TestCallWithoutDaemon(t *testing.T) {
	_, err := Call(filepath.Join(t.TempDir(), "missing", "flockr.sock"), Request{Op: OpCommand}, time.Second)
	if !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("missing dir: %v", err)
	}
	private := t.TempDir()
	os.Chmod(private, 0o700)
	_, err = Call(filepath.Join(private, "flockr.sock"), Request{Op: OpCommand}, time.Second)
	if !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("private dir, no socket: %v", err)
	}
}
