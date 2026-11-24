package repository

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/config"
)

func TestSchemaStatements(t *testing.T) {
	stmts := SchemaStatements("notif_test", 3)
	if len(stmts) != 3 {
		t.Fatalf("ocekivane 3 naredbe, dobijeno %d", len(stmts))
	}
	ks, notif, unread := stmts[0], stmts[1], stmts[2]

	if !strings.Contains(ks, "CREATE KEYSPACE IF NOT EXISTS notif_test") || !strings.Contains(ks, "'replication_factor': 3") {
		t.Errorf("keyspace: %s", ks)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS notif_test.notifications_by_user",
		"PRIMARY KEY ((user_id), notification_id)",   // particija po korisniku
		"CLUSTERING ORDER BY (notification_id DESC)", // najnovije prvo
		"notification_id timeuuid",
		"default_time_to_live = 7776000",
	} {
		if !strings.Contains(notif, want) {
			t.Errorf("notifications_by_user nema %q", want)
		}
	}
	if !strings.Contains(unread, "CREATE TABLE IF NOT EXISTS notif_test.unread_by_user") ||
		!strings.Contains(unread, "PRIMARY KEY ((user_id), notification_id)") {
		t.Errorf("unread_by_user: %s", unread)
	}
}

// TestConnectAndMigrate radi samo sa pravom Cassandrom:
//
//	NOTIFICATIONS_TEST_CASSANDRA_HOSTS=localhost:9042 go test ./internal/repository/
//
// Pravi poseban keyspace i brise ga na kraju.
func TestConnectAndMigrate(t *testing.T) {
	hosts := os.Getenv("NOTIFICATIONS_TEST_CASSANDRA_HOSTS")
	if hosts == "" {
		t.Skip("NOTIFICATIONS_TEST_CASSANDRA_HOSTS nije postavljen")
	}
	cfg := config.Cassandra{
		Hosts: strings.Split(hosts, ","), Keyspace: "notifications_it", ReplicationFactor: 1,
		Timeout: 10 * time.Second, StartupTimeout: 30 * time.Second,
	}
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(s, cfg.Timeout)
	defer func() {
		_ = s.Query("DROP KEYSPACE IF EXISTS notifications_it").Exec()
		store.Close()
	}()

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// Drugo pokretanje ne sme da padne (IF NOT EXISTS).
	if err := Migrate(ctx, s, cfg.Keyspace, 1, cfg.Timeout); err != nil {
		t.Fatalf("ponovna migracija: %v", err)
	}

	var n int
	err = s.Query(`SELECT COUNT(*) FROM system_schema.tables WHERE keyspace_name = ? AND table_name IN ('notifications_by_user', 'unread_by_user')`,
		cfg.Keyspace).Scan(&n)
	if err != nil || n != 2 {
		t.Fatalf("tabele: n = %d, err = %v", n, err)
	}
}
