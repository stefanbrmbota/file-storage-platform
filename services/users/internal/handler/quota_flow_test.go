package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/handler"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/model"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
)

const flowToken = "flow-test-internal-token-at-least-32-chars"

// newFlowEnv sklapa istu putanju kao main.go (zastita, handler, service, pravi
// MongoDB repozitorijum) na privremenoj bazi. Bez MongoDB-a test se preskace.
func newFlowEnv(t *testing.T) (*http.ServeMux, *repository.MongoUserRepository) {
	t.Helper()
	uri := os.Getenv("USERS_MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("MongoDB nije dostupan: %v", err)
	}
	db := client.Database(fmt.Sprintf("users_flow_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo, err := repository.NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	quota := service.NewQuotaService(repo)
	internalOnly, err := handler.RequireInternalToken(flowToken)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /internal/users/{id}/quota/reserve", internalOnly(handler.ReserveQuotaHandler(quota)))
	mux.Handle("POST /internal/users/{id}/quota/release", internalOnly(handler.ReleaseQuotaHandler(quota)))
	return mux, repo
}

func seedFlowUser(t *testing.T, repo *repository.MongoUserRepository, id string, quota int64) {
	t.Helper()
	u := model.NewUser(id, "Pera", "Peric", id+"@example.com", "hash", quota, time.Now().UTC())
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func callQuota(mux *http.ServeMux, op, userID string, bytes int64, token string) (int, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, "/internal/users/"+userID+"/quota/"+op,
		strings.NewReader(fmt.Sprintf(`{"bytes":%d}`, bytes)))
	if token != "" {
		req.Header.Set(handler.InternalTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func usedBytes(t *testing.T, repo *repository.MongoUserRepository, id string) int64 {
	t.Helper()
	u, err := repo.FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u.UsedBytes
}

func TestQuotaFlow_ReserveReleaseReserve(t *testing.T) {
	mux, repo := newFlowEnv(t)
	seedFlowUser(t, repo, "u1", 1000)

	code, body := callQuota(mux, "reserve", "u1", 600, flowToken)
	if code != http.StatusOK || body["remainingBytes"] != float64(400) {
		t.Fatalf("reserve 600: status = %d, telo = %v", code, body)
	}
	code, body = callQuota(mux, "release", "u1", 200, flowToken)
	if code != http.StatusOK || body["usedBytes"] != float64(400) {
		t.Fatalf("release 200: status = %d, telo = %v", code, body)
	}
	code, body = callQuota(mux, "reserve", "u1", 600, flowToken)
	if code != http.StatusOK || body["remainingBytes"] != float64(0) {
		t.Fatalf("reserve 600 (do granice): status = %d, telo = %v", code, body)
	}
	if got := usedBytes(t, repo, "u1"); got != 1000 {
		t.Errorf("usedBytes = %d, ocekivano 1000", got)
	}
}

func TestQuotaFlow_ExceededLeavesStateUnchanged(t *testing.T) {
	mux, repo := newFlowEnv(t)
	seedFlowUser(t, repo, "u1", 1000)
	if code, _ := callQuota(mux, "reserve", "u1", 1000, flowToken); code != http.StatusOK {
		t.Fatalf("reserve do granice: status = %d", code)
	}

	code, body := callQuota(mux, "reserve", "u1", 1, flowToken)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, ocekivano 409", code)
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("odgovor mora imati error: %v", body)
	}
	if got := usedBytes(t, repo, "u1"); got != 1000 {
		t.Errorf("usedBytes = %d, ocekivano 1000 (bez promene)", got)
	}
}

func TestQuotaFlow_ReleaseNeverBelowZero(t *testing.T) {
	mux, repo := newFlowEnv(t)
	seedFlowUser(t, repo, "u1", 1000)
	if code, _ := callQuota(mux, "reserve", "u1", 100, flowToken); code != http.StatusOK {
		t.Fatalf("reserve: status = %d", code)
	}

	code, body := callQuota(mux, "release", "u1", 5000, flowToken)
	if code != http.StatusOK || body["usedBytes"] != float64(0) {
		t.Fatalf("status = %d, telo = %v", code, body)
	}
}

func TestQuotaFlow_UnknownUser(t *testing.T) {
	mux, _ := newFlowEnv(t)
	for _, op := range []string{"reserve", "release"} {
		if code, _ := callQuota(mux, op, "nema", 10, flowToken); code != http.StatusNotFound {
			t.Errorf("%s: status = %d, ocekivano 404", op, code)
		}
	}
}

func TestQuotaFlow_RejectedWithoutValidToken(t *testing.T) {
	mux, repo := newFlowEnv(t)
	seedFlowUser(t, repo, "u1", 1000)
	if code, _ := callQuota(mux, "reserve", "u1", 300, flowToken); code != http.StatusOK {
		t.Fatalf("reserve: status = %d", code)
	}

	for _, op := range []string{"reserve", "release"} {
		for _, token := range []string{"", "pogresan-token-pogresan-token-pogresan"} {
			if code, _ := callQuota(mux, op, "u1", 100, token); code != http.StatusUnauthorized {
				t.Errorf("%s sa tokenom %q: status = %d, ocekivano 401", op, token, code)
			}
		}
	}
	if got := usedBytes(t, repo, "u1"); got != 300 {
		t.Errorf("usedBytes = %d, ocekivano 300 (odbijeni pozivi ne smeju menjati stanje)", got)
	}
}

func TestQuotaFlow_ConcurrentReserveAndReleaseStayInBounds(t *testing.T) {
	mux, repo := newFlowEnv(t)
	const quota = 1000
	seedFlowUser(t, repo, "u1", quota)
	if code, _ := callQuota(mux, "reserve", "u1", 500, flowToken); code != http.StatusOK {
		t.Fatalf("reserve: status = %d", code)
	}

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			callQuota(mux, "reserve", "u1", 100, flowToken)
		}()
		go func() {
			defer wg.Done()
			callQuota(mux, "release", "u1", 100, flowToken)
		}()
	}
	wg.Wait()

	got := usedBytes(t, repo, "u1")
	if got < 0 || got > quota {
		t.Errorf("usedBytes = %d, mora biti u opsegu [0, %d]", got, quota)
	}
	if got%100 != 0 {
		t.Errorf("usedBytes = %d, ocekivan umnozak od 100", got)
	}
}
