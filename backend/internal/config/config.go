package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	AdminJWTSecret string
	AllowedOrigins []string
	LogLevel       string

	// Web registration (vshage.app/e/<slug>).
	//
	// WebregAdminToken gates event-config upserts. Left empty the admin
	// endpoint refuses every request — fail closed, so a stand that forgot
	// the secret cannot be reconfigured by anyone who finds the URL.
	WebregAdminToken string

	// TGInFeed подмешивает студсобытия в общую ленту афиши. Флаг, а не
	// константа, потому что знание про их id живёт в ДВУХ образах: бэкенд
	// начинает отдавать id вида ev_*, а открывать их умеет только новый
	// фронт. Прод-выкатка посервисная — выкатив бэкенд первым, мы получили
	// бы ленту, где каждая импортированная карточка ведёт в 404, и заметил
	// бы это посетитель, а не мы. С флагом порядок выкатки перестаёт быть
	// тихим условием правильности: оба образа едут выключенными, флаг
	// поднимается третьим шагом и опускается за секунду, если что-то не так.
	TGInFeed bool
	// WebregLogPath optionally mirrors submissions to a file; stdout always
	// gets them regardless.
	WebregLogPath string
	// WebregIPSalt salts stored IP hashes. Empty ⇒ a random per-process salt
	// (hashes then stop being comparable across restarts, which is fine —
	// they only serve abuse triage, never identification).
	WebregIPSalt string
}

func Load() (*Config, error) {
	c := &Config{
		Port:           envDefault("PORT", "3003"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       envDefault("REDIS_URL", "redis://redis:6379/0"),
		AdminJWTSecret: os.Getenv("ADMIN_JWT_SECRET"),
		AllowedOrigins: splitCSV(envDefault("ALLOWED_ORIGINS", "https://afisha.vshage.app,https://afisha-dev.vshage.app,http://localhost:5173")),
		LogLevel:       envDefault("LOG_LEVEL", "info"),

		WebregAdminToken: os.Getenv("WEBREG_ADMIN_TOKEN"),
		TGInFeed:         os.Getenv("AFISHA_TG_IN_FEED") == "1",
		WebregLogPath:    os.Getenv("WEBREG_LOG_PATH"),
		WebregIPSalt:     os.Getenv("WEBREG_IP_SALT"),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL required")
	}
	if c.AdminJWTSecret == "" {
		return nil, fmt.Errorf("ADMIN_JWT_SECRET required")
	}
	return c, nil
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
