package model

import "time"

// ConfirmationToken je zapis o jednokratnom tokenu za potvrdu registracije.
// ID je SHA-256 heš tokena (heksadecimalno): sam token se nikad ne čuva, pa
// ni procurela baza ne daje upotrebljive linkove.
type ConfirmationToken struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	ExpiresAt time.Time `bson:"expiresAt"`
}
