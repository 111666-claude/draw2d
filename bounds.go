// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 23/11/2026 by draw2d contributors

package draw2d

import "math"

// boundsEpsilon is the tolerance used when deciding whether a candidate
// extremum angle belongs to the swept part of an arc and whether a sweep
// covers a full turn.
const boundsEpsilon = 1e-12

// Bounds returns the tight axis-aligned bounding box of the path's geometry
// in the path's own coordinate space:
//
//	minX, minY, maxX, maxY
//
// The box is computed from the real curve geometry, not from the control
// points: straight segments, quadratic and cubic Bézier curves and ellipse
// arcs contribute their true extrema. For Bézier curves this means solving
// the derivative for each axis and only taking roots inside the segment; for
// arcs it means intersecting the axis-extremum angles with the swept range.
// A control point that lies outside the drawn curve therefore does not widen
// the box.
//
// Semantics:
//
//   - All subpaths contribute to a single box (the union of their geometry).
//   - A MoveTo contributes its point, so a path consisting of a single
//     MoveTo has a degenerate (zero width and height) box at that point.
//   - Close contributes the segment from the current point back to the start
//     of its subpath. Both endpoints already belong to the path, so a closing
//     segment never widens the axis-aligned box. Close leaves the current
//     point unchanged, matching Path.LastPoint and the flatteners.
//   - ArcTo is interpreted exactly as stored by ArcTo: center (cx, cy),
//     radii (rx, ry), the stored start angle and the signed sweep angle.
//     A sweep whose magnitude reaches 2*Pi covers the whole ellipse; zero
//     sweep, zero radius or a NaN/Inf component yields the arc's endpoints
//     only, so a degenerate arc never widens the box.
//   - Coordinates may be negative and rectangle-like inputs may be given in
//     either order; the result is always normalised with minX <= maxX and
//     minY <= maxY.
//   - Non-finite coordinates (NaN or +/-Inf) never contribute to the box. A
//     curve with any non-finite control point is skipped as a whole, while
//     its finite endpoint is still recorded. If no finite geometry remains,
//     the empty box (0, 0, 0, 0) is returned. Bounds never returns NaN.
//   - Arithmetic that would overflow saturates at +/-math.MaxFloat64 instead
//     of producing Inf or NaN.
//   - The path is not modified.
func (p *Path) Bounds() (minX, minY, maxX, maxY float64) {
	return p.boundsWithMatrix(NewIdentityMatrix(), 0)
}

// BoundsWithMatrix is like Bounds but returns the tight bounding box of the
// path after applying tr. Curves are transformed analytically: Bézier control
// points are transformed and their extrema recomputed, and arcs are treated
// as general affine ellipses, so rotation, scaling (including negative scale)
// and translation stay exact instead of bounding a transformed control
// polygon. If any element of tr is NaN or infinite the box is undefined, and
// the empty box (0, 0, 0, 0) is returned rather than a NaN result.
//
// See Bounds for the complete semantics.
func (p *Path) BoundsWithMatrix(tr Matrix) (minX, minY, maxX, maxY float64) {
	return p.boundsWithMatrix(tr, 0)
}

// BoundsWithStroke returns Bounds grown by half of strokeWidth on every side,
// i.e. the area covered by a stroke of that width centred on the path. The
// stroke is applied after the geometry is measured, so it does not affect the
// shape of the box. A zero, negative or non-finite width is ignored and the
// unexpanded Bounds is returned, as it is for a path with no finite geometry.
func (p *Path) BoundsWithStroke(strokeWidth float64) (minX, minY, maxX, maxY float64) {
	return p.boundsWithMatrix(NewIdentityMatrix(), strokeHalfWidth(strokeWidth))
}

// BoundsWithMatrixStroke returns BoundsWithMatrix grown by half of
// strokeWidth on every side. The width is applied in the target (device)
// space, matching draw2d's stroking order where the path is transformed first
// and the stroke is applied afterwards.
func (p *Path) BoundsWithMatrixStroke(tr Matrix, strokeWidth float64) (minX, minY, maxX, maxY float64) {
	return p.boundsWithMatrix(tr, strokeHalfWidth(strokeWidth))
}

