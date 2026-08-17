package webreg

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Изоляция кабинетов — единственное, ради чего появился owner_slug, и до сих
// пор она держалась только на ручной проверке. Здесь она закреплена: чужое
// событие не читается, не переписывается, не отдаёт список гостей и не даёт
// перевыпустить ключ организатора.
//
// Тест идёт по живой базе: правила живут в SQL (предикат владельца в WHERE,
// фильтр в ON CONFLICT), и мок проверял бы не их, а свою имитацию. Без
// TEST_DATABASE_URL пропускается — как и соседний tests/events_test.go.
func ownerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedEvent(t *testing.T, pool *pgxpool.Pool, slug, title string, owner *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO webreg_events (slug, title, starts_at, timezone, owner_slug, manage_key_hash)
		VALUES ($1, $2, NOW() + INTERVAL '30 days', 'Europe/Moscow', $3, 'seed')
		ON CONFLICT (slug) DO UPDATE SET title = EXCLUDED.title, owner_slug = EXCLUDED.owner_slug
	`, slug, title, owner)
	if err != nil {
		t.Fatalf("seed %s: %v", slug, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM webreg_events WHERE slug = $1`, slug)
	})
}

func titleOf(t *testing.T, pool *pgxpool.Pool, slug string) string {
	t.Helper()
	var title string
	if err := pool.QueryRow(context.Background(),
		`SELECT title FROM webreg_events WHERE slug = $1`, slug).Scan(&title); err != nil {
		t.Fatalf("read title %s: %v", slug, err)
	}
	return title
}

func TestOwnerIsolation(t *testing.T) {
	pool := ownerTestPool(t)
	repo := NewRepository(pool, "test-salt")
	ctx := context.Background()

	mine := "ownertest-a"
	other := "ownertest-b"
	platform := "ownertest-platform" // owner_slug IS NULL — событие площадки
	seedEvent(t, pool, mine, "Моё событие", &[]string{"cab-a"}[0])
	seedEvent(t, pool, other, "Чужое событие", &[]string{"cab-b"}[0])
	seedEvent(t, pool, platform, "Событие площадки", nil)

	cabA := "cab-a"

	t.Run("список отдаёт только свои", func(t *testing.T) {
		list, err := repo.ListEvents(ctx, &cabA)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, e := range list {
			if e.Slug == other || e.Slug == platform {
				t.Fatalf("в списке кабинета оказалось чужое событие %q", e.Slug)
			}
		}
		var found bool
		for _, e := range list {
			if e.Slug == mine {
				found = true
			}
		}
		if !found {
			t.Fatalf("своё событие %q в списке не найдено", mine)
		}
	})

	t.Run("админ видит и чужие, и бесхозные", func(t *testing.T) {
		list, err := repo.ListEvents(ctx, nil)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		seen := map[string]bool{}
		for _, e := range list {
			seen[e.Slug] = true
		}
		for _, slug := range []string{mine, other, platform} {
			if !seen[slug] {
				t.Fatalf("админ не видит %q", slug)
			}
		}
	})

	t.Run("чужой конфиг не читается", func(t *testing.T) {
		if _, err := repo.GetEventConfig(ctx, other, &cabA); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("ждали ErrNoRows на чужом событии, получили %v", err)
		}
		if _, err := repo.GetEventConfig(ctx, platform, &cabA); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("бесхозное событие не должно читаться кабинетом, получили %v", err)
		}
	})

	t.Run("чужой список гостей не отдаётся", func(t *testing.T) {
		if _, err := repo.AdminList(ctx, other, &cabA); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("ждали ErrNoRows, получили %v", err)
		}
	})

	t.Run("чужой ключ организатора не перевыпускается", func(t *testing.T) {
		if err := repo.SetManageKey(ctx, other, "new-key", &cabA); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("ждали ErrNoRows, получили %v", err)
		}
	})

	t.Run("чужой слаг не перезаписывается апсертом", func(t *testing.T) {
		err := repo.UpsertEvent(ctx, &EventUpsert{
			Slug:     other,
			Title:    "ЗАХВАТ",
			StartsAt: time.Now().Add(48 * time.Hour),
		}, &cabA)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
			t.Fatalf("ждали 409 slug_taken, получили %v", err)
		}
		if got := titleOf(t, pool, other); got != "Чужое событие" {
			t.Fatalf("чужое событие изменилось: %q", got)
		}
	})

	t.Run("админская правка не отбирает владельца", func(t *testing.T) {
		err := repo.UpsertEvent(ctx, &EventUpsert{
			Slug:     mine,
			Title:    "Правка админом",
			StartsAt: time.Now().Add(72 * time.Hour),
		}, nil)
		if err != nil {
			t.Fatalf("админский апсерт: %v", err)
		}
		var owner *string
		if err := pool.QueryRow(ctx,
			`SELECT owner_slug FROM webreg_events WHERE slug = $1`, mine).Scan(&owner); err != nil {
			t.Fatalf("read owner: %v", err)
		}
		if owner == nil || *owner != cabA {
			t.Fatalf("владелец потерян после админской правки: %v", owner)
		}
	})
}
