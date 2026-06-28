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
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "", "path to YAML config file (optional)")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}
	slog.Info("config loaded",
		"host", cfg.Host, "port", cfg.Port,
		"state_file", cfg.StateFile, "shutdown_ttl_s", cfg.ShutdownTTL,
	)

	// ── Restore state ─────────────────────────────────────────────────────────
	reg := newRegistry()

	saved, err := loadState(cfg.StateFile)
	if err != nil {
		slog.Warn("could not load state, starting fresh", "err", err)
	} else if len(saved) > 0 {
		slog.Info("restoring subservers", "count", len(saved))
		reg.restore(saved)
	}

	// ── Management API — dual-stack ───────────────────────────────────────────
	// We apply the same dual-stack pattern to the management API itself.
	router := newRouter(reg)

	addr4 := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	addr6 := fmt.Sprintf("[::]:%d", cfg.Port)

	ln4, err := net.Listen("tcp4", addr4)
	if err != nil {
		slog.Error("listen tcp4 failed", "addr", addr4, "err", err)
		os.Exit(1)
	}
	ln6, err := net.Listen("tcp6", addr6)
	if err != nil {
		// IPv6 may be absent — warn and continue IPv4-only.
		slog.Warn("listen tcp6 failed, continuing IPv4-only", "addr", addr6, "err", err)
		ln6 = nil
	}

	mgmt4 := &http.Server{Handler: router}
	mgmt6 := &http.Server{Handler: router}

	go func() {
		slog.Info("mgmt API listening", "stack", "ipv4", "addr", addr4)
		if err := mgmt4.Serve(ln4); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("mgmt ipv4 error", "err", err)
		}
	}()
	if ln6 != nil {
		go func() {
			slog.Info("mgmt API listening", "stack", "ipv6", "addr", addr6)
			if err := mgmt6.Serve(ln6); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("mgmt ipv6 error", "err", err)
			}
		}()
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	// Block until SIGINT or SIGTERM, then:
	//   1. Stop the management API (no new registrations).
	//   2. Stop all sub-servers (drain in-flight requests).
	//   3. Persist state to disk.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("signal received, shutting down", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.ShutdownTTL)*time.Second)
	defer cancel()

	if err := mgmt4.Shutdown(ctx); err != nil {
		slog.Warn("mgmt ipv4 shutdown error", "err", err)
	}
	if ln6 != nil {
		if err := mgmt6.Shutdown(ctx); err != nil {
			slog.Warn("mgmt ipv6 shutdown error", "err", err)
		}
	}

	reg.shutdownAll(ctx)

	snap := reg.snapshot()
	if err := saveState(cfg.StateFile, snap); err != nil {
		slog.Error("failed to save state", "err", err)
	} else {
		slog.Info("state saved", "file", cfg.StateFile, "servers", len(snap))
	}

	slog.Info("shutdown complete")
}