// boundsAccumulator collects a bounding box from finite points.
type boundsAccumulator struct {
	minX, minY, maxX, maxY float64
	valid                  bool
}

func (a *boundsAccumulator) addPoint(x, y float64) {
	if !isFinite(x) || !isFinite(y) {
		return
	}
	if !a.valid {
		a.minX, a.maxX = x, x
		a.minY, a.maxY = y, y
		a.valid = true
		return
	}
	if x < a.minX {
		a.minX = x
	}
	if x > a.maxX {
		a.maxX = x
	}
	if y < a.minY {
		a.minY = y
	}
	if y > a.maxY {
		a.maxY = y
	}
}

// addTransformed transforms the point with saturation (so huge finite
// coordinates never turn into NaN) and adds it when the result is finite.
func (a *boundsAccumulator) addTransformed(tr Matrix, x, y float64) {
	if !isFinite(x) || !isFinite(y) {
		return
	}
	a.addPoint(transformX(tr, x, y), transformY(tr, x, y))
}

func (a *boundsAccumulator) result(halfStrokeWidth float64) (minX, minY, maxX, maxY float64) {
	if !a.valid {
		return 0, 0, 0, 0
	}
	if halfStrokeWidth > 0 {
		return satAdd(a.minX, -halfStrokeWidth), satAdd(a.minY, -halfStrokeWidth),
			satAdd(a.maxX, halfStrokeWidth), satAdd(a.maxY, halfStrokeWidth)
	}
	return a.minX, a.minY, a.maxX, a.maxY
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// satMul multiplies two values and saturates an overflowing product at
// +/-math.MaxFloat64. A NaN product (only reachable from a non-finite
// operand) is reported as 0.
func satMul(a, b float64) float64 {
	if a == 0 || b == 0 {
		return 0
	}
	p := a * b
	if math.IsInf(p, 0) {
		if (a < 0) != (b < 0) {
			return -math.MaxFloat64
		}
		return math.MaxFloat64
	}
	if math.IsNaN(p) {
		return 0
	}
	return p
}

// satAdd adds two values and saturates an overflow at +/-math.MaxFloat64. A
// NaN sum (only reachable from a non-finite operand) is reported as 0.
func satAdd(a, b float64) float64 {
	s := a + b
	if math.IsInf(s, 0) {
		if a > 0 || b > 0 {
			return math.MaxFloat64
		}
		return -math.MaxFloat64
	}
	if math.IsNaN(s) {
		return 0
	}
	return s
}

func transformX(tr Matrix, x, y float64) float64 {
	return satAdd(satAdd(satMul(tr[0], x), satMul(tr[2], y)), tr[4])
}

func transformY(tr Matrix, x, y float64) float64 {
	return satAdd(satAdd(satMul(tr[1], x), satMul(tr[3], y)), tr[5])
}

// arcPoint evaluates the (possibly transformed) ellipse point at theta.
func arcPoint(tr Matrix, cx, cy, rx, ry, theta float64) (x, y float64) {
	px := satAdd(cx, satMul(rx, math.Cos(theta)))
	py := satAdd(cy, satMul(ry, math.Sin(theta)))
	return transformX(tr, px, py), transformY(tr, px, py)
}

// angleInSweep reports whether theta lies on the arc starting at start and
// sweeping by sweep (signed). Endpoints count as part of the arc.
func angleInSweep(theta, start, sweep float64) bool {
	if sweep > 0 {
		d := math.Mod(theta-math.Mod(start, 2*math.Pi), 2*math.Pi)
		if d < 0 {
			d += 2 * math.Pi
		}
		return d <= sweep+boundsEpsilon
	}
	if sweep < 0 {
		d := math.Mod(math.Mod(start, 2*math.Pi)-theta, 2*math.Pi)
		if d < 0 {
			d += 2 * math.Pi
		}
		return d <= -sweep+boundsEpsilon
	}
	return false
}

// addArcBounds adds the extrema of a transformed ellipse arc.
func addArcBounds(a *boundsAccumulator, tr Matrix, cx, cy, rx, ry, start, angle float64) {
	// Both endpoints are part of the arc. The start point is evaluated
	// exactly like Path.ArcTo computes it, so bounds and rendering agree.
	sx, sy := arcPoint(tr, cx, cy, rx, ry, start)
	a.addPoint(sx, sy)
	end := start + angle
	if !isFinite(end) {
		// The sum overflowed even though both parts are finite. Reduce each
		// part first; the sweep is many full turns in that case anyway.
		end = math.Mod(start, 2*math.Pi) + math.Mod(angle, 2*math.Pi)
	}
	if isFinite(end) {
		ex, ey := arcPoint(tr, cx, cy, rx, ry, end)
		a.addPoint(ex, ey)
	}

	if math.Abs(angle) <= boundsEpsilon {
		return
	}

	// An affine map sends the ellipse to another ellipse:
	//   x(theta) = A*cos(theta) + B*sin(theta) + C
	//   y(theta) = D*cos(theta) + E*sin(theta) + F
	// The extrema of A*cos+B*sin occur at phi = atan2(B, A) (maximum) and
	// phi+Pi (minimum). Solving for the angle keeps the result exact for
	// every affine map, including rotations and negative scales.
	A := satMul(tr[0], rx)
	B := satMul(tr[2], ry)
	D := satMul(tr[1], rx)
	E := satMul(tr[3], ry)
	phiX := math.Atan2(B, A)
	phiY := math.Atan2(E, D)

	// A sweep of at least a full turn covers every candidate angle.
	full := math.Abs(angle) >= 2*math.Pi-boundsEpsilon
	for _, theta := range [...]float64{phiX, phiX + math.Pi, phiY, phiY + math.Pi} {
		if !full && !angleInSweep(theta, start, angle) {
			continue
		}
		x, y := arcPoint(tr, cx, cy, rx, ry, theta)
		a.addPoint(x, y)
	}
}

// quadEval evaluates a quadratic Bézier at t using de Casteljau's algorithm
// with saturating arithmetic, so extreme control values cannot overflow.
func quadEval(p0, c, p1, t float64) float64 {
	u := 1 - t
	return satAdd(satMul(u, satAdd(satMul(u, p0), satMul(t, c))),
		satMul(t, satAdd(satMul(u, c), satMul(t, p1))))
}

// forEachQuadRoot calls fn for every root in the open interval (0, 1) of the
// derivative of the quadratic Bézier p0, c, p1 with respect to t.
func forEachQuadRoot(p0, c, p1 float64, fn func(t float64)) {
	if !isFinite(p0) || !isFinite(c) || !isFinite(p1) {
		return
	}
	// Scale the control values into [-1, 1] so the denominator cannot
	// overflow; the root itself is invariant under this scaling.
	m := math.Max(math.Abs(p0), math.Max(math.Abs(c), math.Abs(p1)))
	if m == 0 {
		return
	}
	p0, c, p1 = p0/m, c/m, p1/m
	den := p0 - 2*c + p1
	if den == 0 {
		return
	}
	t := (p0 - c) / den
	if t > 0 && t < 1 {
		fn(t)
	}
}

func addQuadBounds(a *boundsAccumulator, tr Matrix, x0, y0, cx, cy, x1, y1 float64) {
	if !allFinite(x0, y0, cx, cy, x1, y1) {
		// Undefined curve: keep whichever endpoints are finite.
		a.addTransformed(tr, x0, y0)
		a.addTransformed(tr, x1, y1)
		return
	}
	// An affine map sends a quadratic Bézier to the quadratic Bézier of the
	// transformed control points, but it does NOT keep the extrema at the
	// same parameter values (unless it is axis aligned). Transform first and
	// solve for the extrema in target space.
	tx0, ty0 := transformX(tr, x0, y0), transformY(tr, x0, y0)
	tcx, tcy := transformX(tr, cx, cy), transformY(tr, cx, cy)
	tx1, ty1 := transformX(tr, x1, y1), transformY(tr, x1, y1)
	a.addPoint(tx0, ty0)
	a.addPoint(tx1, ty1)
	forEachQuadRoot(tx0, tcx, tx1, func(t float64) {
		a.addPoint(quadEval(tx0, tcx, tx1, t), quadEval(ty0, tcy, ty1, t))
	})
	forEachQuadRoot(ty0, tcy, ty1, func(t float64) {
		a.addPoint(quadEval(tx0, tcx, tx1, t), quadEval(ty0, tcy, ty1, t))
	})
}

// cubicEval evaluates a cubic Bézier at t using de Casteljau's algorithm with
// saturating arithmetic.
func cubicEval(p0, c1, c2, p3, t float64) float64 {
	u := 1 - t
	a := satAdd(satMul(u, p0), satMul(t, c1))
	b := satAdd(satMul(u, c1), satMul(t, c2))
	c := satAdd(satMul(u, c2), satMul(t, p3))
	d := satAdd(satMul(u, a), satMul(t, b))
	e := satAdd(satMul(u, b), satMul(t, c))
	return satAdd(satMul(u, d), satMul(t, e))
}

// forEachCubicRoot calls fn for every root in the open interval (0, 1) of the
// derivative of the cubic Bézier p0, c1, c2, p3 with respect to t.
func forEachCubicRoot(p0, c1, c2, p3 float64, fn func(t float64)) {
	if !allFinite(p0, c1, c2, p3) {
		return
	}
	// Scale the control values into [-1, 1] first; the roots of the
	// derivative are invariant under this scaling and the discriminant
	// cannot overflow.
	m := math.Max(math.Max(math.Abs(p0), math.Abs(c1)), math.Max(math.Abs(c2), math.Abs(p3)))
	if m == 0 {
		return
	}
	p0, c1, c2, p3 = p0/m, c1/m, c2/m, p3/m

	// B'(t)/3 = a*t^2 + b*t + c
	a := -p0 + 3*c1 - 3*c2 + p3
	b := 2 * (p0 - 2*c1 + c2)
	c := c1 - p0

	if a == 0 {
		if b == 0 {
			return
		}
		t := -c / b
		if t > 0 && t < 1 {
			fn(t)
		}
		return
	}

	d := b*b - 4*a*c
	if d < 0 {
		return
	}
	sq := math.Sqrt(d)
	// Numerically stable quadratic roots.
	q := -0.5 * (b + math.Copysign(sq, b))
	if q == 0 {
		t := -b / (2 * a)
		if t > 0 && t < 1 {
			fn(t)
		}
		return
	}
	t1 := q / a
	t2 := c / q
	if t1 > 0 && t1 < 1 {
		fn(t1)
	}
	if t2 > 0 && t2 < 1 && t2 != t1 {
		fn(t2)
	}
}

func addCubicBounds(a *boundsAccumulator, tr Matrix, x0, y0, cx1, cy1, cx2, cy2, x1, y1 float64) {
	if !allFinite(x0, y0, cx1, cy1, cx2, cy2, x1, y1) {
		a.addTransformed(tr, x0, y0)
		a.addTransformed(tr, x1, y1)
		return
	}
	// Transform the control points first: the extrema of the transformed
	// curve generally occur at different parameters, so solving in source
	// space and transforming the extrema would under-cover the box.
	tx0, ty0 := transformX(tr, x0, y0), transformY(tr, x0, y0)
	tx1, ty1 := transformX(tr, cx1, cy1), transformY(tr, cx1, cy1)
	tx2, ty2 := transformX(tr, cx2, cy2), transformY(tr, cx2, cy2)
	tx3, ty3 := transformX(tr, x1, y1), transformY(tr, x1, y1)
	a.addPoint(tx0, ty0)
	a.addPoint(tx3, ty3)
	forEachCubicRoot(tx0, tx1, tx2, tx3, func(t float64) {
		a.addPoint(cubicEval(tx0, tx1, tx2, tx3, t), cubicEval(ty0, ty1, ty2, ty3, t))
	})
	forEachCubicRoot(ty0, ty1, ty2, ty3, func(t float64) {
		a.addPoint(cubicEval(tx0, tx1, tx2, tx3, t), cubicEval(ty0, ty1, ty2, ty3, t))
	})
}

func matrixFinite(tr Matrix) bool {
	for _, v := range tr {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

func strokeHalfWidth(w float64) float64 {
	if !isFinite(w) || w <= 0 {
		return 0
	}
	return w / 2
}

func allFinite(values ...float64) bool {
	for _, v := range values {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

// boundsWithMatrix is the shared implementation of the public bounds queries.
// It walks the path exactly like the flatteners do, but contributes analytic
// extrema instead of flattened vertices.
func (p *Path) boundsWithMatrix(tr Matrix, halfStrokeWidth float64) (minX, minY, maxX, maxY float64) {
	if p == nil || !matrixFinite(tr) {
		return 0, 0, 0, 0
	}

	var acc boundsAccumulator
	var curX, curY float64
	hasCur := false
	i := 0

	// A malformed path (more components than coordinate pairs) is handled
	// gracefully: whatever was parsed up to that point is returned.
Loop:
	for _, cmp := range p.Components {
		switch cmp {
		case MoveToCmp:
			if i+2 > len(p.Points) {
				break Loop
			}
			curX, curY = p.Points[i], p.Points[i+1]
			hasCur = true
			acc.addTransformed(tr, curX, curY)
			i += 2

		case LineToCmp:
			if i+2 > len(p.Points) {
				break Loop
			}
			x, y := p.Points[i], p.Points[i+1]
			// A segment has an axis-aligned bound given by its two
			// endpoints, so recording the endpoint is sufficient. When
			// the current point is non-finite the segment is undefined
			// and only the finite endpoint is recorded.
			acc.addTransformed(tr, x, y)
			curX, curY = x, y
			hasCur = true
			i += 2

		case QuadCurveToCmp:
			if i+4 > len(p.Points) {
				break Loop
			}
			cx, cy := p.Points[i], p.Points[i+1]
			x, y := p.Points[i+2], p.Points[i+3]
			if hasCur {
				addQuadBounds(&acc, tr, curX, curY, cx, cy, x, y)
			} else {
				// Hand-built path without a preceding point: the curve
				// is undefined, but its endpoint still belongs to it.
				acc.addTransformed(tr, x, y)
			}
			curX, curY = x, y
			hasCur = true
			i += 4

		case CubicCurveToCmp:
			if i+6 > len(p.Points) {
				break Loop
			}
			cx1, cy1 := p.Points[i], p.Points[i+1]
			cx2, cy2 := p.Points[i+2], p.Points[i+3]
			x, y := p.Points[i+4], p.Points[i+5]
			if hasCur {
				addCubicBounds(&acc, tr, curX, curY, cx1, cy1, cx2, cy2, x, y)
			} else {
				acc.addTransformed(tr, x, y)
			}
			curX, curY = x, y
			hasCur = true
			i += 6

		case ArcToCmp:
			if i+6 > len(p.Points) {
				break Loop
			}
			cx, cy := p.Points[i], p.Points[i+1]
			rx, ry := p.Points[i+2], p.Points[i+3]
			startAngle, angle := p.Points[i+4], p.Points[i+5]
			if !allFinite(cx, cy, rx, ry, startAngle, angle) {
				// Undefined arc: the current point is lost.
				hasCur = false
				i += 6
				continue
			}
			// ArcTo calls LineTo(start point) when the path is not empty;
			// that segment joins two points that are already recorded.
			addArcBounds(&acc, tr, cx, cy, rx, ry, startAngle, angle)
			// ArcTo leaves the current point at the arc end.
			curX = satAdd(cx, satMul(rx, math.Cos(startAngle+angle)))
			curY = satAdd(cy, satMul(ry, math.Sin(startAngle+angle)))
			hasCur = true
			i += 6

		case CloseCmp:
			// The closing segment joins the current point to the subpath
			// start. Both ends already belong to the path, so it cannot
			// change the axis-aligned box. The current point is left
			// unchanged, as Path.LastPoint does.
		}
	}

	return acc.result(halfStrokeWidth)
}
