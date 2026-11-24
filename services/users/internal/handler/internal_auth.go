package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
)

const (
	// InternalTokenHeader je zaglavlje kojim drugi servisi dokazuju da zovu interne rute.
	InternalTokenHeader = "X-Internal-Token"
	// MinInternalTokenLen je najkraća dozvoljena tajna za interne pozive.
	MinInternalTokenLen = 32
)

// ErrWeakInternalToken se vraća kada INTERNAL_TOKEN nije postavljen ili je prekratak.
var ErrWeakInternalToken = errors.New("handler: INTERNAL_TOKEN mora imati najmanje 32 karaktera")

// RequireInternalToken propušta samo zahteve sa ispravnim X-Internal-Token
// zaglavljem; svi ostali dobijaju 401 bez objašnjenja šta nije u redu.
// Prazan ili prekratak token je greška pri pokretanju (fail closed), da servis
// nikad ne radi sa otvorenim internim rutama. Token se ne loguje i ne vraća klijentu.
func RequireInternalToken(token string) (func(http.Handler) http.Handler, error) {
	if len(token) < MinInternalTokenLen {
		return nil, ErrWeakInternalToken
	}
	// Poredi se SHA-256 (fiksna dužina), pa trajanje poređenja ne otkriva dužinu tajne.
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := sha256.Sum256([]byte(r.Header.Get(InternalTokenHeader)))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeError(w, http.StatusUnauthorized, "neovlašćen pristup")
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
