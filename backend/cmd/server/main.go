package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/pythonjsgo/vshage-afisha/internal/admin"
	"github.com/pythonjsgo/vshage-afisha/internal/config"
	"github.com/pythonjsgo/vshage-afisha/internal/events"
	"github.com/pythonjsgo/vshage-afisha/internal/health"
	"github.com/pythonjsgo/vshage-afisha/internal/tgevents"
	"github.com/pythonjsgo/vshage-afisha/internal/webreg"
	"github.com/pythonjsgo/vshage-afisha/pkg/db"
	"github.com/pythonjsgo/vshage-afisha/pkg/middleware"
)

// randomSalt produces an ephemeral IP-hash salt when none is configured.
func randomSalt() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is fatal for anything security-adjacent; here
		// the salt only anonymises abuse-triage hashes, so degrade loudly
		// rather than refusing to boot the whole events board.
		log.Printf("webreg: crypto/rand unavailable for IP salt: %v", err)
		return "webreg-fallback-salt"
	}
	return hex.EncodeToString(b)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Apply DB migrations before serving. SKIP_MIGRATIONS=1 bypasses the
	// runner for local stands where the schema is provisioned out-of-band.
	if os.Getenv("SKIP_MIGRATIONS") == "1" {
		log.Print("SKIP_MIGRATIONS=1 set; skipping migration runner")
	} else if err := db.RunMigrations(ctx, pool, db.MigrationService, db.FindMigrationsDir()); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	cache, err := events.NewCache(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}

	evRepo := events.NewRepository(pool)
	evHandler := events.NewHandler(evRepo, cache)
	adminHandler := admin.NewHandler(pool)

	// Web registration (vshage.app/e/<slug>) — self-contained module, own
	// tables, no dependency on the shared events schema.
	ipSalt := cfg.WebregIPSalt
	if ipSalt == "" {
		ipSalt = randomSalt()
		log.Print("webreg: WEBREG_IP_SALT unset — using an ephemeral per-process salt")
	}
	if cfg.WebregAdminToken == "" {
		log.Print("webreg: WEBREG_ADMIN_TOKEN unset — event config endpoint is disabled (и импорт tg-events тоже)")
	}
	submitLog := webreg.NewSubmitLog(cfg.WebregLogPath)
	defer func() { _ = submitLog.Close() }()
	webregRepo := webreg.NewRepository(pool, ipSalt)
	webregHandler := webreg.NewHandler(webregRepo, cfg.WebregAdminToken, submitLog)

	// События веб-регистрации показываются и в общей ленте афиши.
	evHandler = evHandler.WithExtraSource(webregRepo)

	// Витрина событий из телеграм-каналов (конвейер vshage-geo). Свой
	// контур: /api/tg-events + страница /uni, в общую ленту не подмешивается
	// (см. пояснение в internal/tgevents). Админ-токен общий с webreg.
	tgRepo := tgevents.NewRepository(pool)
	tgHandler := tgevents.NewHandler(tgRepo, cfg.WebregAdminToken)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(middleware.CORS(cfg.AllowedOrigins))

	r.Get("/healthz", health.Handler())

	// Deliberately loose per-IP cap on the anonymous WRITE surface only — see
	// the calibration note on middleware.RateLimit. 1 rps sustained, burst 20.
	//
	// Reads are deliberately NOT limited. The event page is rendered by
	// afisha-frontend, so every read reaching this backend arrives from one
	// container address: a per-IP limiter on reads would throttle the entire
	// page for everyone the moment an announcement lands — the exact failure
	// the limiter exists to prevent. The frontend forwards the visitor's
	// address on writes (X-Forwarded-For), which is what makes the per-IP
	// bucket mean anything at all.
	webregLimit := middleware.RateLimit(1, 20)

	r.Route("/api", func(r chi.Router) {
		r.Get("/events", evHandler.List)
		r.Get("/events/{id}", evHandler.GetByID)
		r.Post("/events/{id}/registrations", evHandler.RegisterPublic)

		// Web registration: /api/e/{slug}, /api/e/{slug}/register, …
		r.Route("/e", func(r chi.Router) {
			webregHandler.Routes(r, webregLimit)
		})
		r.Route("/webreg/admin", webregHandler.AdminRoutes)

		r.Get("/tg-events", tgHandler.List)
		r.Put("/tg-events/admin/bulk", tgHandler.AdminBulkUpsert)

		r.Route("/admin", func(r chi.Router) {
			r.Use(admin.AuthMiddleware(cfg.AdminJWTSecret))
			r.Post("/featured", adminHandler.Feature)
			r.Post("/unfeature", adminHandler.Unfeature)
		})
	})

	log.Printf("afisha-backend listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
