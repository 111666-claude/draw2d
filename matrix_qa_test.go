// Copyright 2026 The draw2d Authors. All rights reserved.
// Fixed-seed property and fuzz tests for Matrix and Path. The references
// are algebraic invariants computed independently of the implementation.

package draw2d_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/llgcode/draw2d"
)

const qaSeed = 660

// runWithTimeoutQA runs f and reports whether it panicked or exceeded the
// budget (the two failure modes of hostile input).
func runWithTimeoutQA(budget time.Duration, f func()) (panicked, timedOut bool) {
	done := make(chan bool, 1)
	go func() {
		defer func() { done <- recover() != nil }()
		f()
	}()
	select {
	case p := <-done:
		return p, false
	case <-time.After(budget):
		return false, true
	}
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestMatrixTranslateScaleComposition verifies, with a fixed seed, that
// Translate then Scale maps (x,y) to (sx*x+tx, sy*y+ty) — the definition of
// the composition, independent of how it is implemented.
func TestMatrixTranslateScaleComposition(t *testing.T) {
	r := rand.New(rand.NewSource(qaSeed))
	for i := 0; i < 200; i++ {
		tx, ty := r.NormFloat64()*100, r.NormFloat64()*100
		sx, sy := r.NormFloat64()*10, r.NormFloat64()*10
		x, y := r.NormFloat64()*100, r.NormFloat64()*100

		m := draw2d.NewIdentityMatrix()
		m.Translate(tx, ty)
		m.Scale(sx, sy)
		gotX, gotY := m.TransformPoint(x, y)
		if !almostEqual(gotX, sx*x+tx, 1e-6) || !almostEqual(gotY, sy*y+ty, 1e-6) {
			t.Fatalf("Translate(%v,%v).Scale(%v,%v) maps (%v,%v) to (%v,%v), want (%v,%v)",
				tx, ty, sx, sy, x, y, gotX, gotY, sx*x+tx, sy*y+ty)
		}

		// Compose with a translation matrix must match Translate
		m2 := draw2d.NewIdentityMatrix()
		m2.Compose(draw2d.NewTranslationMatrix(tx, ty))
		m2.Scale(sx, sy)
		gotX2, gotY2 := m2.TransformPoint(x, y)
		if !almostEqual(gotX, gotX2, 1e-9) || !almostEqual(gotY, gotY2, 1e-9) {
			t.Fatalf("Compose(translation) != Translate: (%v,%v) vs (%v,%v)", gotX2, gotY2, gotX, gotY)
		}
	}
}

// TestMatrixInverseRoundTrip: for invertible matrices, transforming a point
// and applying the inverse transform must return the original point.
func TestMatrixInverseRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(qaSeed + 1))
	tested := 0
	for tested < 200 {
		m := draw2d.Matrix{
			r.NormFloat64(), r.NormFloat64(), r.NormFloat64(),
			r.NormFloat64(), r.NormFloat64() * 100, r.NormFloat64() * 100,
		}
		if !m.IsInvertible() || math.Abs(m.Determinant()) < 0.1 {
			continue
		}
		tested++
		x, y := r.NormFloat64()*50, r.NormFloat64()*50
		tx, ty := m.TransformPoint(x, y)
		inv := m.Copy()
		inv.Inverse()
		rx, ry := inv.TransformPoint(tx, ty)
		if !almostEqual(rx, x, 1e-6) || !almostEqual(ry, y, 1e-6) {
			t.Fatalf("inverse round trip failed: (%v,%v) -> (%v,%v) -> (%v,%v)", x, y, tx, ty, rx, ry)
		}
		// InverseTransformPoint must agree with the explicit inverse
		ix, iy := m.InverseTransformPoint(tx, ty)
		if !almostEqual(ix, x, 1e-6) || !almostEqual(iy, y, 1e-6) {
			t.Fatalf("InverseTransformPoint round trip failed: got (%v,%v), want (%v,%v)", ix, iy, x, y)
		}
	}
}

