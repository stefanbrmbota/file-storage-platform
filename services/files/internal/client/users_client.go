// Package client holds HTTP clients for calls to other services.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var (
	// ErrQuotaExceeded means the user has no room for the requested bytes.
	ErrQuotaExceeded = errors.New("storage quota exceeded")
	// ErrUsersUnavailable means the Users service did not answer in time or
	// answered with an unexpected error. The upload must be refused (fail closed).
	ErrUsersUnavailable = errors.New("users service unavailable")
)

// InternalTokenHeader carries the shared secret that internal endpoints of
// other services require (same header the Users service checks).
const InternalTokenHeader = "X-Internal-Token"

// UsersClient calls the internal quota endpoints of the Users service
// (design 5.4, S3):
//
//	POST /internal/users/{id}/quota/reserve  {"bytes": n}  -> 2xx or 409 (no room)
//	POST /internal/users/{id}/quota/release  {"bytes": n}  -> 2xx
//
// Every call sends the internal token; without it the Users service refuses.
type UsersClient struct {
	baseURL string
	token   string
	http    *http.Client
	timeout time.Duration
}

// NewUsersClient returns a client where every call has its own timeout (2.6.2).
func NewUsersClient(baseURL, internalToken string, timeout time.Duration) *UsersClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &UsersClient{
		baseURL: baseURL,
		token:   internalToken,
		http:    &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// Reserve takes bytes from the user's free space before the upload starts.
// The Users service does the check and the update atomically, so two uploads
// at the same time can never both take the last free space.
func (c *UsersClient) Reserve(ctx context.Context, userID string, bytes int64) error {
	return c.quota(ctx, userID, "reserve", bytes)
}

// Release gives bytes back, for example when an upload fails halfway or a
// new version of a file is smaller than the old one.
func (c *UsersClient) Release(ctx context.Context, userID string, bytes int64) error {
	return c.quota(ctx, userID, "release", bytes)
}

func (c *UsersClient) quota(ctx context.Context, userID, action string, n int64) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body, err := json.Marshal(map[string]int64{"bytes": n})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/internal/users/%s/quota/%s", c.baseURL, url.PathEscape(userID), action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(InternalTokenHeader, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUsersUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusConflict:
		return ErrQuotaExceeded
	default:
		return fmt.Errorf("%w: quota %s returned status %d", ErrUsersUnavailable, action, resp.StatusCode)
	}
}
