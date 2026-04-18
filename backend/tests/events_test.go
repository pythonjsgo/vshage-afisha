package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pythonjsgo/vshage-afisha/internal/events"
	"github.com/pythonjsgo/vshage-afisha/pkg/db"
)

func TestListEvents_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	cache, _ := events.NewCache("redis://localhost:6379/15")
	h := events.NewHandler(events.NewRepository(pool), cache)

	r := chi.NewRouter()
	r.Get("/api/events", h.List)

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var result events.ListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total < 0 {
		t.Fatal("total must be >= 0")
	}
}
