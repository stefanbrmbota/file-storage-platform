package model

// QuotaRequester su podaci podnosioca zahteva za kvotu koje administrator vidi
// uz zahtev (US-22). Namerno nema heš lozinke ni druga interna polja.
type QuotaRequester struct {
	FirstName  string `bson:"firstName"`
	LastName   string `bson:"lastName"`
	Email      string `bson:"email"`
	QuotaBytes int64  `bson:"quotaBytes"` // trenutna kvota podnosioca
}

// QuotaRequestDetails je zahtev za kvotu zajedno sa podacima podnosioca.
// Requester je nil ako je nalog podnosioca u međuvremenu obrisan.
type QuotaRequestDetails struct {
	QuotaRequest `bson:",inline"`
	Requester    *QuotaRequester `bson:"user"`
}
