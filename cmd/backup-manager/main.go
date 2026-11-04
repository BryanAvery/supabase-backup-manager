package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/server"
)

const defaultListen = "127.0.0.1:8765"

// validateListen enforces a numeric loopback address, not a hostname that may
// resolve differently over time. Remote listeners need a later security review.
func validateListen(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listen address must use a numeric loopback IP")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("listen port must be in the range 1-65535")
	}
	return nil
}

func run() error {
	listen := flag.String("listen", defaultListen, "numeric loopback IP and TCP port")
	flag.Parse()
	if err := validateListen(*listen); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("start loopback listener: %w", err)
	}
	defer listener.Close()

	service := &http.Server{
		Handler:           server.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveResult := make(chan error, 1)
	go func() { serveResult <- service.Serve(listener) }()
	slog.Info("M1 development health service started", "listen", listener.Addr().String(), "version", server.Version)

	select {
	case err = <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve health endpoint: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown health endpoint: %w", err)
		}
		return nil
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}
