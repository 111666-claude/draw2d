// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 2026

package draw2d_test

import (
	"image"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dkit"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// ---------------------------------------------------------------------------
// Independent reference implementation.
//
// The reference is deliberately written without reusing any production code:
// every curve is sampled with a fixed high resolution and point-in-polygon is
// evaluated by a textbook horizontal scanline ray cast, both for the even-odd
// and the non-zero winding rules. It serves as an oracle for the adaptive
// flattening implemented by Path.Contains / Path.ContainsWithMatrix.
// ---------------------------------------------------------------------------

type refPoint struct{ x, y float64 }

type refPolygon struct {
	rings [][]refPoint
}

// refFlatten builds a dense polygon from a draw2d path, sampling every curve
// at a fixed number of steps instead of adaptively subdividing.
func refFlatten(p *draw2d.Path) refPolygon {
	poly := refPolygon{}
	var cur []refPoint
	flush := func() {
		if len(cur) >= 3 {
			poly.rings = append(poly.rings, cur)
		}
		cur = nil
	}
	add := func(x, y float64) { cur = append(cur, refPoint{x, y}) }

	cmps := p.Components
	pts := p.Points
	i := 0
	var lastX, lastY float64
	var startX, startY float64
	have := false
	for _, cmp := range cmps {
		switch cmp {
		case draw2d.MoveToCmp:
			flush()
			startX, startY = pts[i], pts[i+1]
			lastX, lastY = startX, startY
			add(lastX, lastY)
			have = true
			i += 2
		case draw2d.LineToCmp:
			lastX, lastY = pts[i], pts[i+1]
			add(lastX, lastY)
			i += 2
		case draw2d.QuadCurveToCmp:
			cx, cy := pts[i], pts[i+1]
			ex, ey := pts[i+2], pts[i+3]
			sx, sy := lastX, lastY
			for k := 1; k <= 96; k++ {
				t := float64(k) / 96
				u := 1 - t
				x := u*u*sx + 2*u*t*cx + t*t*ex
				y := u*u*sy + 2*u*t*cy + t*t*ey
				add(x, y)
			}
			lastX, lastY = ex, ey
			i += 4
		case draw2d.CubicCurveToCmp:
			c1x, c1y := pts[i], pts[i+1]
			c2x, c2y := pts[i+2], pts[i+3]
			ex, ey := pts[i+4], pts[i+5]
			sx, sy := lastX, lastY
			for k := 1; k <= 128; k++ {
				t := float64(k) / 128
				u := 1 - t
				x := u*u*u*sx + 3*u*u*t*c1x + 3*u*t*t*c2x + t*t*t*ex
				y := u*u*u*sy + 3*u*u*t*c1y + 3*u*t*t*c2y + t*t*t*ey
				add(x, y)
			}
			lastX, lastY = ex, ey
			i += 6
		case draw2d.ArcToCmp:
			cx, cy, rx, ry, start, angle := pts[i], pts[i+1], pts[i+2], pts[i+3], pts[i+4], pts[i+5]
			steps := int(math.Ceil(math.Abs(angle)/(math.Pi/96))) + 1
			if steps < 4 {
				steps = 4
			}
			for k := 1; k <= steps; k++ {
				a := start + angle*float64(k)/float64(steps)
				add(cx+math.Cos(a)*rx, cy+math.Sin(a)*ry)
			}
			lastX = cx + math.Cos(start+angle)*rx
			lastY = cy + math.Sin(start+angle)*ry
			i += 6
		case draw2d.CloseCmp:
			if have {
				add(startX, startY)
				lastX, lastY = startX, startY
			}
			flush()
		}
	}
	// Implicit close, matching the fill specification for open subpaths.
	if len(cur) >= 3 {
		if cur[len(cur)-1].x != startX || cur[len(cur)-1].y != startY {
			cur = append(cur, refPoint{startX, startY})
		}
		poly.rings = append(poly.rings, cur)
	}
	return poly
}

func refRay(px, py float64, a, b refPoint) (crosses bool, up bool) {
	if (a.y <= py && b.y > py) || (b.y <= py && a.y > py) {
		xi := a.x + (b.x-a.x)*(py-a.y)/(b.y-a.y)
		if xi > px {
			return true, a.y < b.y
		}
	}
	return false, false
}

// refContains is the independent scanline oracle. Boundary points are
// classified inside.
func (poly refPolygon) refContains(x, y float64, rule draw2d.FillRule) bool {
	count, winding := 0, 0
	for _, ring := range poly.rings {
		for k := range ring {
			a, b := ring[k], ring[(k+1)%len(ring)]
			if refPointOnSegment(x, y, a, b) {
				return true
			}
			crosses, up := refRay(x, y, a, b)
			if crosses {
				count++
				if up {
					winding++
				} else {
					winding--
				}
			}
		}
	}
	if rule == draw2d.FillRuleWinding {
		return winding != 0
	}
	return count%2 == 1
}

func refPointOnSegment(px, py float64, a, b refPoint) bool {
	if (b.x-a.x)*(py-a.y)-(b.y-a.y)*(px-a.x) != 0 {
		return false
	}
	return px >= fminRef(a.x, b.x) && px <= fmaxRef(a.x, b.x) &&
		py >= fminRef(a.y, b.y) && py <= fmaxRef(a.y, b.y)
}

func fminRef(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func fmaxRef(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// inversePoint maps a device point back through tr. Matrix.Inverse contains
// an off diagonal sign bug, so the tests derive the inverse analytically
// rather than depending on that method.
func inversePoint(tr draw2d.Matrix, x, y float64) (float64, float64) {
	d := tr.Determinant()
	u := ((x-tr[4])*tr[3] - (y-tr[5])*tr[2]) / d
	v := ((y-tr[5])*tr[0] - (x-tr[4])*tr[1]) / d
	return u, v
}

// distanceToPolygon returns the distance from (x,y) to the nearest polygon
// edge. It defines the tolerance band in which the two flatteners are allowed
// to disagree because they approximate curves differently.
func (poly refPolygon) distanceToPolygon(x, y float64) float64 {
	best := math.Inf(1)
	for _, ring := range poly.rings {
		for k := range ring {
			d := distanceToSegment(x, y, ring[k], ring[(k+1)%len(ring)])
			if d < best {
				best = d
			}
		}
	}
	return best
}

func distanceToSegment(px, py float64, a, b refPoint) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-a.x, py-a.y)
	}
	t := ((px-a.x)*dx + (py-a.y)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(a.x+t*dx), py-(a.y+t*dy))
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// intersectingCircles: two overlapping circles built with arcs. With the
// even-odd rule the lens is a hole; with the non-zero rule it stays filled.
func intersectingCircles() *draw2d.Path {
	p := &draw2d.Path{}
	draw2dkit.Circle(p, 30, 50, 30)
	draw2dkit.Circle(p, 70, 50, 30)
	return p
}

// badgeWithHole: an outer rounded rectangle (quadratic curves) containing a
// circular hole drawn in the opposite direction. Even-odd and non-zero must
// agree that the hole is empty and the badge body filled.
func badgeWithHole() *draw2d.Path {
	p := &draw2d.Path{}
	// outer contour, counter-clockwise in the y-down system
	draw2dkit.RoundedRectangle(p, 10, 10, 110, 90, 16, 16)
	// inner hole, opposite direction: a negative angle sweeps clockwise in
	// the y-down coordinate system, so its winding contribution cancels the
	// outer contour under the non-zero rule.
	p.ArcTo(60, 50, 18, 18, 0, -math.Pi*2)
	p.Close()
	return p
}

// badgeWithSameWindingHole uses a hole swept in the same direction as the
// outer contour. Under even-odd it is still a hole, but the non-zero rule
// keeps it filled: the two rules are expected to disagree here.
func badgeWithSameWindingHole() *draw2d.Path {
	p := &draw2d.Path{}
	draw2dkit.RoundedRectangle(p, 10, 10, 110, 90, 16, 16)
	p.ArcTo(60, 50, 18, 18, 0, math.Pi*2)
	p.Close()
	return p
}

// curveBoundary: a closed shape bounded by cubic and quadratic Béziers plus a
// straight edge, including a cusp, so boundary/tangency handling is exercised.
func curveBoundary() *draw2d.Path {
	p := &draw2d.Path{}
	p.MoveTo(20, 80)
	p.CubicCurveTo(20, 20, 60, 20, 60, 50)
	p.QuadCurveTo(60, 90, 100, 80)
	p.LineTo(20, 80)
	p.Close()
	return p
}

// openTriangle is deliberately not closed: fill semantics close it implicitly.
func openTriangle() *draw2d.Path {
	p := &draw2d.Path{}
	p.MoveTo(10, 90)
	p.LineTo(50, 10)
	p.LineTo(90, 90)
	return p
}

// nestedSquares builds three nested contours with alternating directions so
// the non-zero rule keeps the innermost region filled and even-odd alternates.
func nestedSquares() *draw2d.Path {
	p := &draw2d.Path{}
	rect := func(x1, y1, x2, y2 float64, cw bool) {
		if cw {
			p.MoveTo(x1, y1)
			p.LineTo(x1, y2)
			p.LineTo(x2, y2)
			p.LineTo(x2, y1)
		} else {
			p.MoveTo(x1, y1)
			p.LineTo(x2, y1)
			p.LineTo(x2, y2)
			p.LineTo(x1, y2)
		}
		p.Close()
	}
	rect(0, 0, 120, 120, false)
	rect(20, 20, 100, 100, true)
	rect(40, 40, 80, 80, false)
	return p
}

// ---------------------------------------------------------------------------
// Tests against the independent scanline reference
// ---------------------------------------------------------------------------

func TestHitTestMatchesReference(t *testing.T) {
	fixtures := map[string]*draw2d.Path{
		"intersecting circles": intersectingCircles(),
		"badge with hole":      badgeWithHole(),
		"curve boundary":       curveBoundary(),
		"open triangle":        openTriangle(),
		"nested squares":       nestedSquares(),
	}
	rules := []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding}
	for name, path := range fixtures {
		poly := refFlatten(path)
		for _, rule := range rules {
			// Deterministic jittered grid so vertices, horizontal edges and
			// ray/edge tangencies are hit by some sample points.
			for ix := -5; ix <= 125; ix++ {
				for iy := -5; iy <= 105; iy++ {
					x := float64(ix) + 0.37
					y := float64(iy) + 0.61
					got := path.Contains(x, y, rule)
					want := poly.refContains(x, y, rule)
					if got == want {
						continue
					}
					// Disagreement is only tolerated inside the flattening
					// tolerance band around the reference contour, where the
					// two polygon approximations of curves may differ.
					if d := poly.distanceToPolygon(x, y); d < 0.8 {
						continue
					}
					t.Errorf("%s rule=%v point=(%.2f,%.2f): Contains=%v, reference=%v (distance %.3f)",
						name, rule, x, y, got, want, poly.distanceToPolygon(x, y))
				}
			}
		}
	}
}

