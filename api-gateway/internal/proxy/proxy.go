// Package proxy prosledjuje zahteve klijenta servisima u internoj mrezi
// (US-30, design 5.3). Gateway je Go servis sa httputil.ReverseProxy, a ne
// Nginx, jer provera JWT-a i opozvanih tokena (T-30.5) trazi logiku i poziv
// Users servisa.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Headeri identiteta koje postavlja samo gateway iz JWT-a (design 5.3, tacka 2).
// Ako ih klijent posalje, brisu se, da ne bi mogao da se predstavi kao drugi korisnik.
const (
	HeaderUserID   = "X-User-ID"
	HeaderUserRole = "X-User-Role"
)

// dialTimeout je najduze vreme za uspostavljanje TCP veze sa servisom.
const dialTimeout = 5 * time.Second

// New pravi handler koji svaki zahtev prosledjuje servisu na adresi target,
// sa istom putanjom i upitom.
//
// timeout je najduze cekanje na odgovor servisa (zaglavlja). Ne ogranicava
// trajanje prenosa tela, da otpremanje i preuzimanje velikih datoteka ne bi
// bilo prekinuto. Ako servis nije dostupan, klijent dobija 502, a ako ne
// odgovori na vreme, 504; oba bez internih detalja.
func New(target *url.URL, timeout time.Duration, logger *slog.Logger) http.Handler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = timeout

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// Host servisa, a ne gateway-a, jer servisi ne treba da znaju spoljnu adresu.
			pr.Out.Host = target.Host
			pr.SetXForwarded()
			pr.Out.Header.Del(HeaderUserID)
			pr.Out.Header.Del(HeaderUserRole)
		},
		Transport:    transport,
		ErrorHandler: errorHandler(target.Host, logger),
	}
}

// errorHandler pretvara gresku prosledjivanja u JSON odgovor klijentu, a
// detalje belezi samo u log.
func errorHandler(upstream string, logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// Klijent je odustao; nema kome da se odgovori.
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}

		// url.Error sadrzi punu adresu zahteva; u log ide samo uzrok.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}

		status, msg := http.StatusBadGateway, "service unavailable"
		var nerr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
			status, msg = http.StatusGatewayTimeout, "service timeout"
		}
		logger.Warn("prosledjivanje nije uspelo",
			"upstream", upstream, "method", r.Method, "path", r.URL.Path,
			"status", status, "err", err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
	}
}
