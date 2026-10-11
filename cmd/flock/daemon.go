package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/murrayb52/flock/internal/config"
	"github.com/murrayb52/flock/internal/daemon"
	"github.com/murrayb52/flock/internal/notify"
	"github.com/murrayb52/flock/internal/tmux"
	"github.com/murrayb52/flock/internal/x11"
)

func runDaemon(cfg config.Config) error {
	log.SetFlags(log.Ltime)
	log.SetPrefix("flock: ")
	l, err := daemon.Listen(cfg.Socket)
	if err != nil {
		return err
	}
	defer os.Remove(cfg.Socket)
	if conf, err := writeTmuxConf(cfg); err == nil {
		(tmux.Client{Socket: cfg.TmuxSocket}).Source(conf)
	}
	deps := daemon.Deps{
		WM:     daemon.I3{},
		Mux:    tmux.Client{Socket: cfg.TmuxSocket},
		Bin:    binPath(),
		Launch: launch,
	}
	if n, err := notify.New(); err == nil {
		deps.Notifier = n
	} else {
		log.Printf("notifications disabled: %v", err)
	}
	if u, err := x11.New(); err == nil {
		deps.Urgency = u
	} else {
		log.Printf("urgency hints disabled: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	d := daemon.New(cfg, deps)
	go d.Serve(ctx, l)
	d.Run(ctx)
	return nil
}

// launch starts a shell command detached from the daemon (new session, no stdio).
func launch(shellCmd string) error {
	cmd := exec.Command("/bin/sh", "-c", shellCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