func TestHitTestReferenceOnAnchors(t *testing.T) {
	// Exact anchor points of straight edges must agree with the reference
	// without relying on the tolerance band.
	poly := refFlatten(openTriangle())
	anchors := []refPoint{
		{10, 90}, {50, 10}, {90, 90}, // vertices
		{30, 50}, {70, 50}, // points on the two slanted edges
		{50, 90}, // point on the implicit closing edge
		{50, 50}, // strict interior
		{5, 50},  // strict exterior
	}
	for _, q := range anchors {
		got := openTriangle().Contains(q.x, q.y, draw2d.FillRuleEvenOdd)
		want := poly.refContains(q.x, q.y, draw2d.FillRuleEvenOdd)
		if got != want {
			t.Errorf("anchor (%.1f,%.1f): Contains=%v reference=%v", q.x, q.y, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Fill rule semantics: holes, winding and reversed contours
// ---------------------------------------------------------------------------

func TestHitTestFillRules(t *testing.T) {
	circles := intersectingCircles()
	cases := []struct {
		name        string
		x, y        float64
		wantEvenOdd bool
		wantWinding bool
	}{
		{"left lobe", 25, 50, true, true},
		{"right lobe", 75, 50, true, true},
		{"shared lens", 50, 50, false, true},
		{"outside", 50, 5, false, false},
		{"far outside", 200, 200, false, false},
	}
	for _, c := range cases {
		if got := circles.Contains(c.x, c.y, draw2d.FillRuleEvenOdd); got != c.wantEvenOdd {
			t.Errorf("%s even-odd: got %v want %v", c.name, got, c.wantEvenOdd)
		}
		if got := circles.Contains(c.x, c.y, draw2d.FillRuleWinding); got != c.wantWinding {
			t.Errorf("%s winding: got %v want %v", c.name, got, c.wantWinding)
		}
	}

	badge := badgeWithHole()
	for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
		if !badge.Contains(25, 50, rule) {
			t.Errorf("badge body should be inside under rule %v", rule)
		}
		if badge.Contains(60, 50, rule) {
			t.Errorf("badge hole should be outside under rule %v", rule)
		}
	}

	// Same-winding inner contour: even-odd still treats it as a hole while
	// the non-zero rule fills it because the windings add up.
	sameWinding := badgeWithSameWindingHole()
	if sameWinding.Contains(60, 50, draw2d.FillRuleEvenOdd) {
		t.Error("even-odd must leave the inner contour empty regardless of direction")
	}
	if !sameWinding.Contains(60, 50, draw2d.FillRuleWinding) {
		t.Error("non-zero must fill a same-winding inner contour")
	}
	if !sameWinding.Contains(25, 50, draw2d.FillRuleWinding) {
		t.Error("badge body should stay inside under the non-zero rule")
	}

	nested := nestedSquares()
	// regions: [0,20) ring1 only, [20,40) ring1+ring2, [40,80) three rings,
	// [80,100) two rings, [100,120) one ring
	nestedCases := []struct {
		x                        float64
		wantEvenOdd, wantWinding bool
	}{
		{10, true, true},
		{30, false, false}, // opposite winding cancels
		{60, true, true},   // three rings: odd, non-zero
		{90, false, false},
		{110, true, true},
	}
	for _, c := range nestedCases {
		if got := nested.Contains(c.x, c.x, draw2d.FillRuleEvenOdd); got != c.wantEvenOdd {
			t.Errorf("nested (%.0f): even-odd got %v want %v", c.x, got, c.wantEvenOdd)
		}
		if got := nested.Contains(c.x, c.x, draw2d.FillRuleWinding); got != c.wantWinding {
			t.Errorf("nested (%.0f): winding got %v want %v", c.x, got, c.wantWinding)
		}
	}
}

func TestHitTestBoundaryAndTangency(t *testing.T) {
	// Exact polygon boundary points count as inside.
	square := &draw2d.Path{}
	draw2dkit.Rectangle(square, 10, 10, 90, 90)
	for _, q := range []refPoint{{10, 10}, {90, 90}, {50, 10}, {10, 50}, {90, 50}, {50, 90}} {
		if !square.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("square boundary point (%v,%v) must be inside", q.x, q.y)
		}
	}

	// Tangency at arc extrema: fill works on the flattened polygon, which
	// cuts the mathematical curve at the same flatness threshold as the
	// rasterizer. Points an epsilon inside the mathematical boundary are
	// always inside, points an epsilon outside are always outside, and the
	// classification is stable (no panic, no flip between calls).
	circle := &draw2d.Path{}
	draw2dkit.Circle(circle, 50, 50, 40)
	for _, q := range []refPoint{{50, 10.5}, {50, 89.5}, {10.5, 50}, {89.5, 50}, {50, 50}} {
		if !circle.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("circle interior point (%v,%v) must be inside", q.x, q.y)
		}
	}
	for _, q := range []refPoint{{50, 9.5}, {50, 90.5}, {9.5, 50}, {90.5, 50}} {
		if circle.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("circle exterior point (%v,%v) must be outside", q.x, q.y)
		}
	}
	// Horizontal-edge tangency on a polygon: a ray grazing a flat horizontal
	// boundary must count the boundary point itself as inside.
	flat := &draw2d.Path{}
	draw2dkit.Rectangle(flat, 0, 0, 80, 40)
	for _, q := range []refPoint{{40, 0}, {40, 40}, {0, 20}, {80, 20}} {
		if !flat.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("flat tangency point (%v,%v) must be inside", q.x, q.y)
		}
	}
}

