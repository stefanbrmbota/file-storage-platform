package handler

import (
	"context"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
)

// ctxKey je privatni tip ključa, pa vrednost uloge u kontekstu ne može
// biti upisana ni pročitana mimo ovog paketa.
type ctxKey struct{}

var roleKey ctxKey

// ContextWithRole vraća kontekst sa ulogom autentifikovanog korisnika.
// Poziva ga middleware za prijavu nakon provere tokena.
func ContextWithRole(ctx context.Context, role model.Role) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

// RoleFromContext vraća ulogu iz konteksta; ok je false ako uloga nije postavljena.
func RoleFromContext(ctx context.Context) (model.Role, bool) {
	role, ok := ctx.Value(roleKey).(model.Role)
	return role, ok
}
