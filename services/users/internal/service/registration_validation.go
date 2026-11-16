package service

import (
	_ "embed"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	minPasswordLen = 8
	// Gornja granica štiti od preskupog heširanja ogromnih ulaza.
	maxPasswordLen = 128
	maxNameLen     = 50
	maxEmailLen    = 254
)

//go:embed common_passwords.txt
var commonPasswordsRaw string

// commonPasswords je skup čestih lozinki (malim slovima), učitan jednom pri startu.
var commonPasswords = func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, line := range strings.Split(commonPasswordsRaw, "\n") {
		if p := strings.ToLower(strings.TrimSpace(line)); p != "" {
			set[p] = struct{}{}
		}
	}
	return set
}()

// RegistrationInput je zahtev za registraciju (US-04). Nema polje za ulogu:
// uloga se nikad ne uzima iz zahteva.
type RegistrationInput struct {
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"passwordConfirm"`
}

// ValidateRegistration proverava zahtev i vraca listu poruka o tome sta nije
// u redu (prazna lista znaci da je zahtev ispravan). Poruke nikad ne sadrze
// vrednost lozinke.
func ValidateRegistration(in RegistrationInput) []string {
	var problems []string

	problems = append(problems, validateName("ime", in.FirstName)...)
	problems = append(problems, validateName("prezime", in.LastName)...)
	problems = append(problems, validateEmail(in.Email)...)
	problems = append(problems, validatePassword(in.Password, in.PasswordConfirm)...)

	return problems
}

func validateName(label, value string) []string {
	v := strings.TrimSpace(value)
	switch {
	case v == "":
		return []string{label + " je obavezno"}
	case utf8.RuneCountInString(v) > maxNameLen:
		return []string{label + " može imati najviše 50 karaktera"}
	}
	return nil
}

func validateEmail(value string) []string {
	v := strings.TrimSpace(value)
	if v == "" {
		return []string{"email je obavezan"}
	}
	if len(v) > maxEmailLen {
		return []string{"email je predugačak"}
	}
	addr, err := mail.ParseAddress(v)
	// Address mora biti jednak ulazu: odbija oblik "Ime <a@b.c>" i komentare.
	if err != nil || addr.Address != v {
		return []string{"email nije ispravan"}
	}
	at := strings.LastIndex(v, "@")
	domain := v[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return []string{"email nije ispravan"}
	}
	return nil
}

func validatePassword(pw, confirm string) []string {
	if pw == "" {
		return []string{"lozinka je obavezna"}
	}

	var problems []string
	n := utf8.RuneCountInString(pw)
	if n < minPasswordLen {
		problems = append(problems, "lozinka mora imati najmanje 8 karaktera")
	}
	if n > maxPasswordLen {
		problems = append(problems, "lozinka može imati najviše 128 karaktera")
	}

	var upper, lower, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special = true
		}
	}
	if !upper {
		problems = append(problems, "lozinka mora imati veliko slovo")
	}
	if !lower {
		problems = append(problems, "lozinka mora imati malo slovo")
	}
	if !digit {
		problems = append(problems, "lozinka mora imati broj")
	}
	if !special {
		problems = append(problems, "lozinka mora imati specijalni znak")
	}

	if isCommonPassword(pw) {
		problems = append(problems, "lozinka je previše česta, izaberite drugu")
	}

	if confirm == "" {
		problems = append(problems, "ponovljena lozinka je obavezna")
	} else if confirm != pw {
		problems = append(problems, "lozinke se ne poklapaju")
	}
	return problems
}

// isCommonPassword poredi bez obzira na velika i mala slova, i sa lozinkom bez
// završnih cifara i znakova, pa se hvata i "Password123!".
func isCommonPassword(pw string) bool {
	lower := strings.ToLower(pw)
	if _, ok := commonPasswords[lower]; ok {
		return true
	}
	base := strings.TrimRightFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) })
	if base == "" {
		return false
	}
	_, ok := commonPasswords[base]
	return ok
}
