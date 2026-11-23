// Package cors dozvoljava pozive gateway-u iz klijentske aplikacije u
// pretrazivacu (US-30, T-30.3).
//
// Dozvoljeni su samo origin-i sa liste (CORS_ALLOWED_ORIGINS), nikad "*".
// Token ide u headeru Authorization, ne u kolacicu, pa
// Access-Control-Allow-Credentials nije potreban.
package cors

import (
	"net/http"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
)

const (
	allowMethods  = "GET, POST, PATCH, DELETE"
	allowHeaders  = "Authorization, Content-Type"
	exposeHeaders = "Content-Disposition"
	maxAge        = "600" // sekundi; pretrazivac pamti preflight 10 minuta
)

// Middleware dodaje CORS headere za dozvoljene origin-e i odgovara na
// preflight (OPTIONS sa Access-Control-Request-Method).
//
// Zahtev sa nedozvoljenog origin-a ne dobija CORS headere, pa ga
// pretrazivac blokira; preflight sa takvog origin-a dobija 403. Zahtevi bez
// Origin headera (curl, servisi) prolaze bez izmena.
func Middleware(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Add("Vary", "Origin")
		preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

		if !allowed[origin] {
			if preflight {
				apierror.WriteStatus(w, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		h.Set("Access-Control-Allow-Origin", origin)
		if preflight {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", allowMethods)
			h.Set("Access-Control-Allow-Headers", allowHeaders)
			h.Set("Access-Control-Max-Age", maxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.Set("Access-Control-Expose-Headers", exposeHeaders)
		next.ServeHTTP(w, r)
	})
}

// ParseOrigins deli listu razdvojenu zarezima i uklanja razmake i prazne
// stavke, kao i kosu crtu na kraju (pretrazivac salje origin bez nje).
func ParseOrigins(raw string) []string {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}
