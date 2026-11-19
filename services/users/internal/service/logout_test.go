package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/token"
)

type fakeParser struct {
	claims token.Claims
	err    error
}

func (f fakeParser) Parse(string) (token.Claims, error) { return f.claims, f.err }

type fakeRevoked struct {
	jti   string
	exp   time.Time
	calls int
	err   error
}

func (f *fakeRevoked) Revoke(_ context.Context, jti string, exp time.Time) error {
	f.calls++
	f.jti, f.exp = jti, exp
	return f.err
}

func (f *fakeRevoked) IsRevoked(context.Context, string, time.Time) (bool, error) {
	return f.calls > 0, nil
}

func TestLogout_RevokesJTI(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	rev := &fakeRevoked{}
	s := NewLogoutService(fakeParser{claims: token.Claims{UserID: "u1", ID: "jti-1", ExpiresAt: exp}}, rev)

	if err := s.Logout(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
	if rev.jti != "jti-1" || !rev.exp.Equal(exp) {
		t.Errorf("upisano: %q %v", rev.jti, rev.exp)
	}
}

func TestLogout_InvalidToken(t *testing.T) {
	rev := &fakeRevoked{}
	s := NewLogoutService(fakeParser{err: token.ErrInvalidToken}, rev)
	if err := s.Logout(context.Background(), "tok"); !errors.Is(err, ErrInvalidAuthToken) {
		t.Fatalf("err = %v, ocekivano ErrInvalidAuthToken", err)
	}
	if rev.calls != 0 {
		t.Error("neispravan token ne sme biti upisan")
	}
}

func TestLogout_TokenWithoutJTI(t *testing.T) {
	rev := &fakeRevoked{}
	s := NewLogoutService(fakeParser{claims: token.Claims{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)}}, rev)
	if err := s.Logout(context.Background(), "tok"); !errors.Is(err, ErrInvalidAuthToken) {
		t.Fatalf("err = %v, ocekivano ErrInvalidAuthToken", err)
	}
	if rev.calls != 0 {
		t.Error("token bez jti ne sme biti upisan")
	}
}

func TestIsRevoked_AfterLogout(t *testing.T) {
	rev := &fakeRevoked{}
	s := NewLogoutService(fakeParser{claims: token.Claims{ID: "jti-1", ExpiresAt: time.Now().Add(time.Hour)}}, rev)

	if ok, err := s.IsRevoked(context.Background(), "jti-1"); err != nil || ok {
		t.Fatalf("prije odjave: ok=%v err=%v", ok, err)
	}
	if err := s.Logout(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.IsRevoked(context.Background(), "jti-1"); err != nil || !ok {
		t.Fatalf("poslije odjave: ok=%v err=%v", ok, err)
	}
}

func TestIsRevoked_RepositoryError(t *testing.T) {
	boom := errors.New("baza pala")
	s := NewLogoutService(fakeParser{}, &fakeRevokedErr{err: boom})
	if _, err := s.IsRevoked(context.Background(), "j"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

type fakeRevokedErr struct {
	fakeRevoked
	err error
}

func (f *fakeRevokedErr) IsRevoked(context.Context, string, time.Time) (bool, error) {
	return false, f.err
}

func TestLogout_RepositoryError(t *testing.T) {
	boom := errors.New("baza pala")
	s := NewLogoutService(fakeParser{claims: token.Claims{ID: "j", ExpiresAt: time.Now().Add(time.Hour)}}, &fakeRevoked{err: boom})
	err := s.Logout(context.Background(), "tok")
	if !errors.Is(err, boom) || errors.Is(err, ErrInvalidAuthToken) {
		t.Fatalf("err = %v", err)
	}
}
