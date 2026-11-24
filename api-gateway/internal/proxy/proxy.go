// Package proxy prosledjuje zahteve klijenta servisima u internoj mrezi
// (US-30, design 5.3). Gateway je Go servis sa httputil.ReverseProxy, a ne
// Nginx, jer provera JWT-a i opozvanih tokena (T-30.5) trazi logiku i poziv
// Users servisa.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/apierror"
	"github.com/stefanbrmbota/file-storage-platform/api-gateway/internal/auth"
)

// Headeri identiteta koje postavlja samo gateway iz JWT-a (design 5.3, tacka 2).
// Ako ih klijent posalje, brisu se, da ne bi mogao da se predstavi kao drugi
// korisnik; na zasticenim rutama se zatim postavljaju iz proverenog tokena.
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
//
// Greske servisa se svode na jedinstven format (T-30.3, vidi normalizeError).
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
			// Identitet samo iz tokena koji je proverio auth (T-30.5).
			if id, ok := auth.FromContext(pr.In.Context()); ok {
				pr.Out.Header.Set(HeaderUserID, id.UserID)
				pr.Out.Header.Set(HeaderUserRole, id.Role)
			}
		},
		Transport:      transport,
		ModifyResponse: normalizeError(target.Host, logger),
		ErrorHandler:   errorHandler(target.Host, logger),
	}
}

// normalizeError svodi gresku servisa na jedinstven format {"error": "..."}:
//   - 5xx: telo se uvek zamenjuje opstom porukom, jer moze da sadrzi interne
//     detalje; status ostaje isti, a pravi odgovor ide samo u log.
//   - 4xx sa JSON telom: prolazi neizmenjeno (npr. {"error":"folder not found"}).
//   - 4xx bez JSON tela (npr. "404 page not found" iz net/http): telo se
//     zamenjuje opstom porukom za taj status.
func normalizeError(upstream string, logger *slog.Logger) func(*http.Response) error {
	return func(resp *http.Response) error {
		status := resp.StatusCode
		if status < 400 {
			return nil
		}
		if status < 500 && isJSON(resp.Header.Get("Content-Type")) {
			return nil
		}
		if status >= 500 {
			logger.Warn("servis vratio gresku",
				"upstream", upstream, "method", resp.Request.Method,
				"path", resp.Request.URL.Path, "status", status)
		}
		_ = resp.Body.Close()
		body, _ := json.Marshal(apierror.Body{Error: apierror.Message(status)})
		body = append(body, '\n')
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Transfer-Encoding")
		return nil
	}
}

// isJSON prihvata application/json i tipove sa sufiksom +json.
func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
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

		status := http.StatusBadGateway
		var nerr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
			status = http.StatusGatewayTimeout
		}
		logger.Warn("prosledjivanje nije uspelo",
			"upstream", upstream, "method", r.Method, "path", r.URL.Path,
			"status", status, "err", err)

		apierror.WriteStatus(w, status)
	}
}
