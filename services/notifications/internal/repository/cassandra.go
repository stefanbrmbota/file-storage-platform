// Package repository je pristup Cassandri za Notifications servis (US-07,
// design poglavlje 3).
package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"

	"github.com/stefanbrmbota/file-storage-platform/services/notifications/internal/config"
)

// retryPause je pauza izmedju pokusaja povezivanja pri pokretanju.
const retryPause = 3 * time.Second

// Connect se povezuje na Cassandru, pravi keyspace i tabele ako ne postoje
// (Migrate) i vraca sesiju vezanu za keyspace servisa.
//
// Cassandra se sporo podize, pa Connect pokusava ponovo dok ne istekne
// cfg.StartupTimeout ili ctx.
func Connect(ctx context.Context, cfg config.Cassandra, logger *slog.Logger) (*gocql.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		s, err := connectOnce(ctx, cfg)
		if err == nil {
			return s, nil
		}
		logger.Warn("povezivanje na Cassandru nije uspelo, novi pokusaj",
			"attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("cassandra: povezivanje nije uspelo za %v: %w", cfg.StartupTimeout, err)
		case <-time.After(retryPause):
		}
	}
}

func connectOnce(ctx context.Context, cfg config.Cassandra) (*gocql.Session, error) {
	// Prva sesija bez keyspace-a, jer keyspace mozda jos ne postoji.
	boot, err := newCluster(cfg, "").CreateSession()
	if err != nil {
		return nil, err
	}
	err = Migrate(ctx, boot, cfg.Keyspace, cfg.ReplicationFactor, cfg.Timeout)
	boot.Close()
	if err != nil {
		return nil, err
	}
	return newCluster(cfg, cfg.Keyspace).CreateSession()
}

func newCluster(cfg config.Cassandra, keyspace string) *gocql.ClusterConfig {
	c := gocql.NewCluster(cfg.Hosts...)
	c.Keyspace = keyspace
	c.Timeout = cfg.Timeout
	c.ConnectTimeout = cfg.Timeout
	// Jedan cvor lokalno; u K8s sa faktorom 3 QUORUM trazi vecinu replika.
	c.Consistency = gocql.LocalQuorum
	return c
}

// Migrate pravi keyspace i tabele iz design 3.2-3.4 ako ne postoje. Bezbedno
// je pokrenuti ga vise puta (IF NOT EXISTS).
func Migrate(ctx context.Context, s *gocql.Session, keyspace string, replicationFactor int, timeout time.Duration) error {
	for _, stmt := range SchemaStatements(keyspace, replicationFactor) {
		qctx, cancel := context.WithTimeout(ctx, timeout)
		err := s.Query(stmt).ExecContext(qctx)
		cancel()
		if err != nil {
			return fmt.Errorf("cassandra: sema: %w", err)
		}
	}
	// Sacekaj da svi cvorovi vide nove tabele pre prvog upita.
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.AwaitSchemaAgreement(actx); err != nil {
		return fmt.Errorf("cassandra: usaglasavanje seme: %w", err)
	}
	return nil
}

// SchemaStatements vraca CQL za keyspace i tabele (design 3.2-3.4).
//
// Ime keyspace-a i faktor replikacije ne mogu biti parametri DDL upita, pa se
// ugradjuju u tekst; oba dolaze iz konfiguracije, a config.Load proverava da
// je ime samo [a-z0-9_] i da je faktor >= 1. Korisnicki ulaz nikad ne ide ovde.
func SchemaStatements(keyspace string, replicationFactor int) []string {
	return []string{
		fmt.Sprintf(`CREATE KEYSPACE IF NOT EXISTS %s
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': %d}`, keyspace, replicationFactor),

		// Sva obavestenja jednog korisnika su u jednoj particiji (user_id),
		// sortirana od najnovijeg (timeuuid DESC). TTL 90 dana.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.notifications_by_user (
    user_id         text,
    notification_id timeuuid,
    type            text,
    message         text,
    actor_id        text,
    resource_id     text,
    resource_type   text,
    is_read         boolean,
    PRIMARY KEY ((user_id), notification_id)
) WITH CLUSTERING ORDER BY (notification_id DESC)
  AND default_time_to_live = 7776000`, keyspace),

		// Red postoji dok je obavestenje neprocitano; broj neprocitanih je
		// brojanje u jednoj particiji.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.unread_by_user (
    user_id         text,
    notification_id timeuuid,
    PRIMARY KEY ((user_id), notification_id)
) WITH default_time_to_live = 7776000`, keyspace),
	}
}

// ErrUnavailable znaci da baza ne odgovara.
var ErrUnavailable = errors.New("cassandra: baza nije dostupna")

// Store drzi sesiju ka Cassandri.
type Store struct {
	session *gocql.Session
	timeout time.Duration
}

// NewStore pravi Store nad postojecom sesijom.
func NewStore(s *gocql.Session, timeout time.Duration) *Store {
	return &Store{session: s, timeout: timeout}
}

// Ping proverava da baza odgovara jednim laganim upitom.
func (st *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()
	var v string
	if err := st.session.Query(`SELECT release_version FROM system.local`).ScanContext(ctx, &v); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// Close zatvara sesiju.
func (st *Store) Close() {
	st.session.Close()
}
