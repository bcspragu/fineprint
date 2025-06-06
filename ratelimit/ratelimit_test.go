package ratelimit

import (
	"testing"
	"time"
)

func TestRateLimiter_IsAllowed(t *testing.T) {
	rl := NewRateLimiter()

	key := "test-key"
	limit := 3
	window := time.Minute

	for i := range limit {
		if !rl.IsAllowed(key, limit, window) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if rl.IsAllowed(key, limit, window) {
		t.Error("Request beyond limit should not be allowed")
	}

	if count := rl.GetCurrentCount(key, limit, window); count != limit {
		t.Errorf("Expected count %d, got %d", limit, count)
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	rl := NewRateLimiter()

	key := "test-window"
	limit := 2
	window := 100 * time.Millisecond

	for i := range limit {
		if !rl.IsAllowed(key, limit, window) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if rl.IsAllowed(key, limit, window) {
		t.Error("Request beyond limit should not be allowed")
	}

	time.Sleep(window + 10*time.Millisecond)

	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request should be allowed after window reset")
	}
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Test@Example.com", "test@example.com"},
		{"user+tag@gmail.com", "user@gmail.com"},
		{"us.er+tag@gmail.com", "user@gmail.com"},
		{"user@googlemail.com", "user@gmail.com"},
		{"user+tag@otherdomain.com", "user@otherdomain.com"},
		{" user@domain.com ", "user@domain.com"},
	}

	for _, test := range tests {
		result := NormalizeEmail(test.input)
		if result != test.expected {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter()

	limit := 1
	window := time.Minute

	if !rl.IsAllowed("key1", limit, window) {
		t.Error("First request for key1 should be allowed")
	}

	if !rl.IsAllowed("key2", limit, window) {
		t.Error("First request for key2 should be allowed")
	}

	if rl.IsAllowed("key1", limit, window) {
		t.Error("Second request for key1 should not be allowed")
	}

	if rl.IsAllowed("key2", limit, window) {
		t.Error("Second request for key2 should not be allowed")
	}
}
