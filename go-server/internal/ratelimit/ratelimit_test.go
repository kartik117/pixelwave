package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/kartik117/pixelwave/go-server/internal/testutil"
)

func TestFirstPaintInWindowIsAllowed(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	l := New(rdb, time.Second)

	allowed, err := l.Allow(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("first paint in the window = not allowed, want allowed")
	}
}

func TestSecondPaintInTheSameWindowIsRejected(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	l := New(rdb, time.Second)
	ctx := context.Background()

	if _, err := l.Allow(ctx, "session-1"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	allowed, err := l.Allow(ctx, "session-1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allowed {
		t.Error("second paint within the window = allowed, want rejected")
	}
}

func TestDifferentSessionsAreRateLimitedIndependently(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	l := New(rdb, time.Second)
	ctx := context.Background()

	if _, err := l.Allow(ctx, "session-1"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	allowed, err := l.Allow(ctx, "session-2")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("a different session's first paint = rejected, want allowed")
	}
}

func TestPaintIsAllowedAgainAfterTheWindowExpires(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	l := New(rdb, 200*time.Millisecond)
	ctx := context.Background()

	if _, err := l.Allow(ctx, "session-1"); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	allowed, err := l.Allow(ctx, "session-1")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("paint after the rate limit window expired = rejected, want allowed")
	}
}
