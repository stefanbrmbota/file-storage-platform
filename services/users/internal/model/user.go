package model

import "time"

// Role je uloga korisnika u sistemu (US-24).
type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
)

// Valid vraca true samo za poznate uloge.
func (r Role) Valid() bool {
	return r == RoleUser || r == RoleAdmin
}

// User je nalog korisnika. PasswordHash se nikad ne serijalizuje ka klijentu.
type User struct {
	ID                string    `json:"id" bson:"_id"`
	FirstName         string    `json:"firstName" bson:"firstName"`
	LastName          string    `json:"lastName" bson:"lastName"`
	Email             string    `json:"email" bson:"email"`
	PasswordHash      string    `json:"-" bson:"passwordHash"`
	Role              Role      `json:"role" bson:"role"`
	Active            bool      `json:"active" bson:"active"`
	QuotaBytes        int64     `json:"quotaBytes" bson:"quotaBytes"`
	UsedBytes         int64     `json:"usedBytes" bson:"usedBytes"`
	PasswordChangedAt time.Time `json:"passwordChangedAt" bson:"passwordChangedAt"`
	CreatedAt         time.Time `json:"createdAt" bson:"createdAt"`
}

// NewUser pravi novog korisnika za registraciju. Nalog je neaktivan dok se ne
// aktivira. Kvota dolazi iz konfiguracije. Uloga se ne prosledjuje: uvek je
// USER, pa je zahtev za registraciju ne moze zadati. Admin se postavlja
// iskljucivo van registracije.
func NewUser(id, firstName, lastName, email, passwordHash string, quotaBytes int64, now time.Time) User {
	return User{
		ID:                id,
		FirstName:         firstName,
		LastName:          lastName,
		Email:             email,
		PasswordHash:      passwordHash,
		Role:              RoleUser,
		Active:            false,
		QuotaBytes:        quotaBytes,
		UsedBytes:         0,
		PasswordChangedAt: now,
		CreatedAt:         now,
	}
}

// NewAdmin pravi admin nalog. Koristi se iskljucivo pri pokretanju servisa
// (US-24), nikad iz zahteva za registraciju. Admin je odmah aktivan.
func NewAdmin(id, firstName, lastName, email, passwordHash string, quotaBytes int64, now time.Time) User {
	u := NewUser(id, firstName, lastName, email, passwordHash, quotaBytes, now)
	u.Role = RoleAdmin
	u.Active = true
	return u
}

// PasswordExpiresAt vraca trenutak kad lozinka ističe: poslednja promena lozinke
// (passwordChangedAt) plus maxAge. Ako passwordChangedAt nedostaje (stari dokument),
// koristi se createdAt.
func (u User) PasswordExpiresAt(maxAge time.Duration) time.Time {
	changed := u.PasswordChangedAt
	if changed.IsZero() {
		changed = u.CreatedAt
	}
	return changed.Add(maxAge)
}

// PasswordExpired javlja da je lozinka starija od dozvoljenog (US-16). Tacno na
// granici lozinka jos vazi; istekla je tek posle nje.
func (u User) PasswordExpired(now time.Time, maxAge time.Duration) bool {
	return now.After(u.PasswordExpiresAt(maxAge))
}
