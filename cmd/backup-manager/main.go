package main

import (
	"context"
	"crypto/tls"
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
	o := serviceOptions{}
	flag.StringVar(&o.listen, "listen", defaultListen, "numeric loopback IP and TCP port")
	flag.StringVar(&o.cataloguePath, "catalogue", "", "absolute path to private local catalogue")
	flag.StringVar(&o.tlsCert, "tls-cert", "", "path to TLS server certificate containing numeric listener IP SAN")
	flag.StringVar(&o.tlsKey, "tls-key", "", "path to private TLS server key (file permissions 0600)")
	flag.BoolVar(&o.enableBearerAuth, "enable-bearer-auth", false, "explicitly enable JWT verification (requires TLS and all provider settings)")
	flag.StringVar(&o.issuer, "auth-issuer", "", "exact trusted HTTPS JWT issuer")
	flag.StringVar(&o.audience, "auth-audience", "", "expected API resource audience")
	flag.StringVar(&o.clientID, "auth-client-id", "", "expected authorised OAuth client ID")
	flag.StringVar(&o.requiredACR, "auth-required-acr", "", "required IdP MFA assurance value")
	flag.StringVar(&o.keyID, "auth-key-id", "", "expected JWT signing key ID")
	flag.StringVar(&o.rsaPublicKey, "auth-public-key", "", "pinned RSA verifier PUBLIC KEY PEM")
	flag.Parse()

	if err := o.check(); err != nil {
		return err
	}
	var tlsConfig *tls.Config
	if o.tlsCert != "" {
		cert, err := loadTLSCertificate(o.tlsCert, o.tlsKey, o.listen)
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		}
	}

	handler, closeStore, err := configuredHandler(context.Background(), o)
	if err != nil {
		return err
	}
	defer closeStore()

	listener, err := net.Listen("tcp", o.listen)
	if err != nil {
		return fmt.Errorf("start loopback listener: %w", err)
	}
	defer listener.Close()
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}

	service := &http.Server{
		Handler:           handler,
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
	slog.Info("M1 local API started", "listen", listener.Addr().String(),
		"tls_enabled", tlsConfig != nil, "bearer_auth_enabled", o.enableBearerAuth,
		"version", server.Version)

	select {
	case err = <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown API: %w", err)
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
