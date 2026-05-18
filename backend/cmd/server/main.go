package main

import (
	"context"
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
	"github.com/pythonjsgo/vshage-afisha/pkg/db"
	"github.com/pythonjsgo/vshage-afisha/pkg/middleware"
)

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
	} else if err := db.RunMigrations(ctx, pool, db.FindMigrationsDir()); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	cache, err := events.NewCache(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}

	evRepo := events.NewRepository(pool)
	evHandler := events.NewHandler(evRepo, cache)
	adminHandler := admin.NewHandler(pool)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(middleware.CORS(cfg.AllowedOrigins))

	r.Get("/healthz", health.Handler())

	r.Route("/api", func(r chi.Router) {
		r.Get("/events", evHandler.List)
		r.Get("/events/{id}", evHandler.GetByID)
		r.Post("/events/{id}/registrations", evHandler.RegisterPublic)

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
