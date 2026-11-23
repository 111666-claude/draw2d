package draw2d_test

import (
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

// simpson integrates f over [a, b] with adaptive Simpson quadrature. It is
// used as an independent arc-length reference for the measure tests.
func simpson(f func(float64) float64, a, b float64) float64 {
	c := (a + b) / 2
	fa, fb, fc := f(a), f(b), f(c)
	var rec func(a, b, fa, fb, fc, whole float64, depth int) float64
	rec = func(a, b, fa, fb, fc, whole float64, depth int) float64 {
		c := (a + b) / 2
		d := (a + c) / 2
		e := (c + b) / 2
		fd, fe := f(d), f(e)
		left := (c - a) * (fa + 4*fd + fc) / 6
		right := (b - c) * (fc + 4*fe + fb) / 6
		if depth <= 0 || math.Abs(left+right-whole) <= 1e-12 {
			return left + right
		}
		return rec(a, c, fa, fc, fd, left, depth-1) + rec(c, b, fc, fb, fe, right, depth-1)
	}
	return rec(a, b, fa, fb, fc, (b-a)*(fa+4*fc+fb)/6, 40)
}

// cubicRef returns point and derivative of a cubic Bézier at t. p holds
// x0, y0, cx1, cy1, cx2, cy2, x1, y1.
func cubicRef(p [8]float64) func(t float64) (x, y, dx, dy float64) {
	return func(t float64) (x, y, dx, dy float64) {
		mt := 1 - t
		x = mt*mt*mt*p[0] + 3*mt*mt*t*p[2] + 3*mt*t*t*p[4] + t*t*t*p[6]
		y = mt*mt*mt*p[1] + 3*mt*mt*t*p[3] + 3*mt*t*t*p[5] + t*t*t*p[7]
		dx = 3*mt*mt*(p[2]-p[0]) + 6*mt*t*(p[4]-p[2]) + 3*t*t*(p[6]-p[4])
		dy = 3*mt*mt*(p[3]-p[1]) + 6*mt*t*(p[5]-p[3]) + 3*t*t*(p[7]-p[5])
		return x, y, dx, dy
	}
}

// quadRef returns point and derivative of a quadratic Bézier at t.
func quadRef(p [6]float64) func(t float64) (x, y, dx, dy float64) {
	return func(t float64) (x, y, dx, dy float64) {
		mt := 1 - t
		x = mt*mt*p[0] + 2*mt*t*p[2] + t*t*p[4]
		y = mt*mt*p[1] + 2*mt*t*p[3] + t*t*p[5]
		dx = 2*mt*(p[2]-p[0]) + 2*t*(p[4]-p[2])
		dy = 2*mt*(p[3]-p[1]) + 2*t*(p[5]-p[3])
		return x, y, dx, dy
	}
}

func speedOf(eval func(t float64) (x, y, dx, dy float64)) func(t float64) float64 {
	return func(t float64) float64 {
		_, _, dx, dy := eval(t)
		return math.Hypot(dx, dy)
	}
}

func assertFinite(t *testing.T, what string, values ...float64) {
	t.Helper()
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s: got non-finite value %v", what, v)
		}
	}
}

func assertClose(t *testing.T, what string, x, y, wantX, wantY, tol float64) {
	t.Helper()
	if math.Abs(x-wantX) > tol || math.Abs(y-wantY) > tol {
		t.Errorf("%s: got (%v, %v), want (%v, %v) ± %v", what, x, y, wantX, wantY, tol)
	}
}

// assertPointAt checks the point of m at distance d.
func assertPointAt(t *testing.T, what string, m *draw2d.PathMeasure, d, wantX, wantY, tol float64) {
	t.Helper()
	x, y := m.PointAt(d)
	assertClose(t, what, x, y, wantX, wantY, tol)
}

// assertTangentAt checks the unit tangent of m at distance d.
func assertTangentAt(t *testing.T, what string, m *draw2d.PathMeasure, d, wantX, wantY, tol float64) {
	t.Helper()
	tx, ty := m.TangentAt(d)
	assertUnit(t, what, tx, ty)
	assertClose(t, what, tx, ty, wantX, wantY, tol)
}

