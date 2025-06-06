package ratelimit

import (
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.RWMutex
	limits map[string]*bucketSet
}

type bucketSet struct {
	requests []time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		limits: make(map[string]*bucketSet),
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
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

	now := time.Now()
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

	now := time.Now()
	cutoff := now.Add(-window)

	count := 0
	for _, reqTime := range bucket.requests {
		if reqTime.After(cutoff) {
			count++
		}
	}

	return count
}

func NormalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	localPart := parts[0]
	domain := parts[1]

	if domain == "gmail.com" || domain == "googlemail.com" {
		localPart = strings.ReplaceAll(localPart, ".", "")
		if plusIdx := strings.Index(localPart, "+"); plusIdx != -1 {
			localPart = localPart[:plusIdx]
		}
		domain = "gmail.com"
	} else {
		if plusIdx := strings.Index(localPart, "+"); plusIdx != -1 {
			localPart = localPart[:plusIdx]
		}
	}

	return localPart + "@" + domain
}
