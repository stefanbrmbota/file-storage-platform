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

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/config"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/handler"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("neispravna konfiguracija", "err", err)
		os.Exit(1)
	}

	// Povezivanje na Cassandru i pravljenje tabela (T-07.1). Bez baze servis
	// ne moze da radi, pa se ne pokrece.
	session, err := repository.Connect(context.Background(), cfg.Cassandra, logger)
	if err != nil {
		logger.Error("cassandra nije dostupna", "err", err)
		os.Exit(1)
	}
	store := repository.NewStore(session, cfg.Cassandra.Timeout)
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.Health(store))

	// Obavestenja prijavljenog korisnika (US-07, T-07.3); kroz gateway
	// /api/notifications, a ID korisnika dolazi u X-User-ID.
	notifications := service.NewNotifications(store)
	mux.HandleFunc("GET /notifications", handler.ListNotifications(notifications))
	mux.HandleFunc("GET /notifications/unread-count", handler.UnreadCount(notifications))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("notifications service started", "port", cfg.Port,
			"cassandra_hosts", cfg.Cassandra.Hosts, "keyspace", cfg.Cassandra.Keyspace)
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
	logger.Info("notifications service stopped")
}
