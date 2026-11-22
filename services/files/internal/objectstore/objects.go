package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
)

// ErrNotFound means there is no object under the given key.
var ErrNotFound = errors.New("object not found")

// Object is a file's content read from MinIO. The caller must Close it.
type Object struct {
	io.ReadCloser
	Size        int64
	ContentType string
}

// Put stores content under key. size must be the exact number of bytes, so
// MinIO can stream the upload without keeping the whole file in memory.
func (s *Store) Put(ctx context.Context, key string, content io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, content, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("minio put %q: %w", key, err)
	}
	return nil
}

// Get opens the content stored under key. It returns ErrNotFound if the key
// does not exist.
func (s *Store) Get(ctx context.Context, key string) (*Object, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("minio get %q: %w", key, err)
	}
	// GetObject is lazy: it only talks to MinIO on the first read or Stat,
	// so we call Stat here to find out early whether the object exists.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("minio stat %q: %w", key, err)
	}
	return &Object{ReadCloser: obj, Size: info.Size, ContentType: info.ContentType}, nil
}

// Delete removes the content under key. Deleting a key that does not exist
// is not an error, so a retried delete (saga, US-56) is safe.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("minio delete %q: %w", key, err)
	}
	return nil
}