// ---------------------------------------------------------------------------
// Matrix handling and transformed coordinate systems
// ---------------------------------------------------------------------------

func TestHitTestWithMatrix(t *testing.T) {
	// Unit square at the origin, translated and scaled.
	unit := &draw2d.Path{}
	draw2dkit.Rectangle(unit, 0, 0, 1, 1)

	// Scale followed by Translate: Translate composes T after the current
	// matrix, so device = T(10,20) * S(40) * local, i.e. the local unit
	// square maps to (400,800)-(440,840). Expected coordinates are derived
	// from the matrix itself, which is exactly what the rendering backends
	// apply through draw2dbase.Transformer.
	tr := draw2d.NewScaleMatrix(40, 40)
	tr.Translate(10, 20)
	locals := []struct {
		x, y float64
		want bool
	}{
		{0.5, 0.5, true},
		{0, 0, true}, // corner, boundary
		{1, 1, true}, // corner, boundary
		{-0.05, 0.5, false},
		{1.05, 0.5, false},
		{0.5, 1.05, false},
	}
	for _, c := range locals {
		dx, dy := tr.TransformPoint(c.x, c.y)
		if got := unit.ContainsWithMatrix(dx, dy, tr, draw2d.FillRuleEvenOdd); got != c.want {
			t.Errorf("transformed square local (%v,%v) device (%v,%v): got %v want %v", c.x, c.y, dx, dy, got, c.want)
		}
	}

	// Rotation: a point outside the axis aligned box but inside the rotated
	// square must be found.
	diamond := &draw2d.Path{}
	draw2dkit.Rectangle(diamond, -25, -25, 25, 25)
	rot := draw2d.NewRotationMatrix(math.Pi / 4)
	rot.Translate(100, 100)
	// the rotated square has vertices on the axes at distance 25*sqrt(2)
	ix, iy := rot.TransformPoint(21.2, 0) // 21.2 < 25*sqrt(2) ~= 35.36
	if !diamond.ContainsWithMatrix(ix, iy, rot, draw2d.FillRuleEvenOdd) {
		t.Error("point inside the rotated diamond reported outside")
	}
	ox, oy := rot.TransformPoint(36, 0)
	if diamond.ContainsWithMatrix(ox, oy, rot, draw2d.FillRuleEvenOdd) {
		t.Error("point outside the rotated diamond reported inside")
	}

	// Mirroring (negative determinant) reverses every contour but the
	// non-zero rule must keep the same insideness answer.
	mirror := draw2d.NewScaleMatrix(-1, 1)
	mirror.Translate(100, 0)
	badge := badgeWithHole()
	for _, q := range []refPoint{{25, 50}, {60, 50}, {60, 20}} {
		mx, my := mirror.TransformPoint(q.x, q.y)
		got := badge.ContainsWithMatrix(mx, my, mirror, draw2d.FillRuleWinding)
		want := badge.Contains(q.x, q.y, draw2d.FillRuleWinding)
		if got != want {
			t.Errorf("mirror mismatch at (%v,%v): got %v want %v", mx, my, got, want)
		}
	}

	// Negative coordinates are ordinary coordinates.
	neg := &draw2d.Path{}
	draw2dkit.Rectangle(neg, -100, -100, -20, -20)
	if !neg.Contains(-60, -60, draw2d.FillRuleEvenOdd) {
		t.Error("negative-coordinate interior missed")
	}
	if neg.Contains(-110, -60, draw2d.FillRuleEvenOdd) {
		t.Error("negative-coordinate exterior missed")
	}
}

