package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"DeepSeek_Web_To_API/internal/auth"
	"DeepSeek_Web_To_API/internal/config"
	"DeepSeek_Web_To_API/internal/server"
	"DeepSeek_Web_To_API/internal/version"
	"DeepSeek_Web_To_API/internal/webui"
)

func main() {
	config.ApplyLegacyEnvAliases()
	if err := config.LoadDotEnv(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to load .env: %v\n", err)
	}

	app, err := server.NewApp()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	webui.EnsureBuiltOnStartup(app.Store)

	if err := auth.ValidateAdminRuntimeSecurity(app.Store); err != nil {
		config.Logger.Warn("[admin] security configuration incomplete", "error", err)
	}

	addr := net.JoinHostPort(app.Store.ServerBindAddr(), app.Store.ServerPort())
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Router,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	v, _ := version.Current()
	if v != "" {
		config.Logger.Info("starting DeepSeek_Web_To_API", "version", v, "addr", addr)
	} else {
		config.Logger.Info("starting DeepSeek_Web_To_API", "addr", addr)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		_, _ = fmt.Fprintf(os.Stderr, "fatal: server error: %v\n", err)
		os.Exit(1)
	case sig := <-quit:
		config.Logger.Info("shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: graceful shutdown error: %v\n", err)
	}
}
