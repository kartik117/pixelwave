// Package canvas stores the live 500x500 pixel grid in a single Redis key
// using BITFIELD, 4 bits per pixel (16 colors = exactly u4). 500*500*4 bits
// = 125,000 bytes total -- one Redis string, one round trip for a full
// snapshot.
package canvas

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const canvasKey = "pixelwave:canvas"

type Canvas struct {
	rdb    *redis.Client
	Width  int
	Height int
}

func New(rdb *redis.Client, width, height int) *Canvas {
	return &Canvas{rdb: rdb, Width: width, Height: height}
}

func (c *Canvas) index(x, y int) int64 {
	return int64(y*c.Width + x)
}

// InBounds reports whether (x, y) is a valid coordinate on this canvas.
func (c *Canvas) InBounds(x, y int) bool {
	return x >= 0 && x < c.Width && y >= 0 && y < c.Height
}

// SetPixel writes a single pixel's palette index (0-15) via BITFIELD SET.
// The "#idx" offset form tells Redis to compute the bit offset as idx*4
// (the width of a u4 field) rather than taking idx as a raw bit offset --
// that's what keeps 500x500 pixels packed with no gaps.
func (c *Canvas) SetPixel(ctx context.Context, x, y int, colorIndex uint8) error {
	offset := fmt.Sprintf("#%d", c.index(x, y))
	return c.rdb.BitField(ctx, canvasKey, "SET", "u4", offset, colorIndex).Err()
}

// GetPixel reads a single pixel's palette index via BITFIELD GET.
func (c *Canvas) GetPixel(ctx context.Context, x, y int) (uint8, error) {
	offset := fmt.Sprintf("#%d", c.index(x, y))
	res, err := c.rdb.BitField(ctx, canvasKey, "GET", "u4", offset).Result()
	if err != nil {
		return 0, err
	}
	if len(res) == 0 {
		return 0, nil
	}
	return uint8(res[0]), nil
}

// Exists reports whether the canvas key is present in Redis at all -- false
// right after a fresh Redis start with no AOF/RDB file, which is the signal
// used to trigger reconstruction from Postgres history (see RestoreFromHistory).
func (c *Canvas) Exists(ctx context.Context) (bool, error) {
	n, err := c.rdb.Exists(ctx, canvasKey).Result()
	return n > 0, err
}

// Snapshot reads the entire canvas in one round trip and unpacks it into a
// [height][width] grid of palette indices. Redis packs BITFIELD u4 values
// MSB-first within each byte: pixel index 0 is the high nibble of byte 0,
// index 1 is the low nibble of byte 0, index 2 is the high nibble of byte
// 1, and so on. Unpacking it by hand here (rather than issuing 250,000
// individual BITFIELD GETs) is the only way a full-canvas read stays a
// single round trip.
func (c *Canvas) Snapshot(ctx context.Context) ([][]uint8, error) {
	raw, err := c.rdb.Get(ctx, canvasKey).Bytes()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	grid := make([][]uint8, c.Height)
	for y := 0; y < c.Height; y++ {
		grid[y] = make([]uint8, c.Width)
	}

	for i := 0; i < c.Width*c.Height; i++ {
		byteIdx := i / 2
		var nibble uint8
		if byteIdx < len(raw) {
			if i%2 == 0 {
				nibble = (raw[byteIdx] >> 4) & 0x0F
			} else {
				nibble = raw[byteIdx] & 0x0F
			}
		}
		x := i % c.Width
		y := i / c.Width
		grid[y][x] = nibble
	}
	return grid, nil
}

// PixelWrite is one (x, y, colorIndex) write, applied in chronological order
// when rebuilding the canvas from Postgres history.
type PixelWrite struct {
	X, Y       int
	ColorIndex uint8
}

// RestoreFromHistory replays a chronologically-ordered list of pixel writes
// through a single Redis pipeline, rebuilding the BITFIELD canvas from
// Postgres's durable event log. Used on startup when the canvas key doesn't
// exist (Exists() returned false) -- e.g. Redis was restarted without
// persistence enabled, or its volume was wiped, but Postgres still has the
// full history.
func (c *Canvas) RestoreFromHistory(ctx context.Context, writes []PixelWrite) error {
	pipe := c.rdb.Pipeline()
	for _, w := range writes {
		offset := fmt.Sprintf("#%d", c.index(w.X, w.Y))
		pipe.BitField(ctx, canvasKey, "SET", "u4", offset, w.ColorIndex)
	}
	_, err := pipe.Exec(ctx)
	return err
}
