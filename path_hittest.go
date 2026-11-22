// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 2026

package draw2d

import "math"

// hitTestCurveRecursionLimit is the maximum number of subdivisions used to
// flatten a Bézier curve for a hit test. It matches the flattening limit used
// by the rendering backends (see draw2dbase.CurveRecursionLimit).
const hitTestCurveRecursionLimit = 32

// Contains reports whether the point (x, y) is inside the path, using the
// identity transformation and the given fill rule.
//
// The path is not modified: curves and arcs are flattened into a temporary
// polygon representation that follows exactly the same subdivision rules as
// the one used when the path is filled. FillRuleEvenOdd and FillRuleWinding
// (non-zero) are supported; any other FillRule value is handled as
// FillRuleEvenOdd.
//
// A point lying exactly on the contour of the path is considered inside, as
// well as a point on the implicit segment that closes a subpath that was not
// terminated with Close. Empty, nil or fully degenerate paths never contain a
// point. Non finite points always return false and the function never panics,
// even for extremely large coordinates or malformed path data.
func (p *Path) Contains(x, y float64, fillRule FillRule) bool {
	return p.ContainsWithMatrix(x, y, NewIdentityMatrix(), fillRule)
}

// ContainsWithMatrix reports whether the point (x, y), expressed in the
// coordinate system obtained by applying tr to the path coordinate system,
// is inside the path. The query is equivalent to transforming the whole path
// with tr and testing the point against the transformed path, so the result
// is consistent with filling the path through the same matrix, including
// rotations, scales, mirrors (negative determinant) and translations.
//
// The path itself is never modified. A singular or non finite matrix cannot
// map plane regions and always yields false. Non finite points return false.
// See Contains for the handling of the fill rule, boundary points, empty and
// degenerate paths.
func (p *Path) ContainsWithMatrix(x, y float64, tr Matrix, fillRule FillRule) bool {
	if p == nil || p.IsEmpty() {
		return false
	}
	if !isFinite(x) || !isFinite(y) {
		return false
	}
	if !matrixIsFinite(tr) {
		return false
	}
	// A singular matrix collapses the path onto a line or a point: the
	// transformed path has no interior and the inverse transform does not
	// exist, so no point can be contained.
	det := tr.Determinant()
	if det == 0 {
		return false
	}
	// Flatten directly in device space. The arc segmentation step count used
	// by the rendering backends depends on the matrix scale only.
	f := &hitFlattener{tr: tr, scale: tr.GetScale()}
	f.flatten(p)
	return f.contains(x, y, fillRule)
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func fmin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func fmax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func matrixIsFinite(tr Matrix) bool {
	for _, v := range tr {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

// hitFlattener collects the path, transformed by tr and flattened into
// polygon subpaths (rings). Every stored point is expressed in the
// transformed (device) coordinate system.
//
// Ring boundaries follow the path stream exactly like the rendering
// rasterizers do: only a MoveTo starts a new ring. A drawing command that
// follows a Close without an intervening MoveTo keeps extending the current
// ring, producing a zero area "bridge" between the two shapes. The bridge is
// traversed twice in opposite directions, so it contributes neither an
// even-odd crossing nor a winding delta, exactly as for the rasterizer.
type hitFlattener struct {
	tr        Matrix
	scale     float64
	rings     [][]float64
	cur       []float64 // points of the ring being built, x0,y0,x1,y1,...
	subStartX float64   // transformed start point of the current subpath
	subStartY float64
}

func (f *hitFlattener) moveTo(x, y float64) {
	u, v := f.tr.TransformPoint(x, y)
	f.finishRing()
	f.cur = append(f.cur, u, v)
	f.subStartX, f.subStartY = u, v
}

func (f *hitFlattener) lineTo(x, y float64) {
	if len(f.cur) == 0 {
		// A drawing command without a preceding MoveTo (for example the
		// implicit LineTo emitted by ArcTo after a Close) continues the
		// current stream instead of starting a ring at the origin.
		f.moveTo(x, y)
		return
	}
	u, v := f.tr.TransformPoint(x, y)
	f.cur = append(f.cur, u, v)
}

// closeSubpath appends the closing segment back to the subpath start and
// stores the ring. This mirrors the Close handling of draw2dbase.Flatten.
func (f *hitFlattener) closeSubpath() {
	if len(f.cur) >= 4 {
		lastX, lastY := f.cur[len(f.cur)-2], f.cur[len(f.cur)-1]
		if lastX != f.subStartX || lastY != f.subStartY {
			f.cur = append(f.cur, f.subStartX, f.subStartY)
		}
		// Detach the stored ring from f.cur so that points appended to the
		// next subpath cannot overwrite the retained geometry.
		ring := make([]float64, len(f.cur))
		copy(ring, f.cur)
		f.rings = append(f.rings, ring)
	}
	f.cur = nil
}

// finishRing implicitly closes a ring left open, as required by the fill
// specification, and stores it.
func (f *hitFlattener) finishRing() {
	if len(f.cur) == 0 {
		return
	}
	if len(f.cur) < 4 {
		// A ring needs at least two distinct points to carry an edge.
		f.cur = f.cur[:0]
		return
	}
	lastX, lastY := f.cur[len(f.cur)-2], f.cur[len(f.cur)-1]
	if lastX != f.subStartX || lastY != f.subStartY {
		f.cur = append(f.cur, f.subStartX, f.subStartY)
	}
	ring := make([]float64, len(f.cur))
	copy(ring, f.cur)
	f.rings = append(f.rings, ring)
	f.cur = nil
}

// flatten walks the path components exactly like draw2dbase.Flatten does,
// while applying the transformation matrix to every emitted point.
func (f *hitFlattener) flatten(path *Path) {
	var x, y float64
	i := 0
	pts := path.Points
	for _, cmp := range path.Components {
		switch cmp {
		case MoveToCmp:
			if i+1 >= len(pts) {
				return
			}
			x, y = pts[i], pts[i+1]
			f.moveTo(x, y)
			i += 2
		case LineToCmp:
			if i+1 >= len(pts) {
				break
			}
			x, y = pts[i], pts[i+1]
			f.lineTo(x, y)
			i += 2
		case QuadCurveToCmp:
			if i+3 >= len(pts) {
				break
			}
			// pts[i-2:] holds the current point followed by the control
			// point and the end point of the quadratic Bézier.
			if i >= 2 {
				f.traceQuad(pts[i-2 : i+4])
			}
			x, y = pts[i+2], pts[i+3]
			f.lineTo(x, y)
			i += 4
		case CubicCurveToCmp:
			if i+5 >= len(pts) {
				break
			}
			if i >= 2 {
				f.traceCubic(pts[i-2 : i+6])
			}
			x, y = pts[i+4], pts[i+5]
			f.lineTo(x, y)
			i += 6
		case ArcToCmp:
			if i+5 >= len(pts) {
				break
			}
			f.traceArc(pts[i], pts[i+1], pts[i+2], pts[i+3], pts[i+4], pts[i+5])
			x, y = pts[i]+math.Cos(pts[i+4]+pts[i+5])*pts[i+2],
				pts[i+1]+math.Sin(pts[i+4]+pts[i+5])*pts[i+3]
			f.lineTo(x, y)
			i += 6
		case CloseCmp:
			// Flatten emits a line back to the subpath start then marks the
			// ring closed. The ring is retained so that any subsequent
			// drawing command shares the same polygon stream.
			f.closeSubpath()
		}
	}
	f.finishRing()
}

// subdivideCubic splits the cubic Bézier c (8 floats) into c1 and c2.
func subdivideCubic(c, c1, c2 []float64) {
	c1[0], c1[1] = c[0], c[1]
	c2[6], c2[7] = c[6], c[7]
	c1[2] = (c[0] + c[2]) / 2
	c1[3] = (c[1] + c[3]) / 2
	midX := (c[2] + c[4]) / 2
	midY := (c[3] + c[5]) / 2
	c2[4] = (c[4] + c[6]) / 2
	c2[5] = (c[5] + c[7]) / 2
	c1[4] = (c1[2] + midX) / 2
	c1[5] = (c1[3] + midY) / 2
	c2[2] = (midX + c2[4]) / 2
	c2[3] = (midY + c2[5]) / 2
	c1[6] = (c1[4] + c2[2]) / 2
	c1[7] = (c1[5] + c2[3]) / 2
	c2[0], c2[1] = c1[6], c1[7]
}

// traceCubic flattens the cubic Bézier cubic with the same flatness threshold
// (0.5) used by the rendering backends.
func (f *hitFlattener) traceCubic(cubic []float64) {
	var curves [hitTestCurveRecursionLimit * 8]float64
	copy(curves[0:8], cubic[0:8])
	i := 0
	var c []float64
	var dx, dy, d2, d3 float64
	for i >= 0 {
		c = curves[i:]
		dx = c[6] - c[0]
		dy = c[7] - c[1]
		d2 = math.Abs((c[2]-c[6])*dy - (c[3]-c[7])*dx)
		d3 = math.Abs((c[4]-c[6])*dy - (c[5]-c[7])*dx)
		if (d2+d3)*(d2+d3) <= 0.5*(dx*dx+dy*dy) || i == len(curves)-8 {
			f.lineTo(c[6], c[7])
			i -= 8
		} else {
			subdivideCubic(c, curves[i+8:], curves[i:])
			i += 8
		}
	}
}

// subdivideQuad splits the quadratic Bézier c (6 floats) into c1 and c2.
func subdivideQuad(c, c1, c2 []float64) {
	c1[0], c1[1] = c[0], c[1]
	c2[4], c2[5] = c[4], c[5]
	c1[2] = (c[0] + c[2]) / 2
	c1[3] = (c[1] + c[3]) / 2
	c2[2] = (c[2] + c[4]) / 2
	c2[3] = (c[3] + c[5]) / 2
	c1[4] = (c1[2] + c2[2]) / 2
	c1[5] = (c1[3] + c2[3]) / 2
	c2[0], c2[1] = c1[4], c1[5]
}

// traceQuad flattens the quadratic Bézier quad with the same flatness
// threshold (0.5) used by the rendering backends.
func (f *hitFlattener) traceQuad(quad []float64) {
	var curves [hitTestCurveRecursionLimit * 6]float64
	copy(curves[0:6], quad[0:6])
	i := 0
	var c []float64
	var dx, dy, d float64
	for i >= 0 {
		c = curves[i:]
		dx = c[4] - c[0]
		dy = c[5] - c[1]
		d = math.Abs((c[2]-c[4])*dy - (c[3]-c[5])*dx)
		if d*d <= 0.5*(dx*dx+dy*dy) || i == len(curves)-6 {
			f.lineTo(c[4], c[5])
			i -= 6
		} else {
			subdivideQuad(c, curves[i+6:], curves[i:])
			i += 6
		}
	}
}

// traceArc flattens an elliptical arc. It mirrors draw2dbase.TraceArc, whose
// segment count depends on the matrix scale, so that the hit polygon matches
// the polygon fed to the rasterizer.
func (f *hitFlattener) traceArc(cx, cy, rx, ry, start, angle float64) {
	end := start + angle
	clockWise := angle >= 0
	ra := (math.Abs(rx) + math.Abs(ry)) / 2
	da := math.Acos(ra/(ra+0.125/f.scale)) * 2
	if !clockWise {
		da = -da
	}
	a := start + da
	var curX, curY float64
	for {
		if (a < end-da/4) != clockWise {
			curX = cx + math.Cos(end)*rx
			curY = cy + math.Sin(end)*ry
			f.lineTo(curX, curY)
			return
		}
		curX = cx + math.Cos(a)*rx
		curY = cy + math.Sin(a)*ry
		a += da
		f.lineTo(curX, curY)
	}
}

// contains evaluates the point in polygon test over every flattened ring and
// combines the per-ring contributions according to fillRule. Every ring is
// treated as closed.
func (f *hitFlattener) contains(x, y float64, fillRule FillRule) bool {
	nonZero := fillRule == FillRuleWinding
	crossings := 0
	winding := 0
	for _, ring := range f.rings {
		if len(ring) < 4 {
			continue
		}
		for i := 0; i+3 < len(ring); i += 2 {
			x0, y0 := ring[i], ring[i+1]
			x1, y1 := ring[i+2], ring[i+3]
			// Skip degenerate edges and any edge carrying non finite data;
			// such edges cannot delimit a fill region and would poison the
			// ray casting arithmetic.
			if x0 == x1 && y0 == y1 {
				continue
			}
			if !isFinite(x0) || !isFinite(y0) || !isFinite(x1) || !isFinite(y1) {
				continue
			}
			// Boundary points are inside, including the implicit closing
			// segment of an open subpath.
			if pointOnSegment(x, y, x0, y0, x1, y1) {
				return true
			}
			// Half-open horizontal ray to +infinity. The half-open interval
			// makes vertices shared by two edges count exactly once.
			if y0 <= y && y1 > y { // edge straddles the ray upward
				if intersectsRayRight(x, y, x0, y0, x1, y1) {
					crossings++
					winding++
				}
			} else if y1 <= y && y0 > y { // edge straddles the ray downward
				if intersectsRayRight(x, y, x0, y0, x1, y1) {
					crossings++
					winding--
				}
			}
		}
	}
	if nonZero {
		return winding != 0
	}
	return crossings%2 != 0
}

// intersectsRayRight reports whether the edge (x0,y0)-(x1,y1), known to
// straddle the horizontal line through (x,y), crosses the ray from (x,y)
// towards +infinity on the x axis.
func intersectsRayRight(px, py, x0, y0, x1, y1 float64) bool {
	// x coordinate of the edge at y = py, computed without division by the
	// vertical delta which is guaranteed non zero here.
	dx := x1 - x0
	dy := y1 - y0
	xi := x0 + dx*(py-y0)/dy
	return xi > px
}

// pointOnSegment reports whether (px,py) lies on the segment
// (x0,y0)-(x1,y1), using a cross product collinearity test followed by a
// bounding box check.
func pointOnSegment(px, py, x0, y0, x1, y1 float64) bool {
	cross := (x1-x0)*(py-y0) - (y1-y0)*(px-x0)
	if cross != 0 {
		return false
	}
	return px >= fmin(x0, x1) && px <= fmax(x0, x1) &&
		py >= fmin(y0, y1) && py <= fmax(y0, y1)
}
