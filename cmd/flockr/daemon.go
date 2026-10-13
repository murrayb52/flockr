package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/murrayb52/flockr/internal/config"
	"github.com/murrayb52/flockr/internal/daemon"
	"github.com/murrayb52/flockr/internal/fontwidth"
	"github.com/murrayb52/flockr/internal/notify"
	"github.com/murrayb52/flockr/internal/tmux"
	"github.com/murrayb52/flockr/internal/x11"
)

func runDaemon(cfg config.Config) error {
	log.SetFlags(log.Ltime)
	log.SetPrefix("flockr: ")
	l, err := daemon.Listen(cfg.Socket)
	if err != nil {
		return err
	}
	// remove the socket on exit only if it is still ours (a newer daemon may
	// have replaced a stale one at the same path)
	if mine, err := os.Stat(cfg.Socket); err == nil {
		defer func() {
			if now, err := os.Stat(cfg.Socket); err == nil && os.SameFile(mine, now) {
				os.Remove(cfg.Socket)
			}
		}()
	}
	if conf, err := writeTmuxConf(cfg); err == nil {
		(tmux.Client{Socket: cfg.TmuxSocket}).Source(conf)
	}
	deps := daemon.Deps{
		WM:  daemon.I3{},
		Mux: tmux.Client{Socket: cfg.TmuxSocket},
		Bin: binPath(),
	}
	if n, err := notify.New(); err == nil {
		deps.Notifier = n
	} else {
		log.Printf("notifications disabled: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if u, err := x11.New(); err == nil {
		deps.Urgency = u
		go func() { // the X session ended: exit rather than linger into the next login
			<-u.Closed()
			log.Printf("X connection closed; exiting")
			stop()
		}()
	} else {
		log.Printf("urgency hints disabled: %v", err)
	}
	if cfg.TitleCharWidth > 0 {
		deps.Measure = fontwidth.Mono(cfg.TitleCharWidth)
	} else if m, err := daemon.TitleMeasure(); err == nil {
		deps.Measure = m
	} else {
		log.Printf("title font: %v (set title_char_width to right-align directories)", err)
	}
	d := daemon.New(cfg, deps)
	go d.Serve(ctx, l)
	d.Run(ctx)
	return nil
}

// launch starts a shell command detached from flockr (new session, no stdio).
func launch(shellCmd string) error {
	cmd := exec.Command("/bin/sh", "-c", shellCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