func assertUnit(t *testing.T, what string, tx, ty float64) {
	t.Helper()
	assertFinite(t, what, tx, ty)
	if l := math.Hypot(tx, ty); math.Abs(l-1) > 1e-9 {
		t.Errorf("%s: tangent (%v, %v) is not a unit vector (|t|=%v)", what, tx, ty, l)
	}
}

func TestPathMeasureLine(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(3, 4)
	m := draw2d.NewPathMeasure(p, 0.01)
	if m.Length() != 5 {
		t.Errorf("length: got %v, want 5", m.Length())
	}
	assertPointAt(t, "start", m, 0, 0, 0, 1e-12)
	assertPointAt(t, "mid", m, 2.5, 1.5, 2, 1e-12)
	assertPointAt(t, "end", m, 5, 3, 4, 1e-12)
	assertTangentAt(t, "tangent", m, 2.5, 0.6, 0.8, 1e-12)
}

func TestPathMeasurePolylineBoundary(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	m := draw2d.NewPathMeasure(p, 0.01)
	if m.Length() != 20 {
		t.Errorf("length: got %v, want 20", m.Length())
	}
	assertPointAt(t, "joint", m, 10, 10, 0, 1e-12)
	// The position is continuous across the segment boundary.
	x0, y0 := m.PointAt(10 - 1e-9)
	x1, y1 := m.PointAt(10 + 1e-9)
	if math.Hypot(x1-x0, y1-y0) > 1e-6 {
		t.Errorf("discontinuity at joint: (%v,%v) -> (%v,%v)", x0, y0, x1, y1)
	}
	tx, ty := m.TangentAt(9.999)
	assertUnit(t, "tangent before joint", tx, ty)
	tx, ty = m.TangentAt(10.001)
	assertUnit(t, "tangent after joint", tx, ty)
}

func TestPathMeasureQuad(t *testing.T) {
	pts := [6]float64{0, 0, 50, 100, 100, 0}
	p := new(draw2d.Path)
	p.MoveTo(pts[0], pts[1])
	p.QuadCurveTo(pts[2], pts[3], pts[4], pts[5])
	m := draw2d.NewPathMeasure(p, 1e-4)
	ref := simpson(speedOf(quadRef(pts)), 0, 1)
	if d := math.Abs(m.Length() - ref); d > 1e-3 {
		t.Errorf("length: got %v, reference %v, error %v > 1e-3", m.Length(), ref, d)
	}
	if m.Length() > ref*(1+1e-9) {
		t.Errorf("chord length %v must not exceed reference %v", m.Length(), ref)
	}
	assertPointAt(t, "start", m, 0, 0, 0, 1e-9)
	assertPointAt(t, "end", m, m.Length(), 100, 0, 1e-9)
	// Analytic end tangents of the quadratic.
	l := math.Hypot(50, 100)
	assertTangentAt(t, "tangent at start", m, 0, 50/l, 100/l, 1e-6)
	assertTangentAt(t, "tangent at end", m, m.Length(), 50/l, -100/l, 1e-6)
}

// TestPathMeasureCubicHighCurvature checks a cubic with tight curvature
// against an independent numerical integration of its speed.
func TestPathMeasureCubicHighCurvature(t *testing.T) {
	pts := [8]float64{0, 0, 300, 0, -200, 200, 100, 0}
	p := new(draw2d.Path)
	p.MoveTo(pts[0], pts[1])
	p.CubicCurveTo(pts[2], pts[3], pts[4], pts[5], pts[6], pts[7])
	m := draw2d.NewPathMeasure(p, 1e-4)
	eval := cubicRef(pts)
	ref := simpson(speedOf(eval), 0, 1)
	if d := math.Abs(m.Length() - ref); d > 1e-2 {
		t.Errorf("length: got %v, reference %v, error %v > 1e-2", m.Length(), ref, d)
	}
	assertPointAt(t, "start", m, 0, pts[0], pts[1], 1e-9)
	assertPointAt(t, "end", m, m.Length(), pts[6], pts[7], 1e-9)

	// Equally spaced distances advance by nearly equal chords and the
	// tangent stays a finite unit vector that varies continuously.
	n := 200
	step := m.Length() / float64(n)
	px, py := m.PointAt(0)
	ptx, pty := m.TangentAt(0)
	for i := 1; i <= n; i++ {
		d := float64(i) * step
		x, y, tx, ty := m.PointAndTangentAt(d)
		assertFinite(t, "point", x, y)
		assertUnit(t, "tangent", tx, ty)
		chord := math.Hypot(x-px, y-py)
		if chord > step*1.01 || chord < step*0.5 {
			t.Fatalf("d=%v: chord %v far from step %v", d, chord, step)
		}
		if dot := tx*ptx + ty*pty; dot < 0.5 {
			t.Fatalf("d=%v: tangent turned by more than 60° in one step", d)
		}
		px, py, ptx, pty = x, y, tx, ty
	}
}

