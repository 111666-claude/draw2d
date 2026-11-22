// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 23/11/2026 by draw2d contributors

package draw2d

import (
	"math"
	"math/rand"
	"testing"
)

// rect is a small helper used to compare a 4-float box.
type rect struct {
	minX, minY, maxX, maxY float64
}

func boundsOf(p *Path) rect {
	x0, y0, x1, y1 := p.Bounds()
	return rect{x0, y0, x1, y1}
}

func boundsOfMatrix(p *Path, tr Matrix) rect {
	x0, y0, x1, y1 := p.BoundsWithMatrix(tr)
	return rect{x0, y0, x1, y1}
}

// assertRect compares a box with the wanted one using an absolute and a
// relative tolerance.
func assertRect(t *testing.T, name string, got, want rect) {
	t.Helper()
	for _, c := range []struct {
		label string
		got   float64
		want  float64
	}{
		{"minX", got.minX, want.minX},
		{"minY", got.minY, want.minY},
		{"maxX", got.maxX, want.maxX},
		{"maxY", got.maxY, want.maxY},
	} {
		tol := 1e-9 + 1e-9*math.Abs(c.want)
		if math.IsNaN(c.got) || math.Abs(c.got-c.want) > tol {
			t.Errorf("%s: %s = %v, want %v (tol %g)", name, c.label, c.got, c.want, tol)
		}
	}
	if math.IsNaN(got.minX) || math.IsNaN(got.minY) || math.IsNaN(got.maxX) || math.IsNaN(got.maxY) {
		t.Fatalf("%s: bounds must never be NaN", name)
	}
}

// ---------------------------------------------------------------------------
// Dense-sampling reference implementations (independent of bounds.go).
// ---------------------------------------------------------------------------

// sampleQuad evaluates the quadratic Bézier with the Bernstein form.
func sampleQuad(p0, c, p1, t float64) float64 {
	mt := 1 - t
	return mt*mt*p0 + 2*mt*t*c + t*t*p1
}

// sampleCubic evaluates the cubic Bézier with the Bernstein form.
func sampleCubic(p0, c1, c2, p3, t float64) float64 {
	mt := 1 - t
	return mt*mt*mt*p0 + 3*mt*mt*t*c1 + 3*mt*t*t*c2 + t*t*t*p3
}

