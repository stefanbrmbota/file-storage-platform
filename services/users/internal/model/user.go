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
	ID           string    `json:"id" bson:"_id"`
	Username     string    `json:"username" bson:"username"`
	Email        string    `json:"email" bson:"email"`
	PasswordHash string    `json:"-" bson:"passwordHash"`
	Role         Role      `json:"role" bson:"role"`
	CreatedAt    time.Time `json:"createdAt" bson:"createdAt"`
}

// NewUser pravi novog korisnika za registraciju. Uloga se ne prosledjuje:
// uvek je USER, pa je zahtev za registraciju ne moze zadati. Admin se
// postavlja iskljucivo van registracije.
func NewUser(id, username, email, passwordHash string, now time.Time) User {
	return User{
		ID:           id,
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         RoleUser,
		CreatedAt:    now,
	}
}
