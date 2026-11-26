// Package client sadrži klijente za pozive drugih servisa.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// internalTokenHeader je zaglavlje kojim se Users predstavlja Files servisu
// na internim rutama (isto kao handler.InternalTokenHeader).
const internalTokenHeader = "X-Internal-Token"

// FilesClient poziva Files servis.
type FilesClient struct {
	baseURL       string
	internalToken string
	http          *http.Client
	timeout       time.Duration
}

// NewFilesClient vraća klijenta za Files servis; svaki poziv ima timeout.
// internalToken (INTERNAL_TOKEN) se šalje u X-Internal-Token, jer Files
// interne rute propušta samo uz njega (US-62, T-62.2).
func NewFilesClient(baseURL, internalToken string, timeout time.Duration) *FilesClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &FilesClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		internalToken: internalToken,
		http:          &http.Client{Timeout: timeout},
		timeout:       timeout,
	}
}

// CreateRootFolder traži od Files servisa korenski folder za korisnika (T-01.2).
// Poziv je idempotentan: 201 (kreiran) i 200 (već postoji) su uspeh.
func (c *FilesClient) CreateRootFolder(ctx context.Context, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	payload, err := json.Marshal(map[string]string{"userId": userID})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/folders/root", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(internalTokenHeader, c.internalToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("files servis nije dostupan: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("files servis vratio status %d", resp.StatusCode)
	}
	return nil
}
