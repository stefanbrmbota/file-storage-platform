package service

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxMailNameRunes je najveća dužina imena u tekstu mejla (isto kao maxNameLen).
const maxMailNameRunes = 50

// mailName enkoduje korisnički podatak (ime) za tekst mejla (US-66). Mejlovi
// su text/plain, pa HTML nije opasan, ali ime ne sme da promeni izgled
// poruke: uklanjaju se prelomi redova i ostali kontrolni znakovi (ime ne može
// da doda lažne redove, npr. "Vaš nalog je blokiran, otvorite ..."), kao i
// nevidljivi znakovi za smer teksta (U+202E i slični), a dužina je ograničena.
func mailName(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError, unicode.Is(unicode.Cf, r):
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		case unicode.IsControl(r):
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > maxMailNameRunes {
		out = string([]rune(out)[:maxMailNameRunes])
	}
	if out == "" {
		return "korisniče"
	}
	return out
}
