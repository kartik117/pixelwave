// Package ratelimit enforces "1 pixel per second per user" via a Redis key
// with a TTL -- the key's own expiry is the rate limit window, there's no
// separate cleanup job or in-memory map to leak.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb    *redis.Client
	window time.Duration
}

func New(rdb *redis.Client, window time.Duration) *Limiter {
	return &Limiter{rdb: rdb, window: window}
}

// Allow reports whether sessionID may paint right now. SET ... NX is atomic:
// under concurrent requests from the same session (e.g. a buggy client
// firing two paints back to back), only one can win the key, so this can't
// race the way a "GET then SET if absent" pair of commands would.
func (l *Limiter) Allow(ctx context.Context, sessionID string) (bool, error) {
	key := fmt.Sprintf("pixelwave:rate:%s", sessionID)
	ok, err := l.rdb.SetNX(ctx, key, "1", l.window).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}
