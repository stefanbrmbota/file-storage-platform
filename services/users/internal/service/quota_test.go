package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
)

type fakeQuotaRepo struct {
	remaining int64
	err       error
	calls     int
	id        string
	bytes     int64
}

func (f *fakeQuotaRepo) ReserveQuota(_ context.Context, id string, bytes int64) (int64, error) {
	f.calls++
	f.id, f.bytes = id, bytes
	return f.remaining, f.err
}

func TestReserveQuota_OK(t *testing.T) {
	repo := &fakeQuotaRepo{remaining: 900}
	got, err := NewQuotaService(repo).ReserveQuota(context.Background(), "u1", 100)
	if err != nil || got != 900 {
		t.Fatalf("got = %d, err = %v", got, err)
	}
	if repo.id != "u1" || repo.bytes != 100 {
		t.Errorf("prosledjeno: %q, %d", repo.id, repo.bytes)
	}
}

func TestReserveQuota_InvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		id    string
		bytes int64
	}{
		{"prazan id", "", 10},
		{"id samo razmaci", "  ", 10},
		{"predug id", strings.Repeat("a", maxQuotaUserIDLen+1), 10},
		{"nula bajtova", "u1", 0},
		{"negativno", "u1", -5},
		{"iznad granice", "u1", MaxQuotaRequestBytes + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeQuotaRepo{}
			_, err := NewQuotaService(repo).ReserveQuota(context.Background(), tc.id, tc.bytes)
			if !errors.Is(err, ErrInvalidQuotaRequest) {
				t.Fatalf("err = %v, ocekivano ErrInvalidQuotaRequest", err)
			}
			if repo.calls != 0 {
				t.Error("repozitorijum ne sme biti pozvan")
			}
		})
	}
}

func TestReserveQuota_BoundaryBytesAllowed(t *testing.T) {
	repo := &fakeQuotaRepo{}
	if _, err := NewQuotaService(repo).ReserveQuota(context.Background(), "u1", MaxQuotaRequestBytes); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestReserveQuota_MapsRepoErrors(t *testing.T) {
	boom := errors.New("mongo: connection refused")
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"nepoznat korisnik", repository.ErrNotFound, ErrUserNotFound},
		{"nema mesta", repository.ErrQuotaExceeded, ErrQuotaExceeded},
		{"greska baze", boom, boom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewQuotaService(&fakeQuotaRepo{err: tc.in}).ReserveQuota(context.Background(), "u1", 10)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, ocekivano %v", err, tc.want)
			}
		})
	}
}