func TestHitTestMatrixMatchesReference(t *testing.T) {
	path := curveBoundary()
	poly := refFlatten(path)
	// Build a scale then rotate then translate matrix directly so that the
	// device mapping is scale * rotate * local + translation.
	tr := draw2d.NewTranslationMatrix(130, 40)
	rot := draw2d.NewRotationMatrix(0.6)
	scl := draw2d.NewScaleMatrix(1.4, 0.9)
	rot.Compose(scl)
	tr.Compose(rot)
	for ix := 0; ix < 180; ix++ {
		for iy := 0; iy < 140; iy++ {
			x := float64(ix) + 0.43
			y := float64(iy) + 0.29
			got := path.ContainsWithMatrix(x, y, tr, draw2d.FillRuleWinding)
			// transform the point back into path coordinates for the oracle
			ux, uy := inversePoint(tr, x, y)
			want := poly.refContains(ux, uy, draw2d.FillRuleWinding)
			if got != want && poly.distanceToPolygon(ux, uy) >= 0.8 {
				t.Errorf("transformed curve point (%.2f,%.2f): got %v reference %v", x, y, got, want)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Degenerate, empty and non finite inputs must never panic
// ---------------------------------------------------------------------------

func TestHitTestSpecialInputs(t *testing.T) {
	rules := []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding, draw2d.FillRule(7), draw2d.FillRule(-1)}
	mustBeFalse := func(name string, p *draw2d.Path, x, y float64, tr draw2d.Matrix) {
		for _, rule := range rules {
			if p.ContainsWithMatrix(x, y, tr, rule) {
				t.Errorf("%s rule %v: expected false", name, rule)
			}
		}
	}
	// Same expectation under the identity matrix, for inputs that stay invalid
	// without a matrix.
	mustBeFalseIdentity := func(name string, p *draw2d.Path, x, y float64) {
		for _, rule := range rules {
			if p.Contains(x, y, rule) {
				t.Errorf("%s (identity) rule %v: expected false", name, rule)
			}
		}
	}

	var nilPath *draw2d.Path
	mustBeFalse("nil path", nilPath, 0, 0, draw2d.NewIdentityMatrix())
	mustBeFalseIdentity("nil path", nilPath, 0, 0)
	mustBeFalse("empty path", &draw2d.Path{}, 0, 0, draw2d.NewIdentityMatrix())
	mustBeFalseIdentity("empty path", &draw2d.Path{}, 0, 0)

	degenerate := &draw2d.Path{}
	degenerate.MoveTo(10, 10)
	degenerate.LineTo(10, 10)
	degenerate.MoveTo(20, 20)
	degenerate.Close()
	mustBeFalse("degenerate path", degenerate, 10, 10, draw2d.NewIdentityMatrix())
	mustBeFalseIdentity("degenerate path", degenerate, 10, 10)

	id := draw2d.NewIdentityMatrix()
	mustBeFalse("NaN point", squarePath(), math.NaN(), 5, id)
	mustBeFalseIdentity("NaN point", squarePath(), math.NaN(), 5)
	mustBeFalse("+Inf point", squarePath(), math.Inf(1), 5, id)
	mustBeFalseIdentity("+Inf point", squarePath(), math.Inf(1), 5)
	mustBeFalse("-Inf point", squarePath(), 5, math.Inf(-1), id)
	mustBeFalseIdentity("-Inf point", squarePath(), 5, math.Inf(-1))

	// Singular matrices collapse the plane: no interior can exist.
	singular := draw2d.Matrix{1, 2, 2, 4, 0, 0} // determinant 0
	mustBeFalse("singular matrix", squarePath(), 50, 50, singular)
	zeroMatrix := draw2d.Matrix{}
	mustBeFalse("zero matrix", squarePath(), 50, 50, zeroMatrix)

	// Non finite matrix entries.
	badMatrix := draw2d.NewScaleMatrix(math.NaN(), 1)
	mustBeFalse("NaN matrix", squarePath(), 50, 50, badMatrix)
	infMatrix := draw2d.NewScaleMatrix(math.Inf(1), 1)
	mustBeFalse("Inf matrix", squarePath(), 50, 50, infMatrix)

	// Huge but finite coordinates must not panic and must stay consistent.
	huge := &draw2d.Path{}
	draw2dkit.Rectangle(huge, 1e150, 1e150, 2e150, 2e150)
	_ = huge.Contains(1.5e150, 1.5e150, draw2d.FillRuleEvenOdd)
	_ = huge.Contains(1e200, 1e200, draw2d.FillRuleWinding)

	// Unknown fill rule behaves as even-odd (same default as the image
	// backend, which only switches the rasterizer for FillRuleWinding).
	circles := intersectingCircles()
	if got := circles.Contains(50, 50, draw2d.FillRule(42)); got != circles.Contains(50, 50, draw2d.FillRuleEvenOdd) {
		t.Error("unknown fill rule should follow the even-odd default")
	}
}

func squarePath() *draw2d.Path {
	p := &draw2d.Path{}
	draw2dkit.Rectangle(p, 0, 0, 100, 100)
	return p
}

func TestHitTestOpenAndDisjointSubpaths(t *testing.T) {
	// An open subpath is filled by implicitly closing it: the interior of
	// the open triangle must be reachable and the implicit closing edge
	// itself is a boundary.
	tri := openTriangle()
	if !tri.Contains(50, 50, draw2d.FillRuleEvenOdd) {
		t.Error("open triangle interior must be inside")
	}
	if !tri.Contains(50, 90, draw2d.FillRuleEvenOdd) {
		t.Error("point on the implicit closing edge must be inside")
	}
	if tri.Contains(50, 95, draw2d.FillRuleEvenOdd) {
		t.Error("point beyond the implicit closing edge must be outside")
	}

	// Several independent closed subpaths, each contributing its own region.
	disjoint := &draw2d.Path{}
	draw2dkit.Rectangle(disjoint, 0, 0, 20, 20)
	draw2dkit.Rectangle(disjoint, 80, 80, 100, 100)
	for _, q := range []refPoint{{10, 10}, {90, 90}} {
		if !disjoint.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("disjoint region at (%v,%v) must be inside", q.x, q.y)
		}
	}
	for _, q := range []refPoint{{50, 50}, {-1, 10}, {90, 79}} {
		if disjoint.Contains(q.x, q.y, draw2d.FillRuleEvenOdd) {
			t.Errorf("gap point (%v,%v) must be outside", q.x, q.y)
		}
	}
}

func TestHitTestMalformedPathsDoNotPanic(t *testing.T) {
	// Truncated point slices: components referencing missing points must be
	// ignored gracefully instead of panicking.
	malformed := []*draw2d.Path{
		{Components: []draw2d.PathCmp{draw2d.LineToCmp}, Points: []float64{1}},
		{Components: []draw2d.PathCmp{draw2d.QuadCurveToCmp}, Points: []float64{1, 2}},
		{Components: []draw2d.PathCmp{draw2d.CubicCurveToCmp}, Points: []float64{1, 2, 3}},
		{Components: []draw2d.PathCmp{draw2d.ArcToCmp}, Points: []float64{1, 2, 3, 4}},
		{Components: []draw2d.PathCmp{draw2d.MoveToCmp, draw2d.CloseCmp}, Points: []float64{1}},
		{Components: []draw2d.PathCmp{draw2d.MoveToCmp, draw2d.LineToCmp}, Points: []float64{1, 2, 3}},
	}
	for i, p := range malformed {
		for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("malformed path %d rule %v panicked: %v", i, rule, r)
					}
				}()
				_ = p.Contains(5, 5, rule)
				_ = p.ContainsWithMatrix(5, 5, draw2d.NewScaleMatrix(2, 3), rule)
			}()
		}
	}

	// Non finite coordinates inside an otherwise valid path.
	weird := &draw2d.Path{}
	weird.MoveTo(0, 0)
	weird.LineTo(math.Inf(1), 0)
	weird.LineTo(10, 10)
	weird.LineTo(0, 10)
	weird.Close()
	if weird.Contains(5, 5, draw2d.FillRuleEvenOdd) {
		t.Error("path carrying an infinite vertex must not report a finite interior")
	}
}

