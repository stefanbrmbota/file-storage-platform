package handler

import (
	"net/http"
	"regexp"
)

// pathIDPattern je whitelist za ID iz putanje (korisnik, zahtev za kvotu):
// samo slova, cifre, crtica i donja crta, najviše 128 znakova (US-64).
var pathIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// pathID čita {id} iz putanje i odbija ga sa 400 ako nije u dozvoljenom obliku,
// pre nego što stigne do servisa i baze.
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !pathIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "neispravan id")
		return "", false
	}
	return id, true
}
