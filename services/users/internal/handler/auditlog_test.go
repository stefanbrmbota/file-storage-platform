package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

// captureLog preusmerava podrazumevani slog u bafer za trajanje testa.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// attempts vraća samo zapise o pokušajima prijave (bez ostalih logova).
func attempts(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log nije JSON: %q", line)
		}
		if m["msg"] == "pokusaj prijave" {
			out = append(out, m)
		}
	}
	return out
}

func loginReq(body, remote, xff string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

func TestLoginAttemptLogged(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantOutcome string
		wantLevel   string
	}{
		{"uspeh", nil, "success", "INFO"},
		{"neuspeh", service.ErrInvalidCredentials, "failure", "WARN"},
		{"istekla lozinka", service.ErrPasswordExpired, "expired", "WARN"},
		{"interna greska", errors.New("smtp pao"), "error", "WARN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			rec := httptest.NewRecorder()
			LoginHandler(&fakeLogin{err: tc.err}).ServeHTTP(rec,
				loginReq(`{"email":" Pera@Example.com ","password":"TajnaLozinka123!"}`, "10.0.0.7:5555", ""))

			got := attempts(t, buf)
			if len(got) != 1 {
				t.Fatalf("ocekivan 1 zapis o pokusaju, ima %d: %s", len(got), buf.String())
			}
			e := got[0]
			if e["email"] != "pera@example.com" || e["ip"] != "10.0.0.7" || e["outcome"] != tc.wantOutcome || e["step"] != "login" {
				t.Errorf("zapis: %v", e)
			}
			if e["level"] != tc.wantLevel {
				t.Errorf("level = %v, ocekivano %s", e["level"], tc.wantLevel)
			}
			if e["time"] == nil || e["time"] == "" {
				t.Error("zapis nema vreme")
			}
			if strings.Contains(buf.String(), "TajnaLozinka123!") {
				t.Errorf("lozinka je procurela u log: %s", buf.String())
			}
		})
	}
}

func TestLoginVerifyAttemptLogged(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantOutcome string
	}{
		{"uspeh", nil, "success"},
		{"neuspeh", service.ErrInvalidOTP, "failure"},
		{"istekla lozinka", service.ErrPasswordExpired, "expired"},
		{"interna greska", errors.New("baza pala"), "error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			rec := httptest.NewRecorder()
			LoginVerifyHandler(&fakeFinisher{err: tc.err}).ServeHTTP(rec,
				loginReq(`{"email":"pera@example.com","code":"482915"}`, "10.0.0.7:5555", ""))

			got := attempts(t, buf)
			if len(got) != 1 {
				t.Fatalf("ocekivan 1 zapis o pokusaju, ima %d: %s", len(got), buf.String())
			}
			if got[0]["step"] != "verify" || got[0]["outcome"] != tc.wantOutcome || got[0]["email"] != "pera@example.com" {
				t.Errorf("zapis: %v", got[0])
			}
			if strings.Contains(buf.String(), "482915") {
				t.Errorf("OTP je procurio u log: %s", buf.String())
			}
			if strings.Contains(buf.String(), "jwt-token") {
				t.Errorf("JWT je procurio u log: %s", buf.String())
			}
		})
	}
}

func TestLogin_BadRequestIsNotLoggedAsAttempt(t *testing.T) {
	buf := captureLog(t)
	LoginHandler(&fakeLogin{}).ServeHTTP(httptest.NewRecorder(), loginReq(`{nije json`, "10.0.0.7:5555", ""))
	LoginVerifyHandler(&fakeFinisher{}).ServeHTTP(httptest.NewRecorder(), loginReq(`{"email":"a@b.c","code":"12"}`, "10.0.0.7:5555", ""))
	if got := attempts(t, buf); len(got) != 0 {
		t.Errorf("neispravan zahtev se ne loguje kao pokusaj: %v", got)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, remote, xff, want string
	}{
		{"RemoteAddr", "192.168.1.5:4000", "", "192.168.1.5"},
		{"X-Forwarded-For", "10.0.0.1:80", "203.0.113.9", "203.0.113.9"},
		{"X-Forwarded-For lanac", "10.0.0.1:80", "203.0.113.9, 10.0.0.1", "203.0.113.9"},
		{"nevazeci XFF pada na RemoteAddr", "10.0.0.1:80", "nije-ip\nlazan red", "10.0.0.1"},
		{"IPv6", "[2001:db8::1]:443", "", "2001:db8::1"},
		{"bez porta", "10.0.0.2", "", "10.0.0.2"},
		{"smece", "nesto", "", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIP(loginReq("", tc.remote, tc.xff)); got != tc.want {
				t.Errorf("clientIP = %q, ocekivano %q", got, tc.want)
			}
		})
	}
}

func TestLoggableEmail(t *testing.T) {
	if got := loggableEmail("  Pera@Example.COM "); got != "pera@example.com" {
		t.Errorf("normalizacija: %q", got)
	}
	// Kontrolni znakovi (lazni redovi u logu) se uklanjaju.
	if got := loggableEmail("a@b.c\n{\"level\":\"ERROR\"}\r"); strings.ContainsAny(got, "\r\n") {
		t.Errorf("kontrolni znakovi nisu uklonjeni: %q", got)
	}
	if got := loggableEmail(strings.Repeat("a", 1000)); len(got) != maxLoggedEmailLen {
		t.Errorf("duzina = %d, ocekivano %d", len(got), maxLoggedEmailLen)
	}
}

// Ubacivanje novog reda kroz email ne sme da napravi poseban lazni zapis u logu.
func TestLoginLog_NoLogInjection(t *testing.T) {
	buf := captureLog(t)
	body := `{"email":"x@y.z\n{\"msg\":\"pokusaj prijave\",\"outcome\":\"success\"}","password":"p"}`
	LoginHandler(&fakeLogin{err: service.ErrInvalidCredentials}).ServeHTTP(httptest.NewRecorder(), loginReq(body, "10.0.0.7:5555", ""))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("ocekivan tacno 1 linija loga, ima %d: %s", len(lines), buf.String())
	}
	if got := attempts(t, buf); len(got) != 1 || got[0]["outcome"] != "failure" {
		t.Errorf("zapis: %v", got)
	}
}
