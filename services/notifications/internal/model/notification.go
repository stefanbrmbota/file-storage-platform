// Package model su entiteti Notifications servisa.
package model

import "time"

// Notification je jedno obavestenje korisnika (design 3.3, tabela
// notifications_by_user). Vreme nastanka se cita iz notification_id
// (timeuuid), pa nema posebne kolone.
type Notification struct {
	ID           string    `json:"id"` // timeuuid kao tekst
	Type         string    `json:"type"`
	Message      string    `json:"message"`
	ActorID      string    `json:"actorId,omitempty"`
	ResourceID   string    `json:"resourceId,omitempty"`
	ResourceType string    `json:"resourceType,omitempty"`
	IsRead       bool      `json:"isRead"`
	CreatedAt    time.Time `json:"createdAt"`
}
