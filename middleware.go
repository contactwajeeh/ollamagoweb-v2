package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

var (
	limiters     = make(map[string]*rate.Limiter)
	limiterMu    sync.Mutex
	limiterRate  = rate.Limit(10)
	limiterBurst = 50
)

func getLimiter(ip string) *rate.Limiter {
	limiterMu.Lock()
	defer limiterMu.Unlock()

	if l, exists := limiters[ip]; exists {
		return l
	}

	l := rate.NewLimiter(limiterRate, limiterBurst)
	limiters[ip] = l
	return l
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = forwarded
		}

		limiter := getLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
