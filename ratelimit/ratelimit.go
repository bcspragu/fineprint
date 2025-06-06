package ratelimit

import (
	"sync"
	"time"
)

type Time interface {
	Now() time.Time
	NewTicker(d time.Duration) *time.Ticker
}

type RealTimeProvider struct{}

func (RealTimeProvider) Now() time.Time {
	return time.Now()
}

func (RealTimeProvider) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

type RateLimiter struct {
	mu     sync.RWMutex
	limits map[string]*bucketSet
	time   Time
}

type bucketSet struct {
	requests []time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter() *RateLimiter {
	return NewRateLimiterWithTimeProvider(RealTimeProvider{})
}

func NewRateLimiterWithTimeProvider(timeProvider Time) *RateLimiter {
	rl := &RateLimiter{
		limits: make(map[string]*bucketSet),
		time:   timeProvider,
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := rl.time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := rl.time.Now()
		for key, bucket := range rl.limits {
			cutoff := now.Add(-bucket.window)
			var validRequests []time.Time
			for _, reqTime := range bucket.requests {
				if reqTime.After(cutoff) {
					validRequests = append(validRequests, reqTime)
				}
			}
			bucket.requests = validRequests
			if len(bucket.requests) == 0 {
				delete(rl.limits, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) IsAllowed(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.time.Now()
	cutoff := now.Add(-window)

	bucket, exists := rl.limits[key]
	if !exists {
		bucket = &bucketSet{
			requests: make([]time.Time, 0),
			limit:    limit,
			window:   window,
		}
		rl.limits[key] = bucket
	}

	var validRequests []time.Time
	for _, reqTime := range bucket.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	bucket.requests = validRequests

	if len(bucket.requests) >= limit {
		return false
	}

	bucket.requests = append(bucket.requests, now)
	return true
}

func (rl *RateLimiter) GetCurrentCount(key string, limit int, window time.Duration) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	bucket, exists := rl.limits[key]
	if !exists {
		return 0
	}

	now := rl.time.Now()
	cutoff := now.Add(-window)

	count := 0
	for _, reqTime := range bucket.requests {
		if reqTime.After(cutoff) {
			count++
		}
	}

	return count
}
