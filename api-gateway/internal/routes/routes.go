// Package routes je eksplicitna lista spoljnih ruta gateway-a (US-30,
// design 5.2). Gateway prosledjuje samo rute sa ove liste; sve ostalo,
// ukljucujuci /internal/..., vraca 404 (design 5.3, tacka 4).
//
// Putanja se servisu prosledjuje bez prefiksa /api (POST /api/folders →
// Files POST /folders), a rute /api/auth/... bez /api/auth
// (POST /api/auth/login → Users POST /login).
package routes

import (
	"net/http"
	"strings"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
)

// Service oznacava ciljni servis rute.
type Service string

const (
	Users         Service = "users"
	Files         Service = "files"
	Sharing       Service = "sharing"
	Notifications Service = "notifications"
	Analytics     Service = "analytics"
)

// Route je jedna spoljna ruta: obrazac za http.ServeMux ("METODA /api/...")
// i servis kome se prosledjuje.
type Route struct {
	Pattern string
	Service Service
}

// Prefiksi koji se uklanjaju pre prosledjivanja; prvi koji odgovara se primenjuje.
const (
	authPrefix = "/api/auth"
	apiPrefix  = "/api"
)

// All su sve rute iz design 5.2, implementirane i planirane. Planirane su
// unapred na listi da bi servis mogao da doda endpoint bez izmene gateway-a;
// dok ga servis nema, odgovara 404 sam servis.
//
// Nova spoljna ruta se dodaje ovde i u design 5.2. Rute /internal/... se
// nikad ne dodaju.
var All = []Route{
	// Users: autentifikacija (bez JWT-a, T-30.5).
	{"POST /api/auth/register", Users},
	{"POST /api/auth/confirm", Users},
	{"POST /api/auth/login", Users},
	{"POST /api/auth/login/verify", Users},
	// Users: nalog i admin (ADMIN uloga se proverava u T-30.5 i u servisu).
	{"GET /api/users/me", Users},
	{"GET /api/admin/users", Users},

	// Files: folderi, datoteke, pretraga.
	{"POST /api/folders", Files},
	{"GET /api/folders/{id}/contents", Files},
	{"PATCH /api/folders/{id}", Files},
	{"DELETE /api/folders/{id}", Files},
	{"POST /api/folders/{id}/files", Files},
	{"GET /api/files/{id}/content", Files},
	{"PATCH /api/files/{id}", Files},
	{"DELETE /api/files/{id}", Files},
	{"GET /api/search", Files},

	// Sharing.
	{"POST /api/shares", Sharing},
	{"DELETE /api/shares/{resourceId}/{granteeId}", Sharing},
	{"GET /api/shares/with-me", Sharing},
	{"GET /api/shares/{resourceId}", Sharing},

	// Notifications.
	{"GET /api/notifications", Notifications},
	{"GET /api/notifications/unread-count", Notifications},
	{"POST /api/notifications/{id}/read", Notifications},
	{"POST /api/notifications/read-all", Notifications},

	// Analytics.
	{"GET /api/analytics/me", Analytics},
	{"GET /api/analytics/me/events", Analytics},
}

// Register registruje sve rute na mux, i 404 za svaku putanju koja nije na
// listi. upstreams za svaki servis daje handler koji prosledjuje zahtev
// (proxy.New); servis bez handlera je greska u konfiguraciji i izaziva panic
// pri pokretanju.
func Register(mux *http.ServeMux, upstreams map[Service]http.Handler) {
	for _, rt := range All {
		h, ok := upstreams[rt.Service]
		if !ok || h == nil {
			panic("routes: nema handlera za servis " + string(rt.Service))
		}
		mux.Handle(rt.Pattern, stripPrefix(h))
	}
	// 404 i za pogresnu metodu postojece putanje, bez otkrivanja koje rute postoje.
	mux.HandleFunc("/", apierror.NotFound)
}

// stripPrefix uklanja /api/auth ili /api sa pocetka putanje i prosledjuje
// kopiju zahteva, a original ostaje nepromenjen.
func stripPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := apiPrefix
		if strings.HasPrefix(r.URL.Path, authPrefix+"/") {
			prefix = authPrefix
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		if r.URL.RawPath != "" {
			r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
		}
		next.ServeHTTP(w, r2)
	})
}
