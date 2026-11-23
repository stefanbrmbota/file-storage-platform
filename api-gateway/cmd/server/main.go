package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/config"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/cors"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/proxy"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/routes"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("neispravna konfiguracija", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// Eksplicitna lista ruta (design 5.2); sve ostalo, ukljucujuci
	// /internal/..., vraca 404 (design 5.3, tacka 4). Rute koje nisu javne
	// traze ispravan, neopozvan JWT (T-30.5).
	routes.Register(mux, map[routes.Service]http.Handler{
		routes.Users:         proxy.New(cfg.UsersURL, cfg.UpstreamTimeout, logger),
		routes.Files:         proxy.New(cfg.FilesURL, cfg.UpstreamTimeout, logger),
		routes.Sharing:       proxy.New(cfg.SharingURL, cfg.UpstreamTimeout, logger),
		routes.Notifications: proxy.New(cfg.NotificationsURL, cfg.UpstreamTimeout, logger),
		routes.Analytics:     proxy.New(cfg.AnalyticsURL, cfg.UpstreamTimeout, logger),
	}, auth.New(cfg.JWTSecret, cfg.UsersURL, cfg.AuthTimeout, logger))

	srv := newServer(cfg, cors.Middleware(cfg.CORSAllowedOrigins, mux))

	go func() {
		logger.Info("gateway service started", "port", cfg.Port,
			"upstream_timeout", cfg.UpstreamTimeout.String(),
			"auth_timeout", cfg.AuthTimeout.String(),
			"read_header_timeout", cfg.ReadHeaderTimeout.String(),
			"idle_timeout", cfg.IdleTimeout.String(),
			"users", cfg.UsersURL.Host, "files", cfg.FilesURL.Host,
			"notifications", cfg.NotificationsURL.Host, "sharing", cfg.SharingURL.Host,
			"analytics", cfg.AnalyticsURL.Host,
			"cors_origins", cfg.CORSAllowedOrigins)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "err", err)
	}
	logger.Info("gateway service stopped")
}