// TestMatrixSingularDetection: singular and non-finite matrices must be
// reported as non-invertible so callers can reject them instead of
// propagating NaN coordinates.
func TestMatrixSingularDetection(t *testing.T) {
	singular := []draw2d.Matrix{
		{},                                 // zero matrix
		{1, 2, 2, 4, 0, 0},                 // linearly dependent rows
		{0, 0, 0, 1, 5, 6},                 // collapses x
		{math.NaN(), 0, 0, 1, 0, 0},        // NaN
		{math.Inf(1), 0, 0, 1, 0, 0},       // +Inf
		{1e308, 1e308, 1e308, 1e308, 0, 0}, // determinant overflows to NaN
		{1e-300, 0, 0, 1e-300, 0, 0},       // determinant underflows to 0
	}
	for i, m := range singular {
		if m.IsInvertible() {
			t.Fatalf("matrix %d %v is singular but reported invertible", i, m)
		}
	}
	if !draw2d.NewIdentityMatrix().IsInvertible() {
		t.Fatal("identity matrix must be invertible")
	}
}

// FuzzMatrix pushes hostile matrix coefficients (NaN, ±Inf, huge values)
// through the matrix API: no panic, and the invertibility contract holds.
func FuzzMatrix(f *testing.F) {
	f.Add([]byte{1, 1, 1, 1, 1, 1}) // benign
	f.Add([]byte{0, 0, 0, 0, 0, 0}) // zero matrix (singular)
	f.Add([]byte{3, 1, 1, 1, 1, 1}) // NaN coefficient
	f.Add([]byte{4, 1, 1, 1, 1, 1}) // +Inf coefficient
	f.Add([]byte{6, 6, 6, 6, 1, 1}) // determinant overflow
	f.Add([]byte{1, 2, 2, 4, 0, 0}) // wait: bytes decode below; kept as regression seed
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) != 6 {
			return
		}
		var m draw2d.Matrix
		for i, b := range data {
			m[i] = decodeMatrixFuzzFloat(b)
		}
		panicked, timedOut := runWithTimeoutQA(2*time.Second, func() {
			_ = m.Determinant()
			inv := m.Copy()
			inv.Inverse() // must not panic even when singular
			pts := []float64{1, 2, 3, 4}
			m.Transform(pts)
			m.InverseTransform(pts)
		})
		if panicked {
			t.Fatal("matrix API panicked on hostile input")
		}
		if timedOut {
			t.Fatal("matrix API did not finish in time")
		}
		if m.IsInvertible() {
			// The algebraic contract: for points whose forward transform
			// stays finite, the inverse transform must not produce NaN.
			// (Accuracy under extreme translations is a floating-point
			// conditioning matter and is covered separately by
			// TestMatrixInverseRoundTrip with well-conditioned matrices.)
			fx, fy := m.TransformPoint(3, 4)
			if !math.IsNaN(fx) && !math.IsInf(fx, 0) && !math.IsNaN(fy) && !math.IsInf(fy, 0) {
				x, y := m.InverseTransformPoint(fx, fy)
				if math.IsNaN(x) || math.IsNaN(y) {
					t.Fatalf("invertible matrix %v produced NaN in round trip", m)
				}
			}
		}
	})
}

func decodeMatrixFuzzFloat(b byte) float64 {
	switch b {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return math.NaN()
	case 4:
		return math.Inf(1)
	case 5:
		return math.Inf(-1)
	case 6:
		return 1e308
	case 7:
		return -1e308
	case 8:
		return 1e-300
	default:
		return float64(b) - 128
	}
}

// TestPathBasicInvariants: LineTo on an empty path acts as MoveTo; Copy is
// a deep copy; Clear empties the path.
func TestPathBasicInvariants(t *testing.T) {
	p := new(draw2d.Path)
	p.LineTo(3, 4)
	if len(p.Components) != 1 || p.Components[0] != draw2d.MoveToCmp {
		t.Fatalf("LineTo on empty path must act as MoveTo: %v", p.Components)
	}
	p.MoveTo(1, 1)
	p.LineTo(2, 2)
	c := p.Copy()
	c.Points[0] = 999
	if p.Points[0] == 999 {
		t.Fatal("Copy must be a deep copy")
	}
	x, y := p.LastPoint()
	if x != 2 || y != 2 {
		t.Fatalf("LastPoint = (%v,%v), want (2,2)", x, y)
	}
	p.Clear()
	if !p.IsEmpty() {
		t.Fatal("Clear must empty the path")
	}
}

