package objectstore

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

// testConfig returns settings for a real MinIO with a fresh bucket name.
// Set FILES_TEST_MINIO_ENDPOINT to run these tests, for example:
//
//	$env:FILES_TEST_MINIO_ENDPOINT="localhost:9000"; go test ./...
//
// Credentials default to the values from .env.example.
func testConfig(t *testing.T) Config {
	t.Helper()
	endpoint := os.Getenv("FILES_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("FILES_TEST_MINIO_ENDPOINT not set, skipping MinIO test")
	}
	cfg := Config{
		Endpoint:  endpoint,
		AccessKey: envOr("FILES_TEST_MINIO_ACCESS_KEY", "change-me"),
		SecretKey: envOr("FILES_TEST_MINIO_SECRET_KEY", "change-me-minio"),
		Bucket:    "test-" + uuid.NewString()[:8],
	}
	t.Cleanup(func() {
		if s, err := New(context.Background(), cfg); err == nil {
			_ = s.client.RemoveBucket(context.Background(), cfg.Bucket)
		}
	})
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestNewCreatesBucketOnce(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)

	s, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	exists, err := s.client.BucketExists(ctx, cfg.Bucket)
	if err != nil || !exists {
		t.Fatalf("bucket should exist after New, exists=%v err=%v", exists, err)
	}

	// A restart of the service must not fail because the bucket is already there.
	if _, err := New(ctx, cfg); err != nil {
		t.Fatalf("second New: %v", err)
	}
}

func TestNewFailsWithWrongCredentials(t *testing.T) {
	cfg := testConfig(t)
	cfg.SecretKey = "wrong-secret"

	if _, err := New(context.Background(), cfg); err == nil {
		t.Fatal("New with wrong credentials should fail")
	}
}