func TestPathMeasureArcCircle(t *testing.T) {
	// Circle radius 10 centered at (5, 5), quarter arc from angle 0.
	p := new(draw2d.Path)
	p.ArcTo(5, 5, 10, 10, 0, math.Pi/2)
	m := draw2d.NewPathMeasure(p, 1e-4)
	want := 10 * math.Pi / 2
	if d := math.Abs(m.Length() - want); d > 1e-3 {
		t.Errorf("length: got %v, want %v (error %v)", m.Length(), want, d)
	}
	// Error bound: the chord approximation of a circular arc underestimates
	// by at most roughly angle*tol/12; require it to stay under angle*tol.
	if m.Length() > want {
		t.Errorf("chord length %v must not exceed true arc length %v", m.Length(), want)
	}
	if want-m.Length() > math.Pi/2*1e-4 {
		t.Errorf("arc length error %v exceeds angle*tolerance", want-m.Length())
	}
	assertPointAt(t, "start", m, 0, 15, 5, 1e-9)
	assertPointAt(t, "end", m, m.Length(), 5, 15, 1e-9)
	assertTangentAt(t, "tangent at start", m, 0, 0, 1, 1e-9)
	assertTangentAt(t, "tangent at end", m, m.Length(), -1, 0, 1e-9)
	// Mid arc: angle pi/4.
	mid := m.Length() / 2
	c := math.Sqrt2 / 2
	assertPointAt(t, "mid", m, mid, 5+10*c, 5+10*c, 1e-3)
	assertTangentAt(t, "tangent at mid", m, mid, -c, c, 1e-3)
}

func TestPathMeasureArcEllipse(t *testing.T) {
	rx, ry := 30.0, 10.0
	start, angle := 0.7, 2.3
	p := new(draw2d.Path)
	p.ArcTo(0, 0, rx, ry, start, angle)
	m := draw2d.NewPathMeasure(p, 1e-4)
	speed := func(theta float64) float64 {
		s, c := math.Sin(theta), math.Cos(theta)
		return math.Hypot(rx*s, ry*c)
	}
	ref := simpson(speed, start, start+angle)
	if d := math.Abs(m.Length() - ref); d > 1e-2 {
		t.Errorf("length: got %v, reference %v, error %v > 1e-2", m.Length(), ref, d)
	}
	// Invert the reference integral independently and compare point and
	// tangent at an interior distance.
	d := ref * 0.37
	lo, hi := start, start+angle
	for i := 0; i < 80; i++ {
		mid := (lo + hi) / 2
		if simpson(speed, start, mid) < d {
			lo = mid
		} else {
			hi = mid
		}
	}
	theta := (lo + hi) / 2
	x, y, tx, ty := m.PointAndTangentAt(d)
	assertClose(t, "point at d", x, y, rx*math.Cos(theta), ry*math.Sin(theta), 1e-2)
	wx, wy := -rx*math.Sin(theta), ry*math.Cos(theta)
	l := math.Hypot(wx, wy)
	assertClose(t, "tangent at d", tx, ty, wx/l, wy/l, 1e-2)
}

func TestPathMeasureArcReverse(t *testing.T) {
	// Negative sweep angle: the arc is traversed clockwise.
	p := new(draw2d.Path)
	p.ArcTo(0, 0, 10, 10, 0, -math.Pi/2)
	m := draw2d.NewPathMeasure(p, 1e-4)
	want := 10 * math.Pi / 2
	if d := math.Abs(m.Length() - want); d > 1e-3 {
		t.Errorf("length: got %v, want %v", m.Length(), want)
	}
	assertPointAt(t, "start", m, 0, 10, 0, 1e-9)
	assertPointAt(t, "end", m, m.Length(), 0, -10, 1e-9)
	// Tangents point along the direction of travel.
	assertTangentAt(t, "tangent at start", m, 0, 0, -1, 1e-9)
	assertTangentAt(t, "tangent at end", m, m.Length(), -1, 0, 1e-9)
}

