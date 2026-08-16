package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPKey(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"public peer", "203.0.113.9:5555", "", "203.0.113.9"},
		{"forwarded visitor", "172.18.0.4:5555", "203.0.113.9", "203.0.113.9"},
		{"forwarded chain takes the client", "172.18.0.4:5555", "203.0.113.9, 10.0.0.1", "203.0.113.9"},

		// Our own SSR frontend calling in without a forwarded address. Bucketing
		// these together would throttle the whole audience as one visitor, so
		// the key must be empty and the caller must let them through.
		{"private peer, no XFF", "172.18.0.4:5555", "", ""},
		{"loopback, no XFF", "127.0.0.1:5555", "", ""},
		{"ipv6 loopback, no XFF", "[::1]:5555", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/e/x/register", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := ipKey(r); got != tc.want {
				t.Fatalf("ipKey(remote=%q, xff=%q) = %q, want %q",
					tc.remoteAddr, tc.xff, got, tc.want)
			}
		})
	}
}

// An unattributable request must never be blocked: losing a real registration
// costs more than letting a junk row through.
func TestRateLimit_UnkeyableRequestsAreNeverBlocked(t *testing.T) {
	h := RateLimit(1, 2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 50; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/e/x/register", nil)
		r.RemoteAddr = "172.18.0.4:5555" // SSR frontend, no XFF
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d blocked with %d — unkeyable traffic must pass", i, w.Code)
		}
	}
}

func TestRateLimit_BlocksOneNoisyVisitor(t *testing.T) {
	h := RateLimit(1, 3)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(ip string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/e/x/register", nil)
		r.RemoteAddr = "172.18.0.4:5555"
		r.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	for i := 0; i < 3; i++ {
		if code := send("203.0.113.9"); code != http.StatusOK {
			t.Fatalf("burst request %d got %d, want 200", i, code)
		}
	}
	if code := send("203.0.113.9"); code != http.StatusTooManyRequests {
		t.Fatalf("over-budget request got %d, want 429", code)
	}
	// A different visitor has their own budget and must be unaffected.
	if code := send("203.0.113.10"); code != http.StatusOK {
		t.Fatalf("second visitor got %d, want 200 — buckets must be per-IP", code)
	}
}
