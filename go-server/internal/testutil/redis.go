// Package testutil holds test helpers shared across internal packages.
// It's a normal (non-_test.go) package specifically so other packages'
// _test.go files can import it -- Go doesn't let _test.go files import
// each other across packages.
package testutil

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// NewTestRedis spins up a real redis:7-alpine container via testcontainers-go.
// miniredis (the usual fast in-process fake for Go Redis tests) doesn't
// implement BITFIELD, which matters here, so this trades test speed for
// actually exercising real Redis semantics.
func NewTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get redis connection string: %v", err)
	}

	opt, err := redis.ParseURL(connStr)
	if err != nil {
		t.Fatalf("failed to parse redis URL: %v", err)
	}
	return redis.NewClient(opt)
}
