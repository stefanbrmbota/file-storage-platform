package reqlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, status int, body string, req *http.Request) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	rec := httptest.NewRecorder()
	RejectedInput(logger, mux).ServeHTTP(rec, req)
	if rec.Code != status || rec.Body.String() != body {
		t.Fatalf("odgovor je izmenjen: %d %q", rec.Code, rec.Body.String())
	}
	if buf.Len() == 0 {
		return nil
	}
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log nije JSON: %v", err)
	}
	return entry
}

func TestLogsRejectedInputWithoutSecrets(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login?token=tajna", strings.NewReader(`{"email":"a@b.c","password":"Tajna123!"}`))
	req.Header.Set("X-User-ID", "u1")
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2")
	req.Header.Set("Authorization", "Bearer tajni-token")

	entry := serve(t, http.StatusBadRequest, `{"error":"email nije ispravan"}`, req)
	if entry == nil {
		t.Fatal("neuspela validacija nije zabeležena")
	}
	want := map[string]any{
		"level": "WARN", "route": "POST /login", "status": float64(400),
		"reason": "email nije ispravan", "userId": "u1", "ip": "203.0.113.7",
	}
	for k, v := range want {
		if entry[k] != v {
			t.Errorf("%s = %v, want %v", k, entry[k], v)
		}
	}
	raw, _ := json.Marshal(entry)
	for _, secret := range []string{"Tajna123!", "tajna", "tajni-token", "a@b.c"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("log sadrži osetljiv podatak %q: %s", secret, raw)
		}
	}
}

func TestLogsAllValidationStatuses(t *testing.T) {
	for _, status := range []int{400, 413, 415, 422} {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		if serve(t, status, `{"error":"x"}`, req) == nil {
			t.Errorf("status %d nije zabeležen", status)
		}
	}
}

func TestDoesNotLogOtherResponses(t *testing.T) {
	for _, status := range []int{200, 201, 401, 403, 404, 409, 500} {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		if entry := serve(t, status, `{"error":"x"}`, req); entry != nil {
			t.Errorf("status %d ne treba da se loguje kao validacija: %v", status, entry)
		}
	}
}

func TestReasonCannotForgeLogLines(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set("X-User-ID", "u1\nlevel=ERROR")
	entry := serve(t, http.StatusBadRequest, `{"error":"los\nzahtev"}`, req)
	if entry["userId"] != "u1level=ERROR" || entry["reason"] != "loszahtev" {
		t.Errorf("kontrolni znakovi nisu uklonjeni: %v", entry)
	}
}