// samplingReference returns a box around a parametric point function sampled
// with n+1 uniform parameters in [lo, hi]. It intentionally evaluates the
// curve directly, so it is an independent check of the analytic extrema.
func samplingReference(n int, lo, hi float64, px, py func(t float64) float64) rect {
	r := rect{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for i := 0; i <= n; i++ {
		s := lo + (hi-lo)*float64(i)/float64(n)
		x, y := px(s), py(s)
		r.minX = math.Min(r.minX, x)
		r.minY = math.Min(r.minY, y)
		r.maxX = math.Max(r.maxX, x)
		r.maxY = math.Max(r.maxY, y)
	}
	return r
}

func bezierPath(seed int64) *Path {
	p := new(Path)
	r := rand.New(rand.NewSource(seed))
	for s := 0; s < 3; s++ {
		p.MoveTo(r.Float64()*200, r.Float64()*200)
		p.QuadCurveTo(r.Float64()*200, r.Float64()*200, r.Float64()*200, r.Float64()*200)
		p.CubicCurveTo(r.Float64()*200, r.Float64()*200, r.Float64()*200, r.Float64()*200, r.Float64()*200, r.Float64()*200)
		p.LineTo(r.Float64()*200, r.Float64()*200)
		p.Close()
	}
	return p
}

// ---------------------------------------------------------------------------
// Requirement 1: straight lines, quadratics, cubics and arcs must use the
// real extrema and must not treat control points as curve points.
// ---------------------------------------------------------------------------

func TestBoundsLine(t *testing.T) {
	p := new(Path)
	p.MoveTo(10, 20)
	p.LineTo(-5, 40)
	assertRect(t, "line", boundsOf(p), rect{-5, 20, 10, 40})
}

func TestBoundsQuadExcludesControlPoint(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.QuadCurveTo(10, 10, 20, 0)
	// The true maximum is y=5 at t=0.5, the control point (10,10) is not
	// on the curve and must not be included.
	assertRect(t, "quad", boundsOf(p), rect{0, 0, 20, 5})
}

func TestBoundsQuadAsymmetricExtrema(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.QuadCurveTo(100, 300, 20, 0)
	ref := samplingReference(200000, 0, 1,
		func(t float64) float64 { return sampleQuad(0, 100, 20, t) },
		func(t float64) float64 { return sampleQuad(0, 300, 0, t) })
	assertRect(t, "quad asymmetric", boundsOf(p), ref)
}

func TestBoundsCubicExcludesControlPoints(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(0, 100, 100, 100, 100, 0)
	// Max y is 75, not 100.
	assertRect(t, "cubic", boundsOf(p), rect{0, 0, 100, 75})
}

func TestBoundsCubicRandomAgainstSampling(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		p := bezierPath(seed)
		got := boundsOf(p)

		// Rebuild a sampling reference over every component.
		var ref rect
		init := false
		add := func(x, y float64) {
			if !init {
				ref = rect{x, y, x, y}
				init = true
				return
			}
			ref.minX = math.Min(ref.minX, x)
			ref.minY = math.Min(ref.minY, y)
			ref.maxX = math.Max(ref.maxX, x)
			ref.maxY = math.Max(ref.maxY, y)
		}
		i := 0
		var cx, cy float64
		for _, cmp := range p.Components {
			switch cmp {
			case MoveToCmp:
				cx, cy = p.Points[i], p.Points[i+1]
				add(cx, cy)
				i += 2
			case LineToCmp:
				add(p.Points[i], p.Points[i+1])
				cx, cy = p.Points[i], p.Points[i+1]
				i += 2
			case QuadCurveToCmp:
				qx, qy := p.Points[i], p.Points[i+1]
				ex, ey := p.Points[i+2], p.Points[i+3]
				r := samplingReference(4000, 0, 1,
					func(s float64) float64 { return sampleQuad(cx, qx, ex, s) },
					func(s float64) float64 { return sampleQuad(cy, qy, ey, s) })
				add(r.minX, r.minY)
				add(r.maxX, r.maxY)
				cx, cy = ex, ey
				i += 4
			case CubicCurveToCmp:
				a1x, a1y := p.Points[i], p.Points[i+1]
				a2x, a2y := p.Points[i+2], p.Points[i+3]
				ex, ey := p.Points[i+4], p.Points[i+5]
				r := samplingReference(4000, 0, 1,
					func(s float64) float64 { return sampleCubic(cx, a1x, a2x, ex, s) },
					func(s float64) float64 { return sampleCubic(cy, a1y, a2y, ey, s) })
				add(r.minX, r.minY)
				add(r.maxX, r.maxY)
				cx, cy = ex, ey
				i += 6
			}
		}
		// Cubic extrema sampled at 4000 points are accurate to ~1e-5 of
		// the coordinate range; allow a slightly looser tolerance here.
		tol := 1e-4 + 1e-7*math.Max(math.Abs(ref.maxX-ref.minX), math.Abs(ref.maxY-ref.minY))
		for _, c := range []struct {
			label string
			g, w  float64
		}{
			{"minX", got.minX, ref.minX}, {"minY", got.minY, ref.minY},
			{"maxX", got.maxX, ref.maxX}, {"maxY", got.maxY, ref.maxY},
		} {
			if math.Abs(c.g-c.w) > tol {
				t.Fatalf("seed %d %s = %v, sampling reference %v (tol %g)", seed, c.label, c.g, c.w, tol)
			}
		}
	}
}

func TestBoundsFullEllipse(t *testing.T) {
	p := new(Path)
	p.ArcTo(100, 100, 50, 30, 0, -2*math.Pi)
	assertRect(t, "full ellipse", boundsOf(p), rect{50, 70, 150, 130})
}

func TestBoundsHalfArc(t *testing.T) {
	p := new(Path)
	p.ArcTo(0, 0, 50, 50, 0, math.Pi)
	assertRect(t, "half arc +pi", boundsOf(p), rect{-50, 0, 50, 50})

	p2 := new(Path)
	p2.ArcTo(0, 0, 50, 50, 0, -math.Pi)
	assertRect(t, "half arc -pi", boundsOf(p2), rect{-50, -50, 50, 0})
}

