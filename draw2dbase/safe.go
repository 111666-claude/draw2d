// Copyright 2026 The draw2d Authors. All rights reserved.
// Defensive guards for geometric primitives fed with externally imported
// (and therefore possibly malformed) paths, transforms and dash patterns.

package draw2dbase

import "math"

const (
	// maxTraceSegments bounds the number of line segments a single curve or
	// arc may emit while being flattened. CurveRecursionLimit already bounds
	// the subdivision depth, but adversarial coordinates (e.g. ±1e308) can
	// make every subdivision look non-flat, which would otherwise emit up to
	// 2**CurveRecursionLimit segments.
	maxTraceSegments = 1 << 12

	// maxDashTransitions bounds the total number of dash on/off transitions
	// a DashVertexConverter may perform over its lifetime. A per-segment
	// cap is not enough: a curve can be flattened into many segments and
	// each segment could then trigger the cap, multiplying the work.
	// Once the budget is exhausted the converter passes segments through
	// as solid lines, keeping the total work bounded.
	maxDashTransitions = 1 << 16
)

// isFinite reports whether every value is neither NaN nor ±Inf.
func isFinite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// sanitizeScale returns a usable flattening scale, replacing non-positive or
// non-finite scales (which make TraceArc loop forever) with 1.
func sanitizeScale(scale float64) float64 {
	if !(scale > 0) || math.IsInf(scale, 0) {
		return 1
	}
	return scale
}

// validDashPattern reports whether dash is a usable dash pattern: non-empty
// and made only of finite, strictly positive lengths.
func validDashPattern(dash []float64) bool {
	if len(dash) == 0 {
		return false
	}
	for _, d := range dash {
		// !(d > 0) rejects NaN, zero and negative lengths.
		if !(d > 0) || math.IsInf(d, 0) {
			return false
		}
	}
	return true
}