// FuzzHitTest exercises random points and rules against the fixtures; it only
// asserts that the entry point never panics.
func FuzzHitTest(f *testing.F) {
	f.Add(0.5, 0.5, 0)
	f.Add(50.0, 50.0, 1)
	f.Add(1e100, -1e100, 0)
	f.Add(-1e-300, 1e-300, 1)
	f.Fuzz(func(t *testing.T, x, y float64, rule int) {
		paths := []*draw2d.Path{
			intersectingCircles(), badgeWithHole(), curveBoundary(),
			openTriangle(), nestedSquares(), {},
		}
		fr := draw2d.FillRuleEvenOdd
		if rule%2 == 0 {
			fr = draw2d.FillRuleWinding
		}
		for _, p := range paths {
			_ = p.Contains(x, y, fr)
			tr := draw2d.NewScaleMatrix(1.5, 2.5)
			tr.Translate(7, -3)
			_ = p.ContainsWithMatrix(x, y, tr, fr)
		}
	})
}

// TestHitTestDoesNotMutatePath ensures hit testing is read only.
func TestHitTestDoesNotMutatePath(t *testing.T) {
	p := badgeWithHole()
	snapshot := p.String()
	tr := draw2d.NewRotationMatrix(0.3)
	for i := 0; i < 10; i++ {
		p.ContainsWithMatrix(float64(i*7), float64(i*5), tr, draw2d.FillRuleWinding)
		p.Contains(float64(i*7), float64(i*5), draw2d.FillRuleEvenOdd)
	}
	if got := p.String(); got != snapshot {
		t.Errorf("path was modified by hit testing\nbefore:\n%s\nafter:\n%s", snapshot, got)
	}
}