func TestBoundsQuarterArc(t *testing.T) {
	p := new(Path)
	p.ArcTo(10, 20, 5, 7, math.Pi/2, math.Pi/2) // from top to left
	assertRect(t, "quarter arc", boundsOf(p), rect{5, 20, 10, 27})
}

func TestBoundsArcAgainstSampling(t *testing.T) {
	cases := []struct {
		cx, cy, rx, ry, start, sweep float64
	}{
		{0, 0, 50, 20, 0.3, 1.7},
		{10, -4, 30, 60, -2.1, 2.5},
		{-7, 3, 13, 5, 4.0, -0.9},
		{0, 0, 10, 10, 0, 3 * math.Pi},    // more than a full turn
		{0, 0, 10, 10, 0.7, -2 * math.Pi}, // exactly a full turn, negative
	}
	for _, c := range cases {
		p := new(Path)
		p.ArcTo(c.cx, c.cy, c.rx, c.ry, c.start, c.sweep)
		got := boundsOf(p)
		lo, hi := c.start, c.start+c.sweep
		if lo > hi {
			lo, hi = hi, lo
		}
		n := 200000
		if math.Abs(c.sweep) >= 2*math.Pi {
			lo, hi = 0, 2*math.Pi
			n = 200000
		}
		ref := samplingReference(n, lo, hi,
			func(th float64) float64 { return c.cx + c.rx*math.Cos(th) },
			func(th float64) float64 { return c.cy + c.ry*math.Sin(th) })
		// A sampled extreme is off by O(h^2) near the peak.
		tol := 1e-4 + 1e-7*math.Max(math.Abs(ref.maxX-ref.minX), math.Abs(ref.maxY-ref.minY))
		for _, d := range []struct {
			label string
			g, w  float64
		}{
			{"minX", got.minX, ref.minX}, {"minY", got.minY, ref.minY},
			{"maxX", got.maxX, ref.maxX}, {"maxY", got.maxY, ref.maxY},
		} {
			if math.Abs(d.g-d.w) > tol {
				t.Errorf("arc %+v %s = %v, sampling reference %v (tol %g)", c, d.label, d.g, d.w, tol)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Requirement 2: multi-subpath, Close, negative coordinates, rotation/scale
// and optional stroke expansion, with a normalised result and no mutation.
// ---------------------------------------------------------------------------

func TestBoundsMultipleSubpaths(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 10)
	p.Close()
	p.MoveTo(100, -50)
	p.LineTo(120, -40)
	p.Close()
	assertRect(t, "multi subpath", boundsOf(p), rect{0, -50, 120, 10})
}

func TestBoundsCloseSegment(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Close()
	// The closing segment (10,10)-(0,0) joins two points already inside the
	// box, so it cannot widen it.
	assertRect(t, "close", boundsOf(p), rect{0, 0, 10, 10})
}

func TestBoundsNegativeCoordinates(t *testing.T) {
	p := new(Path)
	p.MoveTo(-10, -20)
	p.LineTo(-30, -5)
	p.CubicCurveTo(-50, -60, -70, 0, -80, -25)
	got := boundsOf(p)

	ref := samplingReference(200000, 0, 1,
		func(s float64) float64 { return sampleCubic(-30, -50, -70, -80, s) },
		func(s float64) float64 { return sampleCubic(-5, -60, 0, -25, s) })
	ref.minX = math.Min(ref.minX, -30)
	ref.maxX = math.Max(ref.maxX, -10)
	ref.minY = math.Min(ref.minY, -20)
	ref.maxY = math.Max(ref.maxY, -5)
	if got.minX != -80 || got.maxX != -10 {
		t.Errorf("negative box = %v, want minX=-80 and maxX=-10", got)
	}
	if got.minY > -5 || got.maxY < -5 {
		t.Errorf("negative box = %v does not contain y=-5", got)
	}
	tol := 1e-4 + 1e-7*math.Max(ref.maxX-ref.minX, ref.maxY-ref.minY)
	for _, c := range []struct {
		label string
		g, w  float64
	}{
		{"minX", got.minX, ref.minX}, {"minY", got.minY, ref.minY},
		{"maxX", got.maxX, ref.maxX}, {"maxY", got.maxY, ref.maxY},
	} {
		if math.Abs(c.g-c.w) > tol {
			t.Errorf("negative %s = %v, sampling reference %v (tol %g)", c.label, c.g, c.w, tol)
		}
	}
	if got.minX > got.maxX || got.minY > got.maxY {
		t.Errorf("box is not normalised: %v", got)
	}
}

func TestBoundsReversedRectangle(t *testing.T) {
	// x1,y1 > x2,y2 must still produce a normalised box.
	p := new(Path)
	p.MoveTo(100, 80)
	p.LineTo(-20, -30)
	p.LineTo(40, 200)
	assertRect(t, "reversed", boundsOf(p), rect{-20, -30, 100, 200})
}

func TestBoundsTranslated(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.QuadCurveTo(10, 10, 20, 0)
	tr := NewTranslationMatrix(100, -50)
	assertRect(t, "translated", boundsOfMatrix(p, tr), rect{100, -50, 120, -45})
}

func TestBoundsRotated(t *testing.T) {
	// A 45-degree rotation of a 20x0 segment: endpoints rotate exactly.
	p := new(Path)
	p.MoveTo(1, 0)
	p.LineTo(-1, 0)
	tr := NewRotationMatrix(math.Pi / 4)
	b := boundsOfMatrix(p, tr)
	s := math.Sqrt2 / 2
	assertRect(t, "rotated", b, rect{-s, -s, s, s})
}

func TestBoundsRotatedEllipse(t *testing.T) {
	// Rotating an axis-aligned ellipse by theta gives a box whose half
	// extents are sqrt((rx cos)^2 + (ry sin)^2) etc.
	p := new(Path)
	p.ArcTo(0, 0, 100, 10, 0, -2*math.Pi)
	for _, theta := range []float64{0.3, math.Pi / 6, math.Pi / 4, 1.1} {
		tr := NewRotationMatrix(theta)
		c, s := math.Cos(theta), math.Sin(theta)
		ex := math.Hypot(100*c, 10*s)
		ey := math.Hypot(100*s, 10*c)
		assertRect(t, "rotated ellipse", boundsOfMatrix(p, tr), rect{-ex, -ey, ex, ey})
	}
}

func TestBoundsScaledAndRotatedCubic(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(0, 100, 100, 100, 100, 0)
	tr := NewRotationMatrix(0.7)
	tr.Scale(2, 3)
	tr.Translate(5, -5)

	// Transform the control points and sample the transformed cubic; the
	// affine image of a cubic is the cubic of the transformed controls.
	tp := func(x, y float64) (float64, float64) {
		tr.TransformPoint(x, y)
		return tr.TransformPoint(x, y)
	}
	x0, y0 := tp(0, 0)
	x1, y1 := tp(0, 100)
	x2, y2 := tp(100, 100)
	x3, y3 := tp(100, 0)
	ref := samplingReference(200000, 0, 1,
		func(t float64) float64 { return sampleCubic(x0, x1, x2, x3, t) },
		func(t float64) float64 { return sampleCubic(y0, y1, y2, y3, t) })
	assertRect(t, "scaled+rotated cubic", boundsOfMatrix(p, tr), ref)
}

func TestBoundsNegativeScale(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.QuadCurveTo(10, 10, 20, 0)
	tr := NewScaleMatrix(-1, -1)
	// Reflecting the curve gives x in [-20, 0], y in [-5, 0].
	assertRect(t, "negative scale", boundsOfMatrix(p, tr), rect{-20, -5, 0, 0})
}

func TestBoundsStrokeExpansion(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 0)

	x0, y0, x1, y1 := p.BoundsWithStroke(4)
	assertRect(t, "stroke", rect{x0, y0, x1, y1}, rect{-2, -2, 12, 2})

	// The base bounds must be unchanged by the stroke query.
	assertRect(t, "base after stroke", boundsOf(p), rect{0, 0, 10, 0})

	// Non-positive and non-finite widths are ignored.
	for _, w := range []float64{0, -4, math.NaN(), math.Inf(1)} {
		x0, y0, x1, y1 := p.BoundsWithStroke(w)
		assertRect(t, "ignored width", rect{x0, y0, x1, y1}, rect{0, 0, 10, 0})
	}
}

func TestBoundsStrokeInTargetSpace(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	// Scale by 2 first: geometry spans [0,20], then a width of 4 adds 2.
	tr := NewScaleMatrix(2, 2)
	x0, y0, x1, y1 := p.BoundsWithMatrixStroke(tr, 4)
	assertRect(t, "stroke after transform", rect{x0, y0, x1, y1}, rect{-2, -2, 22, 2})
}

func TestBoundsDoesNotModifyPath(t *testing.T) {
	p := bezierPath(42)
	components := append([]PathCmp(nil), p.Components...)
	points := append([]float64(nil), p.Points...)
	x, y := p.LastPoint()

	p.Bounds()
	p.BoundsWithStroke(7)
	p.BoundsWithMatrix(NewRotationMatrix(0.5))
	p.BoundsWithMatrixStroke(NewRotationMatrix(0.5), 3)
	p.BoundsWithStroke(2)
	p.BoundsWithMatrixStroke(NewScaleMatrix(3, 4), 1)

	if len(p.Components) != len(components) {
		t.Fatalf("Components changed: %d -> %d", len(components), len(p.Components))
	}
	for i := range components {
		if p.Components[i] != components[i] {
			t.Errorf("component %d changed", i)
		}
	}
	if len(p.Points) != len(points) {
		t.Fatalf("Points changed: %d -> %d", len(points), len(p.Points))
	}
	for i := range points {
		if p.Points[i] != points[i] {
			t.Errorf("point %d changed: %v -> %v", i, points[i], p.Points[i])
		}
	}
	if lx, ly := p.LastPoint(); lx != x || ly != y {
		t.Errorf("LastPoint changed: (%v,%v) -> (%v,%v)", x, y, lx, ly)
	}
}

// ---------------------------------------------------------------------------
// Requirement 3: empty paths, degenerate arcs, non-finite coordinates,
// reversed ranges and extreme magnitudes must have defined, NaN-free results.
// ---------------------------------------------------------------------------

func TestBoundsEmptyPath(t *testing.T) {
	assertRect(t, "empty", boundsOf(new(Path)), rect{})
	// A stroke on an empty path stays empty.
	x0, y0, x1, y1 := new(Path).BoundsWithStroke(10)
	assertRect(t, "empty+stroke", rect{x0, y0, x1, y1}, rect{})
}

func TestBoundsNilPath(t *testing.T) {
	var p *Path
	x0, y0, x1, y1 := p.Bounds()
	assertRect(t, "nil", rect{x0, y0, x1, y1}, rect{})
}

func TestBoundsSingleMoveTo(t *testing.T) {
	p := new(Path)
	p.MoveTo(5, -7)
	assertRect(t, "single move", boundsOf(p), rect{5, -7, 5, -7})
}

func TestBoundsDegenerateArc(t *testing.T) {
	// Zero sweep: only the start/end point.
	p := new(Path)
	p.ArcTo(10, 10, 5, 5, 0.5, 0)
	assertRect(t, "zero sweep", boundsOf(p), rect{
		10 + 5*math.Cos(0.5), 10 + 5*math.Sin(0.5),
		10 + 5*math.Cos(0.5), 10 + 5*math.Sin(0.5),
	})

	// Zero radius: a point at the centre.
	p2 := new(Path)
	p2.ArcTo(10, 10, 0, 0, 1, math.Pi)
	assertRect(t, "zero radius", boundsOf(p2), rect{10, 10, 10, 10})

	// Tiny sweep: still finite and within the ellipse.
	p3 := new(Path)
	p3.ArcTo(0, 0, 5, 5, 0, 1e-15)
	b := boundsOf(p3)
	if math.IsNaN(b.minX) || math.IsNaN(b.maxY) {
		t.Fatalf("tiny sweep returned NaN: %v", b)
	}
	if b.minX < -1e-14 || b.maxX > 5+1e-9 || b.minY < -1e-14 || b.maxY > 1e-14 {
		t.Errorf("tiny sweep box out of range: %v", b)
	}
}

func TestBoundsNonFiniteCoordinates(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)

	cases := []struct {
		name  string
		build func(*Path)
		want  rect
	}{
		{"moveTo NaN", func(p *Path) { p.MoveTo(nan, 0) }, rect{}},
		{"moveTo +Inf", func(p *Path) { p.MoveTo(inf, 0) }, rect{}},
		{"line endpoints", func(p *Path) { p.MoveTo(nan, nan); p.LineTo(10, 5) }, rect{10, 5, 10, 5}},
		{"quad controlpoint", func(p *Path) {
			p.MoveTo(0, 0)
			p.QuadCurveTo(nan, 10, 20, 0)
		}, rect{0, 0, 20, 0}},
		{"cubic controlpoint", func(p *Path) {
			p.MoveTo(0, 0)
			p.CubicCurveTo(1, 2, inf, 4, 30, 0)
		}, rect{0, 0, 30, 0}},
		{"arc component", func(p *Path) {
			p.MoveTo(0, 0)
			p.LineTo(30, 5)
			p.ArcTo(nan, 0, 5, 5, 0, math.Pi)
			p.LineTo(40, 40)
		}, rect{0, 0, 40, 40}},
		{"all non-finite", func(p *Path) {
			p.MoveTo(nan, inf)
			p.LineTo(-inf, nan)
		}, rect{}},
	}
	for _, c := range cases {
		p := new(Path)
		c.build(p)
		b := boundsOf(p)
		assertRect(t, c.name, b, c.want)
		if math.IsNaN(b.minX) || math.IsNaN(b.minY) || math.IsNaN(b.maxX) || math.IsNaN(b.maxY) {
			t.Errorf("%s: NaN in bounds", c.name)
		}
	}
}

func TestBoundsNonFiniteMatrix(t *testing.T) {
	p := new(Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 10)
	for _, tr := range []Matrix{
		{math.NaN(), 0, 0, 1, 0, 0},
		{1, 0, 0, 1, math.Inf(1), 0},
	} {
		b := boundsOfMatrix(p, tr)
		assertRect(t, "non-finite matrix", b, rect{})
	}
}

func TestBoundsHugeCoordinates(t *testing.T) {
	big := math.MaxFloat64 / 4
	p := new(Path)
	p.MoveTo(big, -big)
	p.CubicCurveTo(big, big, -big, big, -big, -big)
	p.QuadCurveTo(0, big, big, 0)
	x0, y0, x1, y1 := p.Bounds()
	for _, v := range []float64{x0, y0, x1, y1} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("huge coordinates produced non-finite bound %v", v)
		}
	}
	if x0 > x1 || y0 > y1 {
		t.Errorf("huge box not normalised: %v", boundsOf(p))
	}

	// Transforming extreme coordinates must saturate, not overflow.
	tr := NewScaleMatrix(10, 10)
	x0, y0, x1, y1 = p.BoundsWithMatrix(tr)
	for _, v := range []float64{x0, y0, x1, y1} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("scaled huge coordinates produced non-finite bound %v", v)
		}
	}
}

