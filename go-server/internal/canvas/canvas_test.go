package canvas

import (
	"context"
	"github.com/kartik117/pixelwave/go-server/internal/testutil"
	"testing"
)

func TestSetAndGetPixel(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	c := New(rdb, 500, 500)
	ctx := context.Background()

	if err := c.SetPixel(ctx, 100, 200, 5); err != nil {
		t.Fatalf("SetPixel: %v", err)
	}
	got, err := c.GetPixel(ctx, 100, 200)
	if err != nil {
		t.Fatalf("GetPixel: %v", err)
	}
	if got != 5 {
		t.Errorf("GetPixel = %d, want 5", got)
	}
}

func TestGetPixelOnEmptyCanvasIsZero(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	c := New(rdb, 500, 500)
	ctx := context.Background()

	got, err := c.GetPixel(ctx, 0, 0)
	if err != nil {
		t.Fatalf("GetPixel: %v", err)
	}
	if got != 0 {
		t.Errorf("GetPixel on empty canvas = %d, want 0", got)
	}
}

func TestSnapshotMatchesIndividualPixelReads(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	c := New(rdb, 500, 500)
	ctx := context.Background()

	// Cover corners, the middle, and both nibble positions of a byte
	// (even and odd linear index) -- exactly the case that would break if
	// the manual nibble-unpacking in Snapshot() disagreed with how Redis
	// itself packs BITFIELD u4 values.
	writes := map[[2]int]uint8{
		{0, 0}:     1,
		{1, 0}:     2,
		{499, 0}:   3,
		{0, 499}:   4,
		{499, 499}: 5,
		{250, 250}: 6,
	}
	for xy, idx := range writes {
		if err := c.SetPixel(ctx, xy[0], xy[1], idx); err != nil {
			t.Fatalf("SetPixel(%v): %v", xy, err)
		}
	}

	grid, err := c.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(grid) != 500 || len(grid[0]) != 500 {
		t.Fatalf("Snapshot grid dimensions = %dx%d, want 500x500", len(grid), len(grid[0]))
	}

	for xy, want := range writes {
		x, y := xy[0], xy[1]
		if grid[y][x] != want {
			t.Errorf("Snapshot[%d][%d] = %d, want %d", y, x, grid[y][x], want)
		}
		// Cross-check against GetPixel too, not just against itself.
		got, err := c.GetPixel(ctx, x, y)
		if err != nil {
			t.Fatalf("GetPixel(%d,%d): %v", x, y, err)
		}
		if got != want {
			t.Errorf("GetPixel(%d,%d) = %d, want %d", x, y, got, want)
		}
	}
}

func TestExistsReflectsWhetherAnyPixelHasBeenWritten(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	c := New(rdb, 500, 500)
	ctx := context.Background()

	exists, err := c.Exists(ctx)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("Exists on a fresh canvas = true, want false")
	}

	if err := c.SetPixel(ctx, 0, 0, 1); err != nil {
		t.Fatalf("SetPixel: %v", err)
	}

	exists, err = c.Exists(ctx)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("Exists after a write = false, want true")
	}
}

func TestRestoreFromHistoryAppliesWritesInOrder(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	c := New(rdb, 500, 500)
	ctx := context.Background()

	// Same pixel painted twice -- the later write in the slice must win,
	// the same way a real chronologically-ordered replay from Postgres
	// would have the most recent paint of a pixel override an earlier one.
	err := c.RestoreFromHistory(ctx, []PixelWrite{
		{X: 10, Y: 10, ColorIndex: 1},
		{X: 20, Y: 20, ColorIndex: 2},
		{X: 10, Y: 10, ColorIndex: 9},
	})
	if err != nil {
		t.Fatalf("RestoreFromHistory: %v", err)
	}

	got, err := c.GetPixel(ctx, 10, 10)
	if err != nil {
		t.Fatalf("GetPixel: %v", err)
	}
	if got != 9 {
		t.Errorf("GetPixel(10,10) after restore = %d, want 9 (last write should win)", got)
	}

	got, err = c.GetPixel(ctx, 20, 20)
	if err != nil {
		t.Fatalf("GetPixel: %v", err)
	}
	if got != 2 {
		t.Errorf("GetPixel(20,20) after restore = %d, want 2", got)
	}
}
