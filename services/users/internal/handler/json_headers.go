package handler

import "net/http"

// setJSONHeaders postavlja zaglavlja za svaki JSON odgovor (US-66): tačan
// Content-Type i nosniff, pa pretraživač nikad ne tumači
// korisničke podatke iz odgovora kao HTML ili skriptu. encoding/json uz to
// enkoduje <, > i & (\u003c, \u003e, \u0026).
func setJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