func TestBoundsExtremeMatrices(t *testing.T) {
	p := new(Path)
	p.MoveTo(1, 1)
	p.ArcTo(0, 0, 1, 1, 0, -2*math.Pi)
	for _, tr := range []Matrix{
		{math.MaxFloat64, 0, 0, math.MaxFloat64, 0, 0},
		{math.MaxFloat64, 0, 0, math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64},
		{0, 0, 0, 0, 1, 2}, // singular: collapses to a point
	} {
		x0, y0, x1, y1 := p.BoundsWithMatrix(tr)
		for _, v := range []float64{x0, y0, x1, y1} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("matrix %v produced non-finite bound %v", tr, v)
			}
		}
		if x0 > x1 || y0 > y1 {
			t.Errorf("matrix %v produced unnormalised box (%v,%v,%v,%v)", tr, x0, y0, x1, y1)
		}
	}
}

func TestBoundsSingularMatrixArc(t *testing.T) {
	// A matrix of all zeros maps every point to the origin.
	p := new(Path)
	p.ArcTo(5, 5, 3, 3, 0, -2*math.Pi)
	assertRect(t, "singular", boundsOfMatrix(p, Matrix{}), rect{})
}

func TestBoundsMalformedPathDoesNotPanic(t *testing.T) {
	p := &Path{
		Components: []PathCmp{MoveToCmp, LineToCmp},
		Points:     []float64{1, 2}, // missing LineTo coordinates
	}
	assertRect(t, "truncated", boundsOf(p), rect{1, 2, 1, 2})
}

