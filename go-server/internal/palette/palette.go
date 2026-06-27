// Package palette defines the 16-color palette and the index <-> hex
// mapping. A 4-bit index per pixel is what makes the Redis BITFIELD layout
// work: 16 colors fit exactly in u4, so 500*500 pixels pack into exactly
// 500*500*4 bits = 125,000 bytes for the whole canvas.
package palette

import (
	"fmt"
	"strings"
)

// Colors is fixed at exactly 16 entries -- the BITFIELD layout (u4 per
// pixel) depends on it. Loosely r/place-inspired: a neutral ramp plus a
// spread of saturated hues so a painted canvas has real contrast.
var Colors = [16]string{
	"#FFFFFF", "#E4E4E4", "#888888", "#222222",
	"#FFA7D1", "#E50000", "#E59500", "#A06A42",
	"#E5D900", "#94E044", "#02BE01", "#00D3DD",
	"#0083C7", "#0000EA", "#CF6EE4", "#820080",
}

var hexToIndex = func() map[string]uint8 {
	m := make(map[string]uint8, len(Colors))
	for i, c := range Colors {
		m[strings.ToUpper(c)] = uint8(i)
	}
	return m
}()

// IndexForHex returns the palette index for a hex color, case-insensitively.
func IndexForHex(hex string) (uint8, error) {
	idx, ok := hexToIndex[strings.ToUpper(hex)]
	if !ok {
		return 0, fmt.Errorf("color %q is not in the 16-color palette", hex)
	}
	return idx, nil
}

// HexForIndex returns the hex color for a palette index (0-15).
func HexForIndex(idx uint8) string {
	return Colors[idx&0x0F]
}
