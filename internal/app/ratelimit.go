package app

import (
	"math"
	"sync"
	"time"
)

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// IPRateLimiter provides thread-safe token bucket rate limiting per client IP.
type IPRateLimiter struct {
	mu      sync.Mutex
	rate    float64 // tokens added per second
	burst   float64
	clients map[string]*clientBucket
}

// NewIPRateLimiter initializes a rate limiter given requests per minute and max burst size.
// Returns nil if reqPerMinute <= 0 to indicate that rate limiting is disabled.
func NewIPRateLimiter(reqPerMinute int, burst int) *IPRateLimiter {
	if reqPerMinute <= 0 {
		return nil
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

// Allow checks whether an operation from client IP is allowed.
// Returns allowed boolean and the recommended Retry-After duration when rate-limited.
func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) {
	if l == nil || l.rate <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.clients[ip]
	if !exists {
		l.clients[ip] = &clientBucket{
			tokens:     l.burst - 1,
			lastUpdate: now,
		}
		// Periodically clean up stale client entries to prevent memory growth
		if len(l.clients) > 2000 {
			for k, v := range l.clients {
				if now.Sub(v.lastUpdate) > 10*time.Minute {
					delete(l.clients, k)
				}
			}
		}
		return true, 0
	}

	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.lastUpdate = now
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	needed := 1.0 - b.tokens
	retrySeconds := needed / l.rate
	if retrySeconds < 1.0 {
		retrySeconds = 1.0
	}
	return false, time.Duration(math.Ceil(retrySeconds)) * time.Second
}
