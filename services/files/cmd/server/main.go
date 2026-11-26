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

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/config"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/handler"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/objectstore"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()

	// Connect to MongoDB and check that it answers before accepting requests.
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		logger.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}
	startCtx, cancelStart := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStart()
	if err := mongoClient.Ping(startCtx, nil); err != nil {
		logger.Error("mongo is not reachable", "err", err)
		os.Exit(1)
	}
	db := mongoClient.Database(cfg.MongoDB)

	folderRepo, err := repository.NewFolderRepository(startCtx, db)
	if err != nil {
		logger.Error("folder repository init failed", "err", err)
		os.Exit(1)
	}
	fileRepo, err := repository.NewFileRepository(startCtx, db)
	if err != nil {
		logger.Error("file repository init failed", "err", err)
		os.Exit(1)
	}
	folders := service.NewFolderService(folderRepo, fileRepo)

	// Connect to MinIO and create the bucket if needed.
	objects, err := objectstore.New(startCtx, objectstore.Config{
		Endpoint:  cfg.MinioEndpoint,
		AccessKey: cfg.MinioAccessKey,
		SecretKey: cfg.MinioSecretKey,
		Bucket:    cfg.MinioBucket,
		UseSSL:    cfg.MinioUseSSL,
	})
	if err != nil {
		logger.Error("object store init failed", "err", err)
		os.Exit(1)
	}

	if cfg.InternalToken == "" {
		logger.Warn("INTERNAL_TOKEN is not set: the Users service will refuse quota calls and uploads will fail")
	}
	users := client.NewUsersClient(cfg.UsersServiceURL, cfg.InternalToken, cfg.RequestTimeout)
	files := service.NewFileService(folders, fileRepo, objects, users).
		WithFileTypes(service.NewFileTypePolicy(cfg.AllowedExtensions))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler.New(folders, files, cfg.MaxUploadBytes).WithInternalToken(cfg.InternalToken).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		// Uploads (and later downloads) of big files take time. WriteTimeout
		// counts from the end of the request headers to the end of the
		// response, so it must cover the whole upload, not only the reply.
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
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
	if err := mongoClient.Disconnect(ctx); err != nil {
		logger.Error("mongo disconnect failed", "err", err)
	}
	logger.Info("files service stopped")
}
