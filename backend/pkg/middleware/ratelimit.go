package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimit is a per-IP token bucket for anonymous write endpoints.
//
// Calibration note — this limiter is deliberately LOOSE. Russian mobile
// carriers put thousands of subscribers behind one CGNAT address, so a tight
// per-IP cap does not stop an attacker (who rotates addresses) while it does
// silently drop real attendees arriving from one carrier during a Telegram
// announcement. Rejecting a real registration is the expensive failure here;
// a handful of junk rows the organizer can see and ignore is the cheap one.
// Duplicate submissions are handled separately by a unique index, not here.
//
// rps is the sustained refill rate per second, burst the bucket size.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	l := &limiter{
		rps:     rps,
		burst:   float64(burst),
		buckets: map[string]*bucket{},
	}
	go l.sweep()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := ipKey(r)
			if key == "" {
				// Unkeyable: no forwarded client address and the peer is on a
				// private network — i.e. our own SSR frontend calling in
				// without ADDRESS_HEADER wired up. Bucketing those together
				// would throttle the entire audience as one visitor, so this
				// degrades to "no limit" rather than "limit everyone".
				// Public traffic always carries X-Forwarded-For from Caddy.
				next.ServeHTTP(w, r)
				return
			}
			if !l.allow(key) {
				log.Printf("webreg: rate limit hit for %s on %s", key, r.URL.Path)
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":"rate_limited","message":"Слишком много запросов — подожди пару секунд"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	tokens float64
	seen   time.Time
}

type limiter struct {
	rps   float64
	burst float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

func (l *limiter) allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, seen: now}
		return true
	}
	b.tokens += now.Sub(b.seen).Seconds() * l.rps
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweep drops idle buckets so the map cannot grow without bound.
func (l *limiter) sweep() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		l.mu.Lock()
		for k, b := range l.buckets {
			if b.seen.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// ipKey resolves the bucket key. Empty means "cannot attribute this request to
// a visitor" — the caller must then let it through (see the note above).
func ipKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	if isPrivate(host) {
		return ""
	}
	return host
}

// isPrivate reports whether addr is on a private/loopback network, i.e. our own
// compose network rather than a visitor.
func isPrivate(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		// An unparseable address is not something we can fairly bucket.
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
