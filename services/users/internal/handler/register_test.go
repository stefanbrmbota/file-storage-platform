package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

type fakeRegistrar struct {
	called bool
	got    service.RegistrationInput
	err    error
}

func (f *fakeRegistrar) Register(_ context.Context, in service.RegistrationInput) (model.User, error) {
	f.called = true
	f.got = in
	if f.err != nil {
		return model.User{}, f.err
	}
	return model.NewUser("id-1", in.FirstName, in.LastName, in.Email, "tajni-hes", 1<<30, time.Now()), nil
}

const validBody = `{"firstName":"Pera","lastName":"Perić","email":"pera@example.com","password":"Zeleni#Konj7","passwordConfirm":"Zeleni#Konj7"}`

func doRegister(reg Registrar, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	RegisterHandler(reg).ServeHTTP(rec, req)
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("telo nije JSON sa poljem error: %q", rec.Body.String())
	}
	return resp["error"]
}

func TestRegisterHandler_BadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"prazno telo", ``, "neispravan zahtev"},
		{"loš JSON", `{nije json`, "neispravan zahtev"},
		{"nepoznato polje (role)", `{"firstName":"Pera","lastName":"P","email":"a@b.rs","password":"Zeleni#Konj7","passwordConfirm":"Zeleni#Konj7","role":"ADMIN"}`, "neispravan zahtev"},
		{"prazan objekat", `{}`, "ime je obavezno"},
		{"loš email", strings.Replace(validBody, "pera@example.com", "pera", 1), "email nije ispravan"},
		{"slaba lozinka", strings.ReplaceAll(validBody, "Zeleni#Konj7", "abc"), "lozinka mora imati"},
		{"česta lozinka", strings.ReplaceAll(validBody, "Zeleni#Konj7", "Password1!"), "previše česta"},
		{"lozinke se ne poklapaju", strings.Replace(validBody, `"passwordConfirm":"Zeleni#Konj7"`, `"passwordConfirm":"Drugo#Konj7"`, 1), "ne poklapaju"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &fakeRegistrar{}
			rec := doRegister(reg, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, očekivano 400", rec.Code)
			}
			if msg := errorOf(t, rec); !strings.Contains(msg, tt.want) {
				t.Errorf("poruka %q ne sadrži %q", msg, tt.want)
			}
			if reg.called {
				t.Error("Registrar ne sme biti pozvan za neispravan zahtev")
			}
			if strings.Contains(rec.Body.String(), "Zeleni#Konj7") {
				t.Error("odgovor ne sme sadržati lozinku")
			}
		})
	}
}

func TestRegisterHandler_Valid(t *testing.T) {
	reg := &fakeRegistrar{}
	rec := doRegister(reg, validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, očekivano 201", rec.Code)
	}
	if !reg.called || reg.got.Email != "pera@example.com" {
		t.Fatalf("Registrar nije pozvan sa zahtevom: %+v", reg.got)
	}
	body := rec.Body.String()
	if strings.Contains(body, "tajni-hes") || strings.Contains(body, "Zeleni#Konj7") {
		t.Errorf("odgovor ne sme sadržati lozinku ni heš: %s", body)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["active"] != false || resp["email"] != "pera@example.com" {
		t.Errorf("neočekivan odgovor: %s", body)
	}
}

func TestRegisterHandler_EmailTaken(t *testing.T) {
	reg := &fakeRegistrar{err: service.ErrEmailTaken}
	rec := doRegister(reg, validBody)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, očekivano 409", rec.Code)
	}
	if msg := errorOf(t, rec); !strings.Contains(msg, "zauzet") {
		t.Errorf("poruka %q", msg)
	}
}

func TestRegisterHandler_RegistrarErrorHidden(t *testing.T) {
	reg := &fakeRegistrar{err: errors.New("mongo: connection refused 10.0.0.5")}
	rec := doRegister(reg, validBody)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, očekivano 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongo") {
		t.Errorf("interni detalji procureli: %s", rec.Body.String())
	}
}