// ---------------------------------------------------------------------------
// Requirement 4: analytic extrema should agree with a very dense sampling of
// the same curves for all four primitive kinds.
// ---------------------------------------------------------------------------

func TestBoundsAnalyticVsDenseSampling(t *testing.T) {
	const n = 1000000 // dense sampling reference

	// Quadratic.
	qx := [3]float64{12, -80, 30}
	qy := [3]float64{-5, 140, 9}
	p := new(Path)
	p.MoveTo(qx[0], qy[0])
	p.QuadCurveTo(qx[1], qy[1], qx[2], qy[2])
	assertRect(t, "dense quad", boundsOf(p), samplingReference(n, 0, 1,
		func(t float64) float64 { return sampleQuad(qx[0], qx[1], qx[2], t) },
		func(t float64) float64 { return sampleQuad(qy[0], qy[1], qy[2], t) }))

	// Cubic with two internal extrema per axis.
	cx := [4]float64{-20, 200, -150, 60}
	cy := [4]float64{0, 300, -200, 10}
	p = new(Path)
	p.MoveTo(cx[0], cy[0])
	p.CubicCurveTo(cx[1], cy[1], cx[2], cy[2], cx[3], cy[3])
	assertRect(t, "dense cubic", boundsOf(p), samplingReference(n, 0, 1,
		func(t float64) float64 { return sampleCubic(cx[0], cx[1], cx[2], cx[3], t) },
		func(t float64) float64 { return sampleCubic(cy[0], cy[1], cy[2], cy[3], t) }))

	// Arc.
	p = new(Path)
	p.ArcTo(-4, 9, 30, 80, -0.6, 1.3)
	assertRect(t, "dense arc", boundsOf(p), samplingReference(n, -0.6, 0.7,
		func(th float64) float64 { return -4 + 30*math.Cos(th) },
		func(th float64) float64 { return 9 + 80*math.Sin(th) }))

	// Line: extrema are the endpoints.
	p = new(Path)
	p.MoveTo(-3, 8)
	p.LineTo(70, -9)
	assertRect(t, "dense line", boundsOf(p), rect{-3, -9, 70, 8})
}