// TestPathArcToHugeSweep is a regression sample: a sweep of 1e308 radians
// used to hang the angle normalization; it must now terminate and the path
// must stay consistent.
func TestPathArcToHugeSweep(t *testing.T) {
	p := new(draw2d.Path)
	panicked, timedOut := runWithTimeoutQA(2*time.Second, func() {
		p.ArcTo(0, 0, 1, 1, 0, 1e308)
		p.ArcTo(0, 0, 1, 1, 0, -1e308)
		p.ArcTo(0, 0, 1, 1, 0, math.NaN())
	})
	if panicked {
		t.Fatal("ArcTo panicked on a huge sweep")
	}
	if timedOut {
		t.Fatal("ArcTo did not finish in time on a huge sweep")
	}
}

// TestPathStringMalformed: String must not panic on a malformed externally
// imported path whose point stream does not match its components.
func TestPathStringMalformed(t *testing.T) {
	p := &draw2d.Path{
		Components: []draw2d.PathCmp{draw2d.MoveToCmp, draw2d.CubicCurveToCmp, draw2d.ArcToCmp},
		Points:     []float64{1, 2, 3},
	}
	panicked, _ := runWithTimeoutQA(2*time.Second, func() {
		_ = p.String()
	})
	if panicked {
		t.Fatal("Path.String panicked on a malformed path")
	}
}

// TestPathVerticalFlipMalformed is a regression sample: VerticalFlip used to
// index the point stream without bounds checks and panicked on truncated
// externally imported paths.
func TestPathVerticalFlipMalformed(t *testing.T) {
	p := &draw2d.Path{
		Components: []draw2d.PathCmp{draw2d.CubicCurveToCmp, draw2d.QuadCurveToCmp},
		Points:     []float64{1, 2, 3, 4, 5, 6, 7},
	}
	panicked, _ := runWithTimeoutQA(2*time.Second, func() {
		_ = p.VerticalFlip()
	})
	if panicked {
		t.Fatal("Path.VerticalFlip panicked on a malformed path")
	}
}

// FuzzPath applies a byte-driven sequence of path operations, including
// NaN/Inf coordinates and raw component injection: no panic, no hang.
func FuzzPath(f *testing.F) {
	f.Add([]byte{0, 1, 1, 1, 130, 130, 5})
	f.Add([]byte{4, 1, 1, 1, 1, 1, 6}) // arc with huge sweep
	f.Add([]byte{3, 3, 3, 3, 3, 3, 3}) // NaN everywhere
	f.Add([]byte{6, 3, 7, 2})          // raw component injection + truncation
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		panicked, timedOut := runWithTimeoutQA(2*time.Second, func() {
			p := new(draw2d.Path)
			i := 0
			next := func() float64 {
				if i >= len(data) {
					return 0
				}
				v := decodeMatrixFuzzFloat(data[i])
				i++
				return v
			}
			for i < len(data) {
				cmd := data[i] % 8
				i++
				switch cmd {
				case 0:
					p.MoveTo(next(), next())
				case 1:
					p.LineTo(next(), next())
				case 2:
					p.QuadCurveTo(next(), next(), next(), next())
				case 3:
					p.CubicCurveTo(next(), next(), next(), next(), next(), next())
				case 4:
					p.ArcTo(next(), next(), next(), next(), next(), next())
				case 5:
					p.Close()
				case 6: // inject a raw component, desynchronizing Points
					p.Components = append(p.Components, draw2d.PathCmp(next()))
				case 7: // truncate the point stream
					if len(p.Points) > 0 {
						p.Points = p.Points[:len(p.Points)/2]
					}
				}
			}
			_ = p.String()
			_ = p.Copy()
			_ = p.VerticalFlip()
		})
		if panicked {
			t.Fatal("Path API panicked on hostile input")
		}
		if timedOut {
			t.Fatal("Path API did not finish in time")
		}
	})
}
