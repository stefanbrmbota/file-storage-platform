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

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/config"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/handler"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()

	// Connect to MongoDB and check that it answers before accepting requests.
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		logger.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}
	startCtx, cancelStart := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStart()
	if err := client.Ping(startCtx, nil); err != nil {
		logger.Error("mongo is not reachable", "err", err)
		os.Exit(1)
	}
	db := client.Database(cfg.MongoDB)

	folderRepo, err := repository.NewFolderRepository(startCtx, db)
	if err != nil {
		logger.Error("folder repository init failed", "err", err)
		os.Exit(1)
	}
	folders := service.NewFolderService(folderRepo)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler.New(folders).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("files service started", "port", cfg.Port)
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
	if err := client.Disconnect(ctx); err != nil {
		logger.Error("mongo disconnect failed", "err", err)
	}
	logger.Info("files service stopped")
}
