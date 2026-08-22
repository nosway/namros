package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/gateway"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, cfg, logger); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("gateway stopped", "error", err)
		os.Exit(1)
	}

	shutdownDelay := 100 * time.Millisecond
	time.Sleep(shutdownDelay)
}