func TestPathMeasureMultiSubPath(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(4, 0)
	p.MoveTo(10, 10)
	p.LineTo(10, 14)
	m := draw2d.NewPathMeasure(p, 0.01)
	if m.Length() != 8 {
		t.Errorf("length: got %v, want 8", m.Length())
	}
	assertPointAt(t, "subpath 1", m, 2, 2, 0, 1e-12)
	assertPointAt(t, "end of subpath 1", m, 4, 4, 0, 1e-12)
	assertPointAt(t, "subpath 2", m, 6, 10, 12, 1e-12)
	assertTangentAt(t, "tangent in subpath 2", m, 6, 0, 1, 1e-12)
	assertPointAt(t, "end", m, 8, 10, 14, 1e-12)
}

func TestPathMeasureClose(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(6, 0)
	p.LineTo(0, 8)
	p.Close()
	m := draw2d.NewPathMeasure(p, 0.01)
	if m.Length() != 24 {
		t.Errorf("length: got %v, want 24", m.Length())
	}
	assertPointAt(t, "before closing edge", m, 16, 0, 8, 1e-12)
	// The closing edge runs back to the subpath start.
	assertPointAt(t, "closed end", m, 24, 0, 0, 1e-12)
	assertPointAt(t, "mid closing edge", m, 16+4, 0, 4, 1e-12)
}

func TestPathMeasureTransform(t *testing.T) {
	line := func() *draw2d.Path {
		p := new(draw2d.Path)
		p.MoveTo(1, 1)
		p.LineTo(4, 5)
		return p
	}
	// Uniform scale scales the length.
	m := draw2d.NewPathMeasureTransform(line(), draw2d.NewScaleMatrix(2, 2), 0.01)
	if d := math.Abs(m.Length() - 10); d > 1e-12 {
		t.Errorf("scaled length: got %v, want 10", m.Length())
	}
	assertPointAt(t, "scaled point", m, 5, 5, 6, 1e-12)
	// Translation keeps the length and shifts the points.
	m = draw2d.NewPathMeasureTransform(line(), draw2d.NewTranslationMatrix(10, 20), 0.01)
	if d := math.Abs(m.Length() - 5); d > 1e-12 {
		t.Errorf("translated length: got %v, want 5", m.Length())
	}
	assertPointAt(t, "translated point", m, 0, 11, 21, 1e-12)
	// Rotation keeps the length and rotates the tangent.
	m = draw2d.NewPathMeasureTransform(line(), draw2d.NewRotationMatrix(math.Pi/2), 0.01)
	if d := math.Abs(m.Length() - 5); d > 1e-12 {
		t.Errorf("rotated length: got %v, want 5", m.Length())
	}
	assertTangentAt(t, "rotated tangent", m, 2.5, -0.8, 0.6, 1e-12)
	// A transformed arc is measured in transformed space.
	p := new(draw2d.Path)
	p.ArcTo(0, 0, 10, 10, 0, math.Pi/2)
	m = draw2d.NewPathMeasureTransform(p, draw2d.NewScaleMatrix(2, 2), 1e-4)
	if d := math.Abs(m.Length() - 10*math.Pi); d > 1e-2 {
		t.Errorf("scaled arc length: got %v, want %v", m.Length(), 10*math.Pi)
	}
}

