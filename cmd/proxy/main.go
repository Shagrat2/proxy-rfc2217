package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/api"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/config"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/connection"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/device"
	"git2.jad.ru/MeterRS485/proxy-rfc2217/internal/session"
)

// Build-time variables (set via ldflags)
var (
	BuildDate = "unknown"
	GitCommit = "unknown"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("RFC-2217 NAT Proxy starting... (build: %s, commit: %s)", BuildDate, GitCommit)

	cfg := config.Load()
	log.Printf("Config: port=%s api_port=%s keepalive=%v debug=%v",
		cfg.Port, cfg.APIPort, cfg.KeepAlive, cfg.Debug)

	// Create shared components
	registry := device.NewRegistry()
	sessions := session.NewManager(cfg.Debug, cfg.IdleTimeout)

	// Set session callbacks for logging
	sessions.SetCallbacks(
		func(s *session.Session) {
			log.Printf("[session] started: id=%s device=%s", s.ID, s.DeviceID)
		},
		func(s *session.Session) {
			log.Printf("[session] ended: id=%s device=%s bytes_in=%d bytes_out=%d",
				s.ID, s.DeviceID, s.BytesIn, s.BytesOut)
		},
	)

	// Create servers
	connServer := connection.NewServer(cfg, registry, sessions)
	apiServer := api.NewServer(cfg, registry, sessions)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start servers
	errCh := make(chan error, 2)

	go func() {
		errCh <- connServer.Start(ctx)
	}()

	go func() {
		errCh <- apiServer.Start(ctx)
	}()

	// Wait for shutdown or error
	select {
	case err := <-errCh:
		if err != nil {
			log.Printf("Server error: %v", err)
			cancel()
		}
	case <-ctx.Done():
	}

	log.Println("RFC-2217 NAT Proxy stopped")
}
