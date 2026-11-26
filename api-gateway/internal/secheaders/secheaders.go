// Package secheaders dodaje bezbednosna zaglavlja na svaki odgovor gateway-a
// (US-62, T-62.1).
//
// Odgovori API-ja sadrze licne podatke (nalog, kvota, lista fajlova), pa:
//   - Cache-Control: no-store: pretrazivac i proxy ih ne cuvaju na disku;
//   - X-Content-Type-Options: nosniff: pretrazivac ne pogadja tip sadrzaja
//     (JSON ili fajl se ne izvrsava kao HTML/skripta);
//   - X-Frame-Options: DENY: odgovor ne moze da se ugradi u tudju stranicu;
//   - Referrer-Policy: no-referrer: adresa se ne salje drugim sajtovima.
//
// Servis moze da prepise Cache-Control (npr. za preuzimanje fajla), jer se
// zaglavlja postavljaju pre nego sto servis odgovori.
package secheaders

import "net/http"

// Middleware postavlja bezbednosna zaglavlja i zatim poziva next.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
