package model

import "time"

// OTP je zapis o jednokratnom kodu (6 cifara) za korisnika. UserID je ujedno
// _id, pa korisnik ima najviše jedan aktivan OTP: novi zamenjuje stari.
// CodeHash je HMAC koda; sam kod se nikad ne čuva.
type OTP struct {
	UserID    string    `bson:"_id"`
	CodeHash  string    `bson:"codeHash"`
	ExpiresAt time.Time `bson:"expiresAt"`
	Attempts  int       `bson:"attempts"`
}
