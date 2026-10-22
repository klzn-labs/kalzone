package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const forgetAfter = time.Hour

type entry struct {
	limiter *rate.Limiter
	seen    time.Time
}

type Limiter struct {
	mu      sync.Mutex
	perHour int
	keys    map[string]*entry
}

func PerHour(n int) *Limiter {
	return &Limiter{perHour: n, keys: map[string]*entry{}}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.forget(now)
	e, ok := l.keys[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(rate.Every(time.Hour/time.Duration(l.perHour)), l.perHour)}
		l.keys[key] = e
	}
	e.seen = now
	return e.limiter.Allow()
}

func (l *Limiter) forget(now time.Time) {
	for key, e := range l.keys {
		if now.Sub(e.seen) > forgetAfter {
			delete(l.keys, key)
		}
	}
}
