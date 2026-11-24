package repository

import (
	"context"
	"errors"
	"fmt"

	gocql "github.com/apache/cassandra-gocql-driver/v2"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/model"
)

// ErrInvalidCursor znaci da kursor za sledecu stranicu nije ispravan timeuuid.
var ErrInvalidCursor = errors.New("repository: neispravan kursor")

const (
	listNewest = `SELECT notification_id, type, message, actor_id, resource_id, resource_type, is_read
FROM notifications_by_user WHERE user_id = ? LIMIT ?`

	listBefore = `SELECT notification_id, type, message, actor_id, resource_id, resource_type, is_read
FROM notifications_by_user WHERE user_id = ? AND notification_id < ? LIMIT ?`

	countUnread = `SELECT COUNT(*) FROM unread_by_user WHERE user_id = ?`
)

// ListByUser vraca najvise limit obavestenja korisnika, od najnovijeg (upit
// N1, design 3.3). Ako before nije prazan, vraca samo obavestenja starija od
// njega (kursor za sledecu stranicu = ID poslednjeg sa prethodne stranice).
//
// Upit cita samo particiju tog korisnika; sve vrednosti su parametri.
func (st *Store) ListByUser(ctx context.Context, userID, before string, limit int) ([]model.Notification, error) {
	var q *gocql.Query
	if before == "" {
		q = st.session.Query(listNewest, userID, limit)
	} else {
		cursor, err := parseCursor(before)
		if err != nil {
			return nil, err
		}
		q = st.session.Query(listBefore, userID, cursor, limit)
	}

	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	out := make([]model.Notification, 0, limit)
	scanner := q.IterContext(ctx).Scanner()
	for scanner.Next() {
		var (
			id gocql.UUID
			n  model.Notification
		)
		if err := scanner.Scan(&id, &n.Type, &n.Message, &n.ActorID, &n.ResourceID, &n.ResourceType, &n.IsRead); err != nil {
			return nil, fmt.Errorf("repository: citanje obavestenja: %w", err)
		}
		n.ID = id.String()
		n.CreatedAt = id.Time().UTC()
		out = append(out, n)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("repository: lista obavestenja: %w", err)
	}
	return out, nil
}

// CountUnread vraca broj neprocitanih obavestenja korisnika (upit N2, design 3.4).
func (st *Store) CountUnread(ctx context.Context, userID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	var n int64
	if err := st.session.Query(countUnread, userID).ScanContext(ctx, &n); err != nil {
		return 0, fmt.Errorf("repository: broj neprocitanih: %w", err)
	}
	return int(n), nil
}

// parseCursor prihvata samo timeuuid (UUID verzije 1), kakav je notification_id.
func parseCursor(s string) (gocql.UUID, error) {
	u, err := gocql.ParseUUID(s)
	if err != nil || u.Version() != 1 {
		return gocql.UUID{}, ErrInvalidCursor
	}
	return u, nil
}
