// Package intconv holds overflow-safe integer conversions for values headed
// into int32/int16 database parameters (#192: gosec G109/G115). Parsing
// with the target bit size and clamping keep a crafted query string or an
// unexpectedly large length from wrapping around.
package intconv

import (
	"math"
	"strconv"
)

// ParseInt32 parses s as a base-10 int32 and returns it when it is within
// [lo, hi]; otherwise (empty, invalid, out of range) it returns def.
func ParseInt32(s string, def, lo, hi int32) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < int64(lo) || v > int64(hi) {
		return def
	}
	return int32(v)
}

// Int32 converts n, clamping it to the int32 range.
func Int32(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	}
	return int32(n)
}

// Int16 converts n, clamping it to the int16 range.
func Int16(n int) int16 {
	switch {
	case n > math.MaxInt16:
		return math.MaxInt16
	case n < math.MinInt16:
		return math.MinInt16
	}
	return int16(n)
}

// Uint32 converts n, clamping it to [0, MaxUint32].
func Uint32(n int64) uint32 {
	switch {
	case n > math.MaxUint32:
		return math.MaxUint32
	case n < 0:
		return 0
	}
	return uint32(n)
}

// Uint64 converts n, clamping negatives to 0.
func Uint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
