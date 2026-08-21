package app

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

type IPRateLimiter struct {
	mu      sync.Mutex
	rate    float64 // tokens added per second
	burst   float64
	clients map[string]*clientBucket
}

func NewIPRateLimiter(reqPerMinute int, burst int) *IPRateLimiter {
	if reqPerMinute <= 0 {
		reqPerMinute = 60
	}
	if burst <= 0 {
		burst = reqPerMinute / 6
		if burst < 5 {
			burst = 5
		}
	}
	return &IPRateLimiter{
		rate:    float64(reqPerMinute) / 60.0,
		burst:   float64(burst),
		clients: make(map[string]*clientBucket),
	}
}

func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.clients[ip]
	if !exists {
		l.clients[ip] = &clientBucket{
			tokens:     l.burst - 1,
			lastUpdate: now,
		}
		// Periodically clean up stale client entries
		if len(l.clients) > 2000 {
			for k, v := range l.clients {
				if now.Sub(v.lastUpdate) > 10*time.Minute {
					delete(l.clients, k)
				}
			}
		}
		return true
	}

	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.lastUpdate = now
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

func extractClientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	return remoteHost
}
