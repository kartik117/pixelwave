package palette

import "testing"

func TestColorsHasExactlySixteenEntries(t *testing.T) {
	// Not just a style preference -- canvas.go's BITFIELD layout uses a u4
	// (4-bit) field per pixel, which can only address 16 distinct values.
	if len(Colors) != 16 {
		t.Fatalf("len(Colors) = %d, want 16 (canvas.go's BITFIELD u4 layout depends on this)", len(Colors))
	}
}

func TestIndexForHexRoundTripsWithHexForIndex(t *testing.T) {
	for i, hex := range Colors {
		idx, err := IndexForHex(hex)
		if err != nil {
			t.Fatalf("IndexForHex(%q): %v", hex, err)
		}
		if int(idx) != i {
			t.Errorf("IndexForHex(%q) = %d, want %d", hex, idx, i)
		}
		if HexForIndex(idx) != hex {
			t.Errorf("HexForIndex(%d) = %q, want %q", idx, HexForIndex(idx), hex)
		}
	}
}

func TestIndexForHexIsCaseInsensitive(t *testing.T) {
	idx, err := IndexForHex("#e50000")
	if err != nil {
		t.Fatalf("IndexForHex: %v", err)
	}
	want, _ := IndexForHex("#E50000")
	if idx != want {
		t.Errorf("lowercase hex resolved to a different index: %d != %d", idx, want)
	}
}

func TestIndexForHexRejectsColorsOutsideThePalette(t *testing.T) {
	if _, err := IndexForHex("#123456"); err == nil {
		t.Error("expected an error for a color outside the 16-color palette, got nil")
	}
}

func TestHexForIndexMasksToFourBits(t *testing.T) {
	// Defensive against a caller passing a corrupted/out-of-range index --
	// &0x0F keeps it from panicking on an array index out of range.
	if got := HexForIndex(31); got != Colors[31&0x0F] {
		t.Errorf("HexForIndex(31) = %q, want %q", got, Colors[31&0x0F])
	}
}
