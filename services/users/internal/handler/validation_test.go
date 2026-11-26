package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPathIDWhitelist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := pathID(w, r); ok {
			w.WriteHeader(http.StatusOK)
		}
	})

	tests := []struct {
		id   string
		want int
	}{
		{"u1", http.StatusOK},
		{"6ae1ac2bc427d65b79b131f7", http.StatusOK},
		{"a-b_c", http.StatusOK},
		{strings.Repeat("a", 129), http.StatusBadRequest},
		{"a%24ne", http.StatusBadRequest},    // $ne
		{"a%7B%7D", http.StatusBadRequest},   // {}
		{"a%27%20OR", http.StatusBadRequest}, // ' OR
		{"a.b", http.StatusBadRequest},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x/"+tc.id, nil))
		if rec.Code != tc.want {
			t.Errorf("id %q: status = %d, want %d", tc.id, rec.Code, tc.want)
		}
	}
}
