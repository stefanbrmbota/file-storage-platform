package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/config"
)

func TestParseCursor(t *testing.T) {
	v1 := gocql.UUIDFromTime(time.Now()).String()
	if _, err := parseCursor(v1); err != nil {
		t.Errorf("timeuuid odbijen: %v", err)
	}
	for _, bad := range []string{"abc", "' OR 1=1 --", "550e8400-e29b-41d4-a716-446655440000" /* v4 */} {
		if _, err := parseCursor(bad); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%q: ocekivan ErrInvalidCursor, dobijeno %v", bad, err)
		}
	}
}

// TestListAndCount radi samo sa pravom Cassandrom (vidi TestConnectAndMigrate).
func TestListAndCount(t *testing.T) {
	hosts := os.Getenv("NOTIFICATIONS_TEST_CASSANDRA_HOSTS")
	if hosts == "" {
		t.Skip("NOTIFICATIONS_TEST_CASSANDRA_HOSTS nije postavljen")
	}
	cfg := config.Cassandra{
		Hosts: strings.Split(hosts, ","), Keyspace: "notifications_it_list", ReplicationFactor: 1,
		Timeout: 10 * time.Second, StartupTimeout: 30 * time.Second,
	}
	ctx := context.Background()
	s, err := Connect(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	st := NewStore(s, cfg.Timeout)
	defer func() {
		_ = s.Query("DROP KEYSPACE IF EXISTS notifications_it_list").Exec()
		st.Close()
	}()

	// u1: 5 obavestenja u razmaku od minut, prva 3 (najstarija) neprocitana; u2: 2.
	base := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	insert := func(user string, i int, unread bool) {
		id := gocql.UUIDFromTime(base.Add(time.Duration(i) * time.Minute))
		err := s.Query(`INSERT INTO notifications_by_user (user_id, notification_id, type, message, is_read) VALUES (?, ?, ?, ?, ?)`,
			user, id, "SHARE_GRANTED", fmt.Sprintf("%s poruka %d", user, i), !unread).Exec()
		if err == nil && unread {
			err = s.Query(`INSERT INTO unread_by_user (user_id, notification_id) VALUES (?, ?)`, user, id).Exec()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		insert("u1", i, i < 3)
	}
	insert("u2", 0, true)
	insert("u2", 1, true)

	// Prva stranica: najnovija dva.
	page, err := st.ListByUser(ctx, "u1", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Message != "u1 poruka 4" || page[1].Message != "u1 poruka 3" {
		t.Fatalf("prva stranica: %+v", page)
	}
	if !page[0].CreatedAt.Equal(base.Add(4 * time.Minute)) {
		t.Errorf("vreme iz timeuuid = %v", page[0].CreatedAt)
	}

	// Sledeca stranica od poslednjeg ID-ja.
	next, err := st.ListByUser(ctx, "u1", page[1].ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 3 || next[0].Message != "u1 poruka 2" || next[2].Message != "u1 poruka 0" {
		t.Fatalf("druga stranica: %+v", next)
	}
	for _, n := range append(page, next...) {
		if !strings.HasPrefix(n.Message, "u1 ") {
			t.Errorf("tudje obavestenje u listi u1: %+v", n)
		}
	}

	if n, err := st.CountUnread(ctx, "u1"); err != nil || n != 3 {
		t.Errorf("neprocitana u1 = %d, err = %v", n, err)
	}
	if n, err := st.CountUnread(ctx, "nepostojeci"); err != nil || n != 0 {
		t.Errorf("neprocitana za nepostojeceg = %d, err = %v", n, err)
	}
	if _, err := st.ListByUser(ctx, "u1", "nije-uuid", 10); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("los kursor: %v", err)
	}
}
