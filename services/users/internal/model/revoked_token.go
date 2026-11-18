package model

import "time"

// RevokedToken je zapis o opozvanom (odjavljenom) JWT-u. ID je jti claim tokena;
// sam token se ne čuva. ExpiresAt je trenutak isteka tokena: TTL indeks briše
// zapis kad token ionako prestane da važi.
type RevokedToken struct {
	ID        string    `bson:"_id"`
	ExpiresAt time.Time `bson:"expiresAt"`
}