// TestBoundsRandomCurvesAgainstSampling checks quadratics, cubics and arcs
// with random geometry and random affine transforms against dense sampling of
// the transformed curve.
func TestBoundsRandomCurvesAgainstSampling(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	const n = 20000
	for i := 0; i < 200; i++ {
		makeTr := func() Matrix {
			m := NewRotationMatrix(r.Float64() * 2 * math.Pi)
			m.Scale(0.2+r.Float64()*3, 0.2+r.Float64()*3)
			if r.Intn(2) == 0 {
				m[0] = -m[0]
			}
			m.Translate(r.Float64()*200-100, r.Float64()*200-100)
			return m
		}
		tr := makeTr()
		tp := func(x, y float64) (float64, float64) { return tr.TransformPoint(x, y) }

		p := new(Path)
		kind := r.Intn(3)
		switch kind {
		case 0:
			x0, y0 := r.Float64()*200-100, r.Float64()*200-100
			cx, cy := r.Float64()*200-100, r.Float64()*200-100
			x1, y1 := r.Float64()*200-100, r.Float64()*200-100
			p.MoveTo(x0, y0)
			p.QuadCurveTo(cx, cy, x1, y1)
			tx0, ty0 := tp(x0, y0)
			tcx, tcy := tp(cx, cy)
			tx1, ty1 := tp(x1, y1)
			ref := samplingReference(n, 0, 1,
				func(s float64) float64 { return sampleQuad(tx0, tcx, tx1, s) },
				func(s float64) float64 { return sampleQuad(ty0, tcy, ty1, s) })
			assertClose(t, "random quad", boundsOfMatrix(p, tr), ref, n)

		case 1:
			x0, y0 := r.Float64()*200-100, r.Float64()*200-100
			a1x, a1y := r.Float64()*200-100, r.Float64()*200-100
			a2x, a2y := r.Float64()*200-100, r.Float64()*200-100
			x1, y1 := r.Float64()*200-100, r.Float64()*200-100
			p.MoveTo(x0, y0)
			p.CubicCurveTo(a1x, a1y, a2x, a2y, x1, y1)
			t0x, t0y := tp(x0, y0)
			t1x, t1y := tp(a1x, a1y)
			t2x, t2y := tp(a2x, a2y)
			t3x, t3y := tp(x1, y1)
			ref := samplingReference(n, 0, 1,
				func(s float64) float64 { return sampleCubic(t0x, t1x, t2x, t3x, s) },
				func(s float64) float64 { return sampleCubic(t0y, t1y, t2y, t3y, s) })
			assertClose(t, "random cubic", boundsOfMatrix(p, tr), ref, n)

		default:
			cx, cy := r.Float64()*100-50, r.Float64()*100-50
			rx, ry := r.Float64()*80+1, r.Float64()*80+1
			start := r.Float64()*4*math.Pi - 2*math.Pi
			sweep := (r.Float64()*4 - 2) * math.Pi
			mx, my := cx+rx, cy
			p.MoveTo(mx, my)
			// ArcTo inserts a LineTo from the current point to the arc
			// start, so both segments belong to the path.
			p.ArcTo(cx, cy, rx, ry, start, sweep)
			lo, hi := start, start+sweep
			if lo > hi {
				lo, hi = hi, lo
			}
			ref := samplingReference(n, lo, hi,
				func(th float64) float64 {
					tx, _ := tp(cx+rx*math.Cos(th), cy+ry*math.Sin(th))
					return tx
				},
				func(th float64) float64 {
					_, ty := tp(cx+rx*math.Cos(th), cy+ry*math.Sin(th))
					return ty
				})
			// Account for the MoveTo point and, when the arc is not a full
			// turn, for the LineTo segment to the arc start.
			tmx, tmy := tp(mx, my)
			ref.minX = math.Min(ref.minX, tmx)
			ref.maxX = math.Max(ref.maxX, tmx)
			ref.minY = math.Min(ref.minY, tmy)
			ref.maxY = math.Max(ref.maxY, tmy)
			assertClose(t, "random arc", boundsOfMatrix(p, tr), ref, n)
		}
	}
}

