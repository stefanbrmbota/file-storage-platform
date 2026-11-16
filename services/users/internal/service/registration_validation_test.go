package service

import (
	"strings"
	"testing"
)

func validInput() RegistrationInput {
	return RegistrationInput{
		FirstName:       "Pera",
		LastName:        "Perić",
		Email:           "pera@example.com",
		Password:        "Zeleni#Konj7",
		PasswordConfirm: "Zeleni#Konj7",
	}
}

func TestValidateRegistration_Valid(t *testing.T) {
	if p := ValidateRegistration(validInput()); len(p) != 0 {
		t.Fatalf("ispravan zahtev dobio greške: %v", p)
	}
}

func TestValidateRegistration_Rules(t *testing.T) {
	setPw := func(pw string) func(*RegistrationInput) {
		return func(i *RegistrationInput) { i.Password, i.PasswordConfirm = pw, pw }
	}
	tests := []struct {
		name   string
		mutate func(*RegistrationInput)
		want   string // deo poruke koja mora postojati
	}{
		{"prazno ime", func(i *RegistrationInput) { i.FirstName = "  " }, "ime je obavezno"},
		{"prazno prezime", func(i *RegistrationInput) { i.LastName = "" }, "prezime je obavezno"},
		{"predugačko ime", func(i *RegistrationInput) { i.FirstName = strings.Repeat("a", 51) }, "ime može imati najviše"},
		{"prazan email", func(i *RegistrationInput) { i.Email = "" }, "email je obavezan"},
		{"email bez @", func(i *RegistrationInput) { i.Email = "pera.example.com" }, "email nije ispravan"},
		{"email bez domena", func(i *RegistrationInput) { i.Email = "pera@" }, "email nije ispravan"},
		{"email bez tačke u domenu", func(i *RegistrationInput) { i.Email = "pera@localhost" }, "email nije ispravan"},
		{"email sa imenom", func(i *RegistrationInput) { i.Email = "Pera <pera@example.com>" }, "email nije ispravan"},
		{"email sa razmakom", func(i *RegistrationInput) { i.Email = "pe ra@example.com" }, "email nije ispravan"},
		{"predugačak email", func(i *RegistrationInput) { i.Email = strings.Repeat("a", 250) + "@example.com" }, "email je predugačak"},
		{"prazna lozinka", setPw(""), "lozinka je obavezna"},
		{"prekratka lozinka", setPw("Ab1!xyz"), "najmanje 8"},
		{"predugačka lozinka", setPw("Ab1!" + strings.Repeat("x", 130)), "najviše 128"},
		{"bez velikog slova", setPw("zeleni#konj7"), "veliko slovo"},
		{"bez malog slova", setPw("ZELENI#KONJ7"), "malo slovo"},
		{"bez broja", setPw("Zeleni#Konjic"), "broj"},
		{"bez specijalnog znaka", setPw("ZeleniKonj77"), "specijalni znak"},
		{"česta lozinka", setPw("Password1!"), "previše česta"},
		{"česta lozinka sa sufiksom", setPw("Welcome2024!"), "previše česta"},
		{"česta lozinka velika slova", setPw("QWERTY123!"), "previše česta"},
		{"nema ponovljene lozinke", func(i *RegistrationInput) { i.PasswordConfirm = "" }, "ponovljena lozinka je obavezna"},
		{"lozinke se ne poklapaju", func(i *RegistrationInput) { i.PasswordConfirm = "Zeleni#Konj8" }, "ne poklapaju"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			problems := ValidateRegistration(in)
			if !strings.Contains(strings.Join(problems, "|"), tt.want) {
				t.Fatalf("očekivana poruka %q, dobijeno %v", tt.want, problems)
			}
		})
	}
}

func TestValidateRegistration_PasswordBoundary(t *testing.T) {
	// Tačno 8 karaktera ispunjava pravila.
	in := validInput()
	in.Password, in.PasswordConfirm = "Ab1!cdEf", "Ab1!cdEf"
	if p := ValidateRegistration(in); len(p) != 0 {
		t.Errorf("8 karaktera treba da prolazi: %v", p)
	}
}

func TestValidateRegistration_ReportsAllProblems(t *testing.T) {
	problems := ValidateRegistration(RegistrationInput{})
	if len(problems) < 4 {
		t.Fatalf("očekivano više grešaka za prazan zahtev, dobijeno %v", problems)
	}
}

func TestValidateRegistration_NeverEchoesPassword(t *testing.T) {
	in := validInput()
	in.Password, in.PasswordConfirm = "tajna-Lozinka-xyz", "druga"
	for _, p := range ValidateRegistration(in) {
		if strings.Contains(p, "tajna-Lozinka-xyz") {
			t.Fatalf("poruka sadrži lozinku: %q", p)
		}
	}
}
