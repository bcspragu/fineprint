package ratelimit

import (
	"testing"
	"time"
)

type MockTimeProvider struct {
	currentTime time.Time
}

func (m *MockTimeProvider) Now() time.Time {
	return m.currentTime
}

func (m *MockTimeProvider) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

func (m *MockTimeProvider) Advance(d time.Duration) {
	m.currentTime = m.currentTime.Add(d)
}

func NewMockTimeProvider(initialTime time.Time) *MockTimeProvider {
	return &MockTimeProvider{currentTime: initialTime}
}

func TestRateLimiter_IsAllowed(t *testing.T) {
	mockTime := NewMockTimeProvider(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	rl := NewRateLimiterWithTimeProvider(mockTime)

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
	mockTime := NewMockTimeProvider(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	rl := NewRateLimiterWithTimeProvider(mockTime)

	key := "test-window"
	limit := 2
	window := time.Minute

	for i := range limit {
		if !rl.IsAllowed(key, limit, window) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if rl.IsAllowed(key, limit, window) {
		t.Error("Request beyond limit should not be allowed")
	}

	mockTime.Advance(window + time.Second)

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
	mockTime := NewMockTimeProvider(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	rl := NewRateLimiterWithTimeProvider(mockTime)

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

func TestRateLimiter_PartialWindowProgression(t *testing.T) {
	mockTime := NewMockTimeProvider(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	rl := NewRateLimiterWithTimeProvider(mockTime)

	key := "test-partial"
	limit := 3
	window := time.Hour

	// Make request 1 at t=0
	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request 1 should be allowed")
	}
	
	// Advance to t=10min and make request 2
	mockTime.Advance(10 * time.Minute)
	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request 2 should be allowed")
	}
	
	// Advance to t=20min and make request 3
	mockTime.Advance(10 * time.Minute)
	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request 3 should be allowed")
	}

	// At t=20min, we should be at the limit (requests at t=0, t=10min, t=20min)
	if rl.IsAllowed(key, limit, window) {
		t.Error("Request beyond limit should not be allowed")
	}

	// Advance to t=70min - requests at t=0 AND t=10min fall out of window
	// Window is now (t=10min, t=70min], so contains: t=20min (1 request)
	mockTime.Advance(50 * time.Minute)

	// Current count should be 1
	if count := rl.GetCurrentCount(key, limit, window); count != 1 {
		t.Errorf("Expected count 1 after first two requests expire, got %d", count)
	}

	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request should be allowed after first request falls out of window")
	}

	// At t=70min, window contains: t=20min, t=70min (2 requests)
	if count := rl.GetCurrentCount(key, limit, window); count != 2 {
		t.Errorf("Expected count 2 after new request, got %d", count)
	}

	// Should still allow one more request since we're under the limit of 3
	if !rl.IsAllowed(key, limit, window) {
		t.Error("Third request should be allowed - under limit")
	}

	// Now at limit with 3 requests in window
	if rl.IsAllowed(key, limit, window) {
		t.Error("Fourth request should not be allowed - at limit")
	}

	// Advance to t=90min - request at t=20min falls out of window  
	// Window now contains: t=70min, t=80min (2 requests)
	mockTime.Advance(20 * time.Minute)

	if !rl.IsAllowed(key, limit, window) {
		t.Error("Request should be allowed after t=20min request falls out of window")
	}
}

func TestRateLimiter_GetCurrentCount(t *testing.T) {
	mockTime := NewMockTimeProvider(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	rl := NewRateLimiterWithTimeProvider(mockTime)

	key := "test-count"
	limit := 5
	window := time.Hour

	if count := rl.GetCurrentCount(key, limit, window); count != 0 {
		t.Errorf("Expected count 0 for non-existent key, got %d", count)
	}

	rl.IsAllowed(key, limit, window)
	if count := rl.GetCurrentCount(key, limit, window); count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}

	mockTime.Advance(30 * time.Minute)
	rl.IsAllowed(key, limit, window)
	if count := rl.GetCurrentCount(key, limit, window); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}

	mockTime.Advance(45 * time.Minute)
	if count := rl.GetCurrentCount(key, limit, window); count != 1 {
		t.Errorf("Expected count 1 after first request expires, got %d", count)
	}
}
