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

// DefaultQuotaBytes je podrazumevana kvota novog korisnika (1 GiB).
const DefaultQuotaBytes int64 = 1 << 30

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

// NewUser pravi novog korisnika za registraciju. Uloga se ne prosledjuje:
// uvek je USER, pa je zahtev za registraciju ne moze zadati. Admin se
// postavlja iskljucivo van registracije.
func NewUser(id, firstName, lastName, email, passwordHash string, now time.Time) User {
	return User{
		ID:                id,
		FirstName:         firstName,
		LastName:          lastName,
		Email:             email,
		PasswordHash:      passwordHash,
		Role:              RoleUser,
		Active:            true,
		QuotaBytes:        DefaultQuotaBytes,
		UsedBytes:         0,
		PasswordChangedAt: now,
		CreatedAt:         now,
	}
}

// NewAdmin pravi admin nalog. Koristi se iskljucivo pri pokretanju servisa
// (US-24), nikad iz zahteva za registraciju.
func NewAdmin(id, firstName, lastName, email, passwordHash string, now time.Time) User {
	u := NewUser(id, firstName, lastName, email, passwordHash, now)
	u.Role = RoleAdmin
	return u
}
