package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginLimiterCapacityIsBounded(t *testing.T) {
	l := newLoginLimiter(3, time.Hour, 8)
	for i := 0; i < 100; i++ {
		l.Fail(fmt.Sprintf("key-%d", i))
	}
	l.mu.Lock()
	n := len(l.entries)
	l.mu.Unlock()
	if n > 8 {
		t.Fatalf("limiter grew past capacity: %d", n)
	}
}

func TestLoginLimiterExpiresBuckets(t *testing.T) {
	l := newLoginLimiter(1, 5*time.Millisecond, 8)
	l.Fail("a")
	if l.Allowed("a") {
		t.Fatal("expected a to be limited")
	}
	time.Sleep(10 * time.Millisecond)
	if !l.Allowed("a") {
		t.Fatal("expected expired bucket to be allowed")
	}
}
