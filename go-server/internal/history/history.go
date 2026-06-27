// Package history is the durable, append-only log of every pixel paint in
// Postgres -- (user, x, y, color, timestamp) for replay/audit, and the
// source of truth for rebuilding the Redis canvas if its state is ever lost.
package history

import (
	"context"
	"database/sql"

	"github.com/kartik117/pixelwave/go-server/internal/canvas"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the pixel_events table if it doesn't already exist.
// Single simple table, single go-server process per the docker-compose spec
// for this project -- no separate one-shot migrate service needed the way
// the multi-replica Python projects in this batch required one.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS pixel_events (
			id          BIGSERIAL PRIMARY KEY,
			session_id  TEXT NOT NULL,
			x           INT NOT NULL,
			y           INT NOT NULL,
			color_index SMALLINT NOT NULL,
			painted_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_pixel_events_xy_time ON pixel_events (x, y, painted_at DESC);
	`)
	return err
}

// RecordPaint appends one pixel event. Never updated or deleted -- this is
// a log, not a cache; LatestPixels (below) is how "current state" gets
// derived from it.
func (s *Store) RecordPaint(ctx context.Context, sessionID string, x, y int, colorIndex uint8) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pixel_events (session_id, x, y, color_index) VALUES ($1, $2, $3, $4)`,
		sessionID, x, y, colorIndex,
	)
	return err
}

// LatestPixels returns the most recent color for every (x, y) that has ever
// been painted, using Postgres's DISTINCT ON rather than replaying the
// entire (potentially much larger) event history through Redis -- this is
// what RestoreFromHistory in the canvas package gets fed when Redis's own
// canvas state is missing on startup.
func (s *Store) LatestPixels(ctx context.Context) ([]canvas.PixelWrite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (x, y) x, y, color_index
		FROM pixel_events
		ORDER BY x, y, painted_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var writes []canvas.PixelWrite
	for rows.Next() {
		var w canvas.PixelWrite
		var colorIndex int
		if err := rows.Scan(&w.X, &w.Y, &colorIndex); err != nil {
			return nil, err
		}
		w.ColorIndex = uint8(colorIndex)
		writes = append(writes, w)
	}
	return writes, rows.Err()
}

// EventCount returns the total number of pixel events ever recorded --
// used by GET /canvas to report how much history exists without returning
// the whole log.
func (s *Store) EventCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pixel_events`).Scan(&count)
	return count, err
}
