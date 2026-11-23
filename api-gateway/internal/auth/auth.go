// Package auth proverava JWT na zasticenim rutama gateway-a (US-30, T-30.5,
// design 5.3 tacke 1-3).
//
// Za svaki zahtev na zasticenoj ruti:
//  1. token iz Authorization: Bearer <token> mora biti potpisan sa HS256
//     (ista tajna JWT_SECRET kao u Users servisu), neistekao, sa sub, role
//     (USER ili ADMIN) i jti;
//  2. Users servis (POST /internal/tokens/check, poziv S6) potvrdjuje da
//     token nije opozvan odjavom;
//  3. za admin rute uloga mora biti ADMIN.
//
// Neispravan, istekao ili opozvan token → 401, pogresna uloga → 403. Ako
// Users servis ne odgovori, zahtev se odbija sa 503 (fail closed): bez
// potvrde da token nije opozvan nema pristupa.
//
// Proveren identitet ide u context zahteva (FromContext); proxy ga salje
// servisu u headerima X-User-ID i X-User-Role.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
)

// Uloge iz tokena (iste kao model.Role u Users servisu).
const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"
)

const (
	// tokenCheckPath je interna ruta Users servisa za proveru opoziva (S6).
	tokenCheckPath = "/internal/tokens/check"
	// maxJTILen je ista granica kao u Users servisu.
	maxJTILen = 128
	// maxCheckResponse ogranicava odgovor Users servisa ({"revoked": true}).
	maxCheckResponse = 1 << 10
)

// Identity je proveren identitet korisnika iz tokena.
type Identity struct {
	UserID string
	Role   string
}

type ctxKey struct{}

// FromContext vraca identitet koji je postavio Authenticator, ako postoji.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// ContextWithIdentity vraca context sa identitetom. Koristi ga Authenticator,
// a van paketa samo testovi.
func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// errRevocationCheck znaci da Users servis nije potvrdio stanje tokena.
var errRevocationCheck = errors.New("auth: provera opoziva nije uspela")

// Authenticator proverava tokene jednom tajnom i pita Users servis za opoziv.
type Authenticator struct {
	secret   []byte
	checkURL string
	client   *http.Client
	timeout  time.Duration
	logger   *slog.Logger
	now      func() time.Time
}

// New pravi Authenticator. usersURL je adresa Users servisa u internoj
// mrezi, a timeout najduze cekanje na proveru opoziva.
func New(secret string, usersURL *url.URL, timeout time.Duration, logger *slog.Logger) *Authenticator {
	return &Authenticator{
		secret:   []byte(secret),
		checkURL: usersURL.JoinPath(tokenCheckPath).String(),
		client:   &http.Client{},
		timeout:  timeout,
		logger:   logger,
		now:      time.Now,
	}
}

// Require propusta samo zahteve sa ispravnim, neopozvanim tokenom.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return a.handler("", next)
}

// RequireAdmin je Require uz uslov da je uloga ADMIN (design 5.3, tacka 3).
func (a *Authenticator) RequireAdmin(next http.Handler) http.Handler {
	return a.handler(RoleAdmin, next)
}

func (a *Authenticator) handler(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := bearerToken(r)
		if !ok {
			apierror.WriteStatus(w, http.StatusUnauthorized)
			return
		}
		c, err := a.parse(tok)
		if err != nil {
			apierror.WriteStatus(w, http.StatusUnauthorized)
			return
		}

		revoked, err := a.isRevoked(r.Context(), c.ID)
		if err != nil {
			// Token se ne loguje; jti je slucajan ID, ali ni on nije potreban.
			a.logger.Warn("provera opozvanog tokena nije uspela", "err", err)
			apierror.WriteStatus(w, http.StatusServiceUnavailable)
			return
		}
		if revoked {
			apierror.WriteStatus(w, http.StatusUnauthorized)
			return
		}

		if role != "" && c.Role != role {
			apierror.WriteStatus(w, http.StatusForbidden)
			return
		}

		id := Identity{UserID: c.Subject, Role: c.Role}
		next.ServeHTTP(w, r.WithContext(ContextWithIdentity(r.Context(), id)))
	})
}

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// parse proverava potpis (samo HS256, odbija alg=none i zamenu algoritma),
// rok vazenja i obavezne claimove. Svaki razlog odbijanja je ista greska.
func (a *Authenticator) parse(tok string) (claims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(tok, &c,
		func(*jwt.Token) (any, error) { return a.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(a.now),
	)
	if err != nil || c.Subject == "" || (c.Role != RoleUser && c.Role != RoleAdmin) ||
		c.ID == "" || len(c.ID) > maxJTILen {
		return claims{}, errors.New("auth: nevazeci token")
	}
	return c, nil
}

// isRevoked pita Users servis da li je token opozvan. jti ide u telo, ne u URL.
func (a *Authenticator) isRevoked(ctx context.Context, jti string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"jti": jti})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.checkURL, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("%w: %v", errRevocationCheck, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err // bez pune adrese u logu
		}
		return false, fmt.Errorf("%w: %v", errRevocationCheck, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%w: status %d", errRevocationCheck, resp.StatusCode)
	}
	var out struct {
		Revoked *bool `json:"revoked"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCheckResponse)).Decode(&out); err != nil || out.Revoked == nil {
		return false, fmt.Errorf("%w: neispravan odgovor", errRevocationCheck)
	}
	return *out.Revoked, nil
}

// bearerToken vraca token iz Authorization headera (sema Bearer, bez obzira na velika slova).
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	return tok, tok != ""
}