func TestPathMeasureDegenerate(t *testing.T) {
	// Empty path.
	m := draw2d.NewPathMeasure(new(draw2d.Path), 0.01)
	if !m.Empty() || m.Length() != 0 {
		t.Errorf("empty path: length %v, empty %v", m.Length(), m.Empty())
	}
	assertPointAt(t, "empty point", m, 3, 0, 0, 0)
	assertTangentAt(t, "empty tangent", m, 3, 1, 0, 0)

	// MoveTo only: zero length, queries return the move point.
	p := new(draw2d.Path)
	p.MoveTo(5, 7)
	m = draw2d.NewPathMeasure(p, 0.01)
	assertPointAt(t, "move-only point", m, 1, 5, 7, 0)
	assertTangentAt(t, "move-only tangent", m, 1, 1, 0, 0)

	// Zero-length segments are skipped.
	p = new(draw2d.Path)
	p.MoveTo(1, 1)
	p.LineTo(1, 1)
	p.CubicCurveTo(1, 1, 1, 1, 1, 1)
	p.QuadCurveTo(1, 1, 1, 1)
	p.LineTo(2, 2)
	m = draw2d.NewPathMeasure(p, 0.01)
	if d := math.Abs(m.Length() - math.Sqrt2); d > 1e-12 {
		t.Errorf("zero-length segments: got %v, want %v", m.Length(), math.Sqrt2)
	}

	// Zero-angle arc contributes nothing but the line to its start.
	p = new(draw2d.Path)
	p.ArcTo(0, 0, 10, 10, 0.3, 0)
	m = draw2d.NewPathMeasure(p, 0.01)
	if m.Length() != 0 {
		t.Errorf("zero-angle arc: got length %v, want 0", m.Length())
	}

	// Distances are clamped; non-finite distances are stable.
	p = new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(3, 4)
	m = draw2d.NewPathMeasure(p, 0.01)
	assertPointAt(t, "negative distance", m, -2, 0, 0, 0)
	assertPointAt(t, "over-long distance", m, 1e300, 3, 4, 0)
	assertPointAt(t, "+Inf distance", m, math.Inf(1), 3, 4, 0)
	assertPointAt(t, "-Inf distance", m, math.Inf(-1), 0, 0, 0)
	assertPointAt(t, "NaN distance", m, math.NaN(), 0, 0, 0)
	assertTangentAt(t, "negative tangent", m, -2, 0.6, 0.8, 1e-12)

	// Non-positive and non-finite tolerances fall back to the default.
	for _, tol := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		mm := draw2d.NewPathMeasure(p, tol)
		assertFinite(t, "length with bad tolerance", mm.Length())
		x, y := mm.PointAt(2)
		assertFinite(t, "point with bad tolerance", x, y)
	}

	// Cusp cubic: the derivative vanishes at t = 0.5; the tangent must
	// stay finite and point along the curve.
	p = new(draw2d.Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(1, 1, 0, 0, 1, 1)
	m = draw2d.NewPathMeasure(p, 1e-6)
	if d := math.Abs(m.Length() - math.Sqrt2); d > 1e-4 {
		t.Errorf("cusp length: got %v, want %v", m.Length(), math.Sqrt2)
	}
	for i := 0; i <= 100; i++ {
		d := m.Length() * float64(i) / 100
		x, y, tx, ty := m.PointAndTangentAt(d)
		assertFinite(t, "cusp point", x, y)
		assertUnit(t, "cusp tangent", tx, ty)
	}
	c := math.Sqrt2 / 2
	assertTangentAt(t, "cusp tangent at apex", m, m.Length()/2, c, c, 1e-6)

	// NaN in path data must not poison the whole measure.
	p = new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(1, 0)
	p.LineTo(math.NaN(), 0)
	p.LineTo(2, 0)
	m = draw2d.NewPathMeasure(p, 0.01)
	assertFinite(t, "NaN path length", m.Length())
	nx, ny := m.PointAt(m.Length())
	assertFinite(t, "NaN path point", nx, ny)
	ntx, nty := m.TangentAt(m.Length() / 2)
	assertFinite(t, "NaN path tangent", ntx, nty)
}

func TestPathMeasureSameTimeBase(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(300, 0, -200, 200, 100, 0)
	p.LineTo(100, 100)
	p.ArcTo(100, 150, 50, 30, -math.Pi/2, math.Pi)
	m := draw2d.NewPathMeasure(p, 1e-4)
	if m.Length() <= 0 {
		t.Fatal("expected positive length")
	}
	for i := 0; i <= 50; i++ {
		d := m.Length() * float64(i) / 50
		x, y, tx, ty := m.PointAndTangentAt(d)
		px, py := m.PointAt(d)
		gx, gy := m.TangentAt(d)
		if x != px || y != py || tx != gx || ty != gy {
			t.Fatalf("d=%v: inconsistent queries (%v,%v,%v,%v) vs (%v,%v,%v,%v)",
				d, x, y, tx, ty, px, py, gx, gy)
		}
		assertFinite(t, "combined query", x, y, tx, ty)
	}
}