// assertClose compares a box with a sampled reference. Sampling a smooth
// curve with n+1 values leaves an O(1/n^2) gap at each interior extremum, so
// the tolerance is derived from the number of samples and the box size.
func assertClose(t *testing.T, name string, got, ref rect, n int) {
	t.Helper()
	size := math.Max(math.Max(ref.maxX-ref.minX, ref.maxY-ref.minY), 1)
	tol := 10*size/(float64(n)*float64(n)) + 1e-9
	for _, d := range []struct {
		label string
		g, w  float64
	}{
		{"minX", got.minX, ref.minX}, {"minY", got.minY, ref.minY},
		{"maxX", got.maxX, ref.maxX}, {"maxY", got.maxY, ref.maxY},
	} {
		if math.IsNaN(d.g) || math.Abs(d.g-d.w) > tol {
			t.Fatalf("%s: %s = %v, sampled %v (tol %g)", name, d.label, d.g, d.w, tol)
		}
	}
}

func TestBoundsShear(t *testing.T) {
	// Shear x by 0.5*y: (x, y) -> (x + 0.5y, y).
	tr := Matrix{1, 0, 0.5, 1, 0, 0}
	p := new(Path)
	p.MoveTo(0, 0)
	p.QuadCurveTo(100, 100, 0, 100)
	// The image of the quadratic has control points (0,0), (150,100),
	// (50,100). Its x maximum is interior (90), so the control point x=150
	// must not be used and the endpoint x=50 is below the true maximum.
	got := boundsOfMatrix(p, tr)
	ref := samplingReference(200000, 0, 1,
		func(s float64) float64 { return sampleQuad(0, 150, 50, s) },
		func(s float64) float64 { return sampleQuad(0, 100, 100, s) })
	assertRect(t, "shear", got, ref)
	if got.maxX <= 50 || got.maxX >= 150 {
		t.Errorf("sheared x maximum %v should be strictly between the endpoint 50 and the control 150", got.maxX)
	}
}

func TestBoundsHugeQuadControlSaturates(t *testing.T) {
	p := new(Path)
	p.MoveTo(-math.MaxFloat64, 0)
	p.QuadCurveTo(math.MaxFloat64, math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64)
	x0, y0, x1, y1 := p.Bounds()
	for _, v := range []float64{x0, y0, x1, y1} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("huge quad produced non-finite bound %v", v)
		}
	}
	if x0 > x1 || y0 > y1 {
		t.Errorf("huge quad box not normalised: (%v,%v,%v,%v)", x0, y0, x1, y1)
	}
}
