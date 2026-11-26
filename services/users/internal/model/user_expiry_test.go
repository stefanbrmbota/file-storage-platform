package model

import (
	"testing"
	"time"
)

var expiryBase = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

const sixtyDays = 60 * 24 * time.Hour

func TestPasswordExpiresAt_UsesPasswordChangedAt(t *testing.T) {
	changed := expiryBase.Add(-10 * 24 * time.Hour)
	u := User{PasswordChangedAt: changed, CreatedAt: expiryBase.Add(-100 * 24 * time.Hour)}
	if got, want := u.PasswordExpiresAt(sixtyDays), changed.Add(sixtyDays); !got.Equal(want) {
		t.Errorf("PasswordExpiresAt = %v, ocekivano %v", got, want)
	}
}

func TestPasswordExpiresAt_FallsBackToCreatedAt(t *testing.T) {
	created := expiryBase.Add(-5 * 24 * time.Hour)
	u := User{CreatedAt: created} // passwordChangedAt nedostaje
	if got, want := u.PasswordExpiresAt(sixtyDays), created.Add(sixtyDays); !got.Equal(want) {
		t.Errorf("PasswordExpiresAt = %v, ocekivano %v", got, want)
	}
}

func TestPasswordExpired(t *testing.T) {
	u := User{PasswordChangedAt: expiryBase}
	expires := expiryBase.Add(sixtyDays)
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"tek promenjena", expiryBase, false},
		{"usred roka", expiryBase.Add(30 * 24 * time.Hour), false},
		{"nanosekundu pre isteka", expires.Add(-time.Nanosecond), false},
		{"tacno na granici", expires, false},
		{"nanosekundu posle isteka", expires.Add(time.Nanosecond), true},
		{"mnogo posle isteka", expires.Add(365 * 24 * time.Hour), true},
	}
	for _, tc := range cases {
		if got := u.PasswordExpired(tc.now, sixtyDays); got != tc.want {
			t.Errorf("%s: PasswordExpired = %v, ocekivano %v", tc.name, got, tc.want)
		}
	}
}

func TestPasswordExpired_NoDatesMeansExpired(t *testing.T) {
	// Bez ikakvog datuma se ne moze dokazati da lozinka vazi: sigurniji ishod je istek.
	if !(User{}).PasswordExpired(expiryBase, sixtyDays) {
		t.Error("nalog bez datuma mora se tretirati kao istekao")
	}
}
