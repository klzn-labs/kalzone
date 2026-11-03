package ratelimit

import (
	"strings"
	"sync"
	"time"
)

const forgetAfter = time.Hour

type entry struct {
	tokens float64
	last   time.Time
	seen   time.Time
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
	return l.Take(key)
}

func (l *Limiter) Take(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.refilled(key, time.Now())
	if e.tokens < 1 {
		return false
	}
	e.tokens--
	return true
}

func (l *Limiter) Refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.refilled(key, time.Now())
	e.tokens = min(e.tokens+1, float64(l.perHour))
}

func (l *Limiter) Clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for existing := range l.keys {
		if existing == key || strings.HasPrefix(existing, key+"|") {
			delete(l.keys, existing)
		}
	}
}

func (l *Limiter) refilled(key string, now time.Time) *entry {
	l.forget(now)
	e, ok := l.keys[key]
	if !ok {
		e = &entry{tokens: float64(l.perHour), last: now}
		l.keys[key] = e
	}
	perToken := time.Hour / time.Duration(l.perHour)
	e.tokens = min(e.tokens+float64(now.Sub(e.last))/float64(perToken), float64(l.perHour))
	e.last, e.seen = now, now
	return e
}

func (l *Limiter) forget(now time.Time) {
	for key, e := range l.keys {
		if now.Sub(e.seen) > forgetAfter {
			delete(l.keys, key)
		}
	}
}
