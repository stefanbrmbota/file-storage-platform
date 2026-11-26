package model

import "time"

// PasswordResetToken je zapis o jednokratnom tokenu za reset lozinke (US-15).
// _id je ID korisnika, pa korisnik ima najviše jedan aktivan token (novi
// zahtev zamenjuje stari). TokenHash je SHA-256 tokena (heksadecimalno): sam
// token se nikad ne čuva, pa ni procurela baza ne daje upotrebljive linkove.
type PasswordResetToken struct {
	UserID    string    `bson:"_id"`
	TokenHash string    `bson:"tokenHash"`
	CreatedAt time.Time `bson:"createdAt"`
	ExpiresAt time.Time `bson:"expiresAt"`
}