// ---------------------------------------------------------------------------
// Cross-check against the image (freetype rasterizer) backend
// ---------------------------------------------------------------------------

func TestHitTestMatchesImageBackend(t *testing.T) {
	fixtures := map[string]*draw2d.Path{
		"intersecting circles": intersectingCircles(),
		"badge with hole":      badgeWithHole(),
		"curve boundary":       curveBoundary(),
		"open triangle":        openTriangle(),
	}
	const size = 140
	for name, path := range fixtures {
		poly := refFlatten(path)
		for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
			img := image.NewRGBA(image.Rect(0, 0, size, size))
			gc := draw2dimg.NewGraphicContext(img)
			gc.SetFillRule(rule)
			gc.Fill(path.Copy())

			for px := 0; px < size; px++ {
				for py := 0; py < size; py++ {
					off := (py*img.Stride + px*4)
					alpha := img.Pix[off+3]
					x, y := float64(px)+0.5, float64(py)+0.5
					got := path.Contains(x, y, rule)
					switch {
					case alpha == 0 && got:
						// Fully transparent pixel reported inside: a real
						// disagreement unless the sample sits on the curve
						// approximation band.
						if d := poly.distanceToPolygon(x, y); d >= 1.0 {
							t.Errorf("%s rule %v: empty pixel (%d,%d) reported inside (dist %.2f)", name, rule, px, py, d)
						}
					case alpha == 255 && !got:
						if d := poly.distanceToPolygon(x, y); d >= 1.0 {
							t.Errorf("%s rule %v: full pixel (%d,%d) reported outside (dist %.2f)", name, rule, px, py, d)
						}
					}
				}
			}
		}
	}
}

