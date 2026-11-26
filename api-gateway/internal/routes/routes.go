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

// Access je ko sme da pozove rutu (design 5.2, kolona JWT).
type Access int

const (
	// User: potreban ispravan, neopozvan token (podrazumevano).
	User Access = iota
	// Public: bez tokena (prijava, registracija, potvrda).
	Public
	// Admin: token sa ulogom ADMIN.
	Admin
)

// Route je jedna spoljna ruta: obrazac za http.ServeMux ("METODA /api/..."),
// servis kome se prosledjuje i ko sme da je pozove.
type Route struct {
	Pattern string
	Service Service
	Access  Access
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
	// Users: autentifikacija. Jedine javne rute (zajedno sa resetom lozinke ispod).
	{"POST /api/auth/register", Users, Public},
	{"POST /api/auth/confirm", Users, Public},
	{"POST /api/auth/login", Users, Public},
	{"POST /api/auth/login/verify", Users, Public},
	// Reset zaboravljene lozinke mejlom (US-15): korisnik nije prijavljen, pa su rute javne.
	{"POST /api/auth/password-reset/request", Users, Public},
	{"POST /api/auth/password-reset/confirm", Users, Public},
	// Odjava: token mora biti ispravan; Users ga opoziva (US-30, T-30.5).
	{"POST /api/auth/logout", Users, User},
	// Users: nalog i admin (ADMIN uloga se proverava i u servisu).
	{"GET /api/users/me", Users, User},
	// Promena lozinke (US-14); Users posle uspeha opoziva token kojim je zahtev poslat.
	{"POST /api/users/me/password", Users, User},
	{"GET /api/admin/users", Users, Admin},

	// Files: folderi, datoteke, pretraga.
	{"POST /api/folders", Files, User},
	{"GET /api/folders/{id}/contents", Files, User},
	{"PATCH /api/folders/{id}", Files, User},
	{"DELETE /api/folders/{id}", Files, User},
	{"POST /api/folders/{id}/files", Files, User},
	{"GET /api/files/{id}/content", Files, User},
	{"PUT /api/files/{id}/content", Files, User},
	{"PATCH /api/files/{id}", Files, User},
	{"DELETE /api/files/{id}", Files, User},
	{"GET /api/search", Files, User},

	// Sharing.
	{"POST /api/shares", Sharing, User},
	{"DELETE /api/shares/{resourceId}/{granteeId}", Sharing, User},
	{"GET /api/shares/with-me", Sharing, User},
	{"GET /api/shares/{resourceId}", Sharing, User},

	// Notifications.
	{"GET /api/notifications", Notifications, User},
	{"GET /api/notifications/unread-count", Notifications, User},
	{"POST /api/notifications/{id}/read", Notifications, User},
	{"POST /api/notifications/read-all", Notifications, User},

	// Analytics.
	{"GET /api/analytics/me", Analytics, User},
	{"GET /api/analytics/me/events", Analytics, User},
}

// Guard obavija handler proverom pristupa (auth.Authenticator).
type Guard interface {
	Require(http.Handler) http.Handler
	RequireAdmin(http.Handler) http.Handler
}

// Register registruje sve rute na mux, i 404 za svaku putanju koja nije na
// listi. upstreams za svaki servis daje handler koji prosledjuje zahtev
// (proxy.New), a guard proverava token na rutama koje nisu javne. Servis bez
// handlera ili nil guard je greska u konfiguraciji i izaziva panic pri
// pokretanju.
func Register(mux *http.ServeMux, upstreams map[Service]http.Handler, guard Guard) {
	if guard == nil {
		panic("routes: guard je obavezan")
	}
	for _, rt := range All {
		h, ok := upstreams[rt.Service]
		if !ok || h == nil {
			panic("routes: nema handlera za servis " + string(rt.Service))
		}
		h = stripPrefix(h)
		switch rt.Access {
		case Public:
		case Admin:
			h = guard.RequireAdmin(h)
		default:
			h = guard.Require(h)
		}
		mux.Handle(rt.Pattern, h)
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
