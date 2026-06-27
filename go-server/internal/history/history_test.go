package history

import (
	"context"
	"testing"

	"github.com/kartik117/pixelwave/go-server/internal/testutil"
)

func setup(t *testing.T) *Store {
	t.Helper()
	db := testutil.NewTestPostgres(t)
	s := New(db)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestRecordPaintAndEventCount(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	if err := s.RecordPaint(ctx, "session-1", 1, 2, 3); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}
	if err := s.RecordPaint(ctx, "session-1", 4, 5, 6); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}

	count, err := s.EventCount(ctx)
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if count != 2 {
		t.Errorf("EventCount = %d, want 2", count)
	}
}

func TestLatestPixelsReturnsOnlyTheMostRecentColorPerCoordinate(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	// Same pixel painted 3 times -- LatestPixels must return only the last
	// color, not all 3 events, and must not also return earlier events for
	// other coordinates as if they were still current.
	if err := s.RecordPaint(ctx, "session-1", 10, 10, 1); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}
	if err := s.RecordPaint(ctx, "session-1", 10, 10, 2); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}
	if err := s.RecordPaint(ctx, "session-2", 10, 10, 9); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}
	if err := s.RecordPaint(ctx, "session-1", 20, 20, 5); err != nil {
		t.Fatalf("RecordPaint: %v", err)
	}

	writes, err := s.LatestPixels(ctx)
	if err != nil {
		t.Fatalf("LatestPixels: %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("LatestPixels returned %d writes, want 2 (one per distinct coordinate)", len(writes))
	}

	byCoord := make(map[[2]int]uint8)
	for _, w := range writes {
		byCoord[[2]int{w.X, w.Y}] = w.ColorIndex
	}
	if byCoord[[2]int{10, 10}] != 9 {
		t.Errorf("(10,10) = %d, want 9 (the most recent paint)", byCoord[[2]int{10, 10}])
	}
	if byCoord[[2]int{20, 20}] != 5 {
		t.Errorf("(20,20) = %d, want 5", byCoord[[2]int{20, 20}])
	}
}

func TestLatestPixelsOnEmptyHistoryIsEmpty(t *testing.T) {
	s := setup(t)
	writes, err := s.LatestPixels(context.Background())
	if err != nil {
		t.Fatalf("LatestPixels: %v", err)
	}
	if len(writes) != 0 {
		t.Errorf("LatestPixels on empty history = %d writes, want 0", len(writes))
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := setup(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}
}