func TestHitTestMatchesTransformedImageBackend(t *testing.T) {
	path := badgeWithHole()
	tr := draw2d.NewTranslationMatrix(15, 25)
	tr.Compose(draw2d.NewRotationMatrix(-0.4))
	tr.Compose(draw2d.NewScaleMatrix(1.2, 0.8))

	const size = 200
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetMatrixTransform(tr)
	gc.SetFillRule(draw2d.FillRuleEvenOdd)
	gc.Fill(path.Copy())

	poly := refFlatten(path)
	for px := 0; px < size; px++ {
		for py := 0; py < size; py++ {
			alpha := img.Pix[py*img.Stride+px*4+3]
			if alpha != 0 && alpha != 255 {
				continue // partial coverage boundary pixel
			}
			x, y := float64(px)+0.5, float64(py)+0.5
			got := path.ContainsWithMatrix(x, y, tr, draw2d.FillRuleEvenOdd)
			ux, uy := inversePoint(tr, x, y)
			if d := poly.distanceToPolygon(ux, uy); d < 1.0 {
				continue
			}
			want := alpha == 255
			if got != want {
				t.Errorf("pixel (%d,%d) alpha=%d: hit=%v want %v", px, py, alpha, got, want)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Cross-check against the SVG backend output
// ---------------------------------------------------------------------------

func TestHitTestMatchesSvgBackend(t *testing.T) {
	path := intersectingCircles()
	svg := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(svg)
	gc.SetFillRule(draw2d.FillRuleEvenOdd)
	gc.Fill(path.Copy())
	if len(svg.Groups) != 1 || len(svg.Groups[0].Paths) != 1 {
		t.Fatalf("unexpected SVG structure: %+v", svg.Groups)
	}
	if svg.Groups[0].FillRule != "evenodd" {
		t.Errorf("svg fill-rule = %q, want evenodd", svg.Groups[0].FillRule)
	}
	// The emitted path keeps both circle contours; the even-odd lens that the
	// SVG renderer would leave unfiled must be reported outside by hit testing.
	if path.Contains(50, 50, draw2d.FillRuleEvenOdd) {
		t.Error("even-odd lens must be outside")
	}
	if !path.Contains(50, 50, draw2d.FillRuleWinding) {
		t.Error("non-zero lens must be inside")
	}

	// A transformed fill records the matrix on the SVG group; the same matrix
	// drives the hit test.
	svg2 := draw2dsvg.NewSvg()
	gc2 := draw2dsvg.NewGraphicContext(svg2)
	tr := draw2d.NewTranslationMatrix(40, 40)
	tr.Compose(draw2d.NewScaleMatrix(2, 2))
	gc2.SetMatrixTransform(tr)
	gc2.SetFillRule(draw2d.FillRuleWinding)
	badge := badgeWithHole()
	gc2.Fill(badge.Copy())
	if len(svg2.Groups) != 1 || svg2.Groups[0].Transform == "" {
		t.Fatalf("expected a transformed SVG group, got %+v", svg2.Groups)
	}
	if svg2.Groups[0].FillRule != "nonzero" {
		t.Errorf("svg fill-rule = %q, want nonzero", svg2.Groups[0].FillRule)
	}
	// tr = T(40,40) * S(2,2), so device = 2*local + (40,40). The same matrix
	// is recorded on the SVG group and drives the hit test.
	bodyX, bodyY := tr.TransformPoint(25, 50)
	if !badge.ContainsWithMatrix(bodyX, bodyY, tr, draw2d.FillRuleWinding) {
		t.Error("transformed badge body should be inside")
	}
	holeX, holeY := tr.TransformPoint(60, 50)
	if badge.ContainsWithMatrix(holeX, holeY, tr, draw2d.FillRuleWinding) {
		t.Error("transformed badge hole should be outside")
	}
}
