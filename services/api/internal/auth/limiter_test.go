package auth

import (
	"testing"
	"time"
)

func TestLimiterAllowsWithinLimit(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th request should be blocked")
	}
}

func TestLimiterIsPerKey(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.Allow("a") {
		t.Fatal("first key should be allowed")
	}
	if !l.Allow("b") {
		t.Fatal("second key should be independent")
	}
}

func TestLimiterResetsAfterWindow(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.Allow("ip") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("ip") {
		t.Fatal("second request should be blocked")
	}

	now = now.Add(time.Minute + time.Second)
	if !l.Allow("ip") {
		t.Fatal("request after window should be allowed")
	}
}

func TestLimiterReset(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.Allow("ip") {
		t.Fatal("first request should be allowed")
	}
	l.Reset("ip")
	if !l.Allow("ip") {
		t.Fatal("request after reset should be allowed")
	}
}
