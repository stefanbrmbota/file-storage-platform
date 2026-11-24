package auth

import (
	"encoding/json"
	"net/http"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
)

// SessionHandler obradjuje GET /api/auth/session (T-32.4): vraca
// {"userId", "role"} za proveren token. Ruta se registruje iza Require, pa do
// ovde stize samo zahtev sa ispravnim i neopozvanim tokenom; inace 401.
//
// Klijent je zove kad ima sacuvan token, da proveri da sesija i dalje vazi
// (npr. da korisnik nije odjavljen u drugom tabu), jer sam ne moze da proveri
// ni potpis ni opoziv.
func SessionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok {
			apierror.WriteStatus(w, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{"userId": id.UserID, "role": id.Role})
	})
}
