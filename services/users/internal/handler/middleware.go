package handler

import (
	"encoding/json"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// RequireRole propušta samo zahteve čija je uloga (iz konteksta) među
// dozvoljenima; svi ostali, uključujući zahtev bez uloge, dobijaju 403.
// Uloga mora biti postavljena ranije (ContextWithRole), npr. u middleware-u za prijavu.
func RequireRole(allowed ...model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := RoleFromContext(r.Context())
			if !ok || !roleAllowed(role, allowed) {
				writeError(w, http.StatusForbidden, "zabranjen pristup")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func roleAllowed(role model.Role, allowed []model.Role) bool {
	if !role.Valid() {
		return false
	}
	for _, a := range allowed {
		if role == a {
			return true
		}
	}
	return false
}

// writeError šalje grešku klijentu kao JSON {"error": "..."} bez internih detalja.
func writeError(w http.ResponseWriter, status int, msg string) {
	setJSONHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
