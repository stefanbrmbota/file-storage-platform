package secheaders

import "net/http"

// HSTS dodaje Strict-Transport-Security (US-67): posle prvog HTTPS odgovora
// pretrazivac godinu dana ovom hostu salje zahteve SAMO preko HTTPS-a, pa
// napadac ne moze da ga "spusti" na nesifrovan HTTP. Koristi se samo kad
// gateway radi preko TLS-a.
func HSTS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}
