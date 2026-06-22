package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	perIP    map[string]*ipCounter
	maxReq   int
	window   time.Duration
	sessions int
	maxS     int
}

type ipCounter struct {
	start time.Time
	count int
}

func NewRateLimiter(maxReqPerMin, maxSessions int) *RateLimiter {
	return &RateLimiter{perIP: map[string]*ipCounter{}, maxReq: maxReqPerMin, window: time.Minute, maxS: maxSessions}
}

func (r *RateLimiter) AllowRequest(req *http.Request) bool {
	if r.maxReq <= 0 {
		return true
	}
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	if host == "" {
		host = req.RemoteAddr
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.perIP[host]
	if c == nil || now.Sub(c.start) >= r.window {
		r.perIP[host] = &ipCounter{start: now, count: 1}
		return true
	}
	if c.count >= r.maxReq {
		return false
	}
	c.count++
	return true
}

func (r *RateLimiter) AcquireSession() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxS > 0 && r.sessions >= r.maxS {
		return false
	}
	r.sessions++
	return true
}

func (r *RateLimiter) ReleaseSession() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions > 0 {
		r.sessions--
	}
}
