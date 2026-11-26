package model

import "time"

// QuotaRequestStatus je stanje zahteva za povećanje kvote (US-21).
type QuotaRequestStatus string

const (
	// QuotaRequestPending je zahtev koji čeka odluku administratora.
	QuotaRequestPending QuotaRequestStatus = "PENDING"
	// QuotaRequestApproved je odobren zahtev (US-23).
	QuotaRequestApproved QuotaRequestStatus = "APPROVED"
	// QuotaRequestRejected je odbijen zahtev (US-23).
	QuotaRequestRejected QuotaRequestStatus = "REJECTED"
)

// QuotaRequest je zahtev korisnika za veću kvotu skladišta (US-21). Korisnik
// može imati najviše jedan zahtev na čekanju; odlučeni zahtevi ostaju kao istorija.
type QuotaRequest struct {
	ID             string             `json:"id" bson:"_id"`
	UserID         string             `json:"userId" bson:"userId"`
	RequestedBytes int64              `json:"requestedBytes" bson:"requestedBytes"`
	Status         QuotaRequestStatus `json:"status" bson:"status"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt"`
	// DecidedAt se postavlja kad administrator odluči (US-23); do tada je nil.
	DecidedAt *time.Time `json:"decidedAt,omitempty" bson:"decidedAt,omitempty"`
}
