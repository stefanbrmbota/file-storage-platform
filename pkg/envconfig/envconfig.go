// Package envconfig je zajednicko citanje konfiguracije Go servisa iz env
// promenljivih (US-31).
//
// Loader cita vrednosti sa podrazumevanim vrednostima i pamti sve greske
// (obavezna vrednost fali, neispravan broj ili trajanje), pa ih Err vraca
// odjednom. Servis na gresku iz Err treba da odbije pokretanje:
//
//	env := envconfig.New()
//	cfg := Config{
//		Port:    env.String("NOTIFICATIONS_PORT", "8083"),
//		Hosts:   env.Required("CASSANDRA_HOSTS"),
//		Timeout: env.Duration("NOTIFICATIONS_REQUEST_TIMEOUT", 5*time.Second),
//	}
//	if err := env.Err(); err != nil {
//		log.Fatal(err)
//	}
//
// Poruke o greskama sadrze samo ime promenljive, nikad njenu vrednost, da se
// lozinke i tajne ne bi pojavile u logovima.
//
// Prazna vrednost (ili samo razmaci) se tretira kao da promenljiva nije
// postavljena.
//
// Modul se u servis ukljucuje kroz go.mod servisa:
//
//	require github.com/stefanbrmbota/file-storage-platform/pkg/envconfig v0.0.0
//	replace github.com/stefanbrmbota/file-storage-platform/pkg/envconfig => ../../pkg/envconfig
package envconfig

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader cita env promenljive i skuplja greske. Nije bezbedan za
// istovremeno koriscenje iz vise gorutina; namenjen je pokretanju servisa.
type Loader struct {
	errs []error
}

// New pravi novi Loader bez gresaka.
func New() *Loader {
	return &Loader{}
}

// lookup vraca vrednost promenljive bez razmaka na krajevima i da li je
// zadata (neprazna).
func lookup(key string) (string, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	return v, v != ""
}

func (l *Loader) fail(format string, key string) {
	l.errs = append(l.errs, fmt.Errorf("envconfig: "+format, key))
}

// String vraca vrednost promenljive ili fallback ako nije zadata.
func (l *Loader) String(key, fallback string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return fallback
}

// Required vraca vrednost obavezne promenljive. Ako nije zadata, belezi
// gresku i vraca prazan string.
func (l *Loader) Required(key string) string {
	v, ok := lookup(key)
	if !ok {
		l.fail("obavezna promenljiva %s nije postavljena", key)
	}
	return v
}

// Int vraca ceo broj iz promenljive ili fallback ako nije zadata. Ako
// vrednost nije ceo broj, belezi gresku i vraca fallback.
func (l *Loader) Int(key string, fallback int) int {
	v, ok := lookup(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail("%s mora biti ceo broj", key)
		return fallback
	}
	return n
}

// Duration vraca pozitivno trajanje (npr. "5s", "24h") ili fallback ako
// promenljiva nije zadata. Ako vrednost nije ispravno pozitivno trajanje,
// belezi gresku i vraca fallback.
func (l *Loader) Duration(key string, fallback time.Duration) time.Duration {
	v, ok := lookup(key)
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.fail("%s mora biti pozitivno trajanje (npr. 5s, 24h)", key)
		return fallback
	}
	return d
}

// Bool vraca logicku vrednost (true/false, 1/0, ...) ili fallback ako
// promenljiva nije zadata. Ako vrednost nije ispravna, belezi gresku i vraca
// fallback.
func (l *Loader) Bool(key string, fallback bool) bool {
	v, ok := lookup(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail("%s mora biti true ili false", key)
		return fallback
	}
	return b
}

// Err vraca sve zabelezene greske spojene u jednu (errors.Join), ili nil ako
// je konfiguracija ispravna.
func (l *Loader) Err() error {
	return errors.Join(l.errs...)
}
