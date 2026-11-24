package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/service"
)

// HeaderUserID postavlja gateway iz proverenog JWT-a (design 5.3, tacka 2).
// Servis ID korisnika cita samo odavde, nikad iz URL-a ili tela zahteva.
const HeaderUserID = "X-User-ID"

// maxCursorLen ogranicava kursor pre parsiranja (timeuuid ima 36 znakova).
const maxCursorLen = 64

// Lister je citanje obavestenja (service.Notifications).
type Lister interface {
	List(ctx context.Context, userID, cursor string, limit int) (service.Page, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
}

// ListNotifications obradjuje GET /notifications?limit=&cursor= (US-07, T-07.3):
// stranica obavestenja prijavljenog korisnika od najnovijeg, i nextCursor za
// sledecu stranicu.
func ListNotifications(l Lister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := currentUser(w, r)
		if !ok {
			return
		}

		q := r.URL.Query()
		limit := 0
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				writeError(w, http.StatusBadRequest, "limit mora biti pozitivan ceo broj")
				return
			}
			limit = n
		}
		cursor := q.Get("cursor")
		if len(cursor) > maxCursorLen {
			writeError(w, http.StatusBadRequest, "neispravan kursor")
			return
		}

		page, err := l.List(r.Context(), userID, cursor, limit)
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, page)
		case errors.Is(err, service.ErrInvalidCursor):
			writeError(w, http.StatusBadRequest, "neispravan kursor")
		default:
			unavailable(w, "lista obavestenja", err)
		}
	}
}

// UnreadCount obradjuje GET /notifications/unread-count: {"count": n}.
func UnreadCount(l Lister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := currentUser(w, r)
		if !ok {
			return
		}
		n, err := l.UnreadCount(r.Context(), userID)
		if err != nil {
			unavailable(w, "broj neprocitanih", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": n})
	}
}

// currentUser cita ID korisnika iz X-User-ID. Bez njega zahtev nije prosao
// kroz gateway (ili token nije proveren), pa se odbija sa 401.
func currentUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get(HeaderUserID))
	if id == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}

// unavailable belezi pravi razlog u log, a klijentu vraca opstu poruku.
func unavailable(w http.ResponseWriter, what string, err error) {
	slog.Error(what+" nije uspela", "err", err)
	writeError(w, http.StatusServiceUnavailable, "servis trenutno nije dostupan")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
