package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jlaska/bbroker/internal/warden"
)

func main() {
	addr := flag.String("addr", ":4545", "warden listen address")
	cdpPort := flag.Int("cdp-port", 9222, "Chrome CDP port")
	idleTimeout := flag.Duration("idle-timeout", 5*time.Minute, "idle timeout before pod self-terminates")
	sessionTimeout := flag.Duration("session-timeout", 30*time.Minute, "max session duration")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	term, err := warden.NewPodTerminator()
	if err != nil {
		slog.Error("init pod terminator", "err", err)
		// Outside k8s (local dev), log and continue without self-term.
		term = nil
	}

	var terminator warden.Terminator
	if term != nil {
		terminator = term
	} else {
		terminator = &logTerminator{}
	}

	d := warden.New(
		fmt.Sprintf("localhost:%d", *cdpPort),
		*idleTimeout,
		*sessionTimeout,
		terminator,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := d.Run(ctx, *addr); err != nil {
		slog.Error("warden exited", "err", err)
		os.Exit(1)
	}
}

type logTerminator struct{}

func (l *logTerminator) Terminate(_ context.Context) error {
	slog.Warn("self-terminate called (no-op outside k8s)")
	return nil
}
