// Copyright 2026 The draw2d Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw2d

import (
	"math"
	"sort"
)

// DefaultMeasureTolerance is the flattening tolerance used when a
// non-positive or non-finite tolerance is passed to NewPathMeasure. It is the
// maximum allowed distance, in path units, between a curve and the polyline
// used to measure it.
const DefaultMeasureTolerance = 0.01

const (
	// measureMaxDepth caps the adaptive subdivision of Bézier curves.
	measureMaxDepth = 32
	// measureMaxArcSteps caps the number of chord steps of one arc segment.
	measureMaxArcSteps = 1 << 16
)

// PathMeasure answers arc-length queries on a Path: total length and the
// position and tangent at any distance along the path. Distances are true
// arc lengths, so sampling at equally spaced distances moves at constant
// speed along the path, including over curved segments.
//
// The measure flattens every segment once at construction time, honouring
// the requested tolerance, so queries are cheap lookups. Points and tangents
// returned by PointAndTangentAt are evaluated at the same curve parameter
// (same time base).
//
// Queries are always stable: distances are clamped to [0, Length] (NaN is
// treated as 0), degenerate segments are skipped, and on an empty or
// zero-length path PointAt returns the first point of the path (or the
// origin) and TangentAt returns the unit vector (1, 0). No query returns
// NaN.
type PathMeasure struct {
	segs   []measureSegment
	cum    []float64 // cumulative length at the start of each segment, plus total
	length float64
	x0, y0 float64 // first point of the path, fallback for empty measures
}

// measureSegment is one flattened path segment with an arc-length lookup
// table. params and cum have the same length; cum is non-decreasing and
// starts at 0. eval returns the point and the derivative of the curve with
// respect to the parameter at t.
type measureSegment struct {
	params []float64
	cum    []float64
	length float64
	eval   func(t float64) (x, y, dx, dy float64)
}

// paramAt maps a distance s along the segment (0 <= s <= length) to a curve
// parameter. It also returns the index of the lookup-table interval
// containing s, used for chord fallbacks.
func (seg *measureSegment) paramAt(s float64) (t float64, idx int) {
	lo, hi := 0, len(seg.cum)-1
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if seg.cum[mid] <= s {
			lo = mid
		} else {
			hi = mid
		}
	}
	idx = lo
	span := seg.cum[idx+1] - seg.cum[idx]
	u := 0.0
	if span > 0 {
		u = (s - seg.cum[idx]) / span
		if u > 1 {
			u = 1
		}
	}
	t = seg.params[idx] + u*(seg.params[idx+1]-seg.params[idx])
	return t, idx
}

// NewPathMeasure builds a PathMeasure for p. tolerance is the maximum
// allowed distance between a curve and its polyline approximation, in path
// units; a non-positive or non-finite tolerance falls back to
// DefaultMeasureTolerance.
func NewPathMeasure(p *Path, tolerance float64) *PathMeasure {
	return NewPathMeasureTransform(p, NewIdentityMatrix(), tolerance)
}

// NewPathMeasureTransform builds a PathMeasure for p transformed by tr.
// Lengths, points and tangents are all expressed in transformed space, so
// distances remain true arc lengths under the matrix (for example a scaled
// path reports scaled lengths).
func NewPathMeasureTransform(p *Path, tr Matrix, tolerance float64) *PathMeasure {
	m := &PathMeasure{}
	if p == nil || p.IsEmpty() {
		return m
	}
	b := &measureBuilder{tr: tr, tol: sanitizeMeasureTolerance(tolerance)}
	var x, y, startX, startY float64
	j := 0
	for _, cmd := range p.Components {
		switch cmd {
		case MoveToCmp:
			x, y = p.Points[j], p.Points[j+1]
			startX, startY = x, y
			if !b.hasStart {
				b.x0, b.y0 = tr.TransformPoint(x, y)
				b.hasStart = true
			}
			j += 2
		case LineToCmp:
			nx, ny := p.Points[j], p.Points[j+1]
			b.addLine(x, y, nx, ny)
			x, y = nx, ny
			j += 2
		case QuadCurveToCmp:
			b.addQuad(x, y, p.Points[j], p.Points[j+1], p.Points[j+2], p.Points[j+3])
			x, y = p.Points[j+2], p.Points[j+3]
			j += 4
		case CubicCurveToCmp:
			b.addCubic(x, y, p.Points[j], p.Points[j+1], p.Points[j+2], p.Points[j+3], p.Points[j+4], p.Points[j+5])
			x, y = p.Points[j+4], p.Points[j+5]
			j += 6
		case ArcToCmp:
			cx, cy := p.Points[j], p.Points[j+1]
			rx, ry := p.Points[j+2], p.Points[j+3]
			start, angle := p.Points[j+4], p.Points[j+5]
			b.addArc(cx, cy, rx, ry, start, angle)
			x = cx + math.Cos(start+angle)*rx
			y = cy + math.Sin(start+angle)*ry
			j += 6
		case CloseCmp:
			if x != startX || y != startY {
				b.addLine(x, y, startX, startY)
			}
			x, y = startX, startY
		}
	}
	m.segs = b.segs
	m.cum = append(b.cum, b.length)
	m.length = b.length
	m.x0, m.y0 = b.x0, b.y0
	return m
}

// Measure returns a PathMeasure for p; see NewPathMeasure.
func (p *Path) Measure(tolerance float64) *PathMeasure {
	return NewPathMeasure(p, tolerance)
}

// ArcLength returns the total arc length of the path, measured with
// DefaultMeasureTolerance.
func (p *Path) ArcLength() float64 {
	return NewPathMeasure(p, DefaultMeasureTolerance).Length()
}

// Length returns the total arc length of the measured path.
func (m *PathMeasure) Length() float64 {
	return m.length
}

// Empty reports whether the measured path has zero length.
func (m *PathMeasure) Empty() bool {
	return len(m.segs) == 0
}

// PointAt returns the point at arc-length distance d along the path.
// d is clamped to [0, Length]; NaN is treated as 0.
func (m *PathMeasure) PointAt(d float64) (x, y float64) {
	if len(m.segs) == 0 {
		return m.x0, m.y0
	}
	seg, t, _ := m.locate(clampMeasureDistance(d, m.length))
	x, y, _, _ = seg.eval(t)
	return x, y
}

// TangentAt returns the unit tangent vector at arc-length distance d along
// the path, pointing in the direction of travel. Where the exact derivative
// vanishes (a cusp) the direction of the surrounding chord is returned
// instead, so the result is always a finite unit vector.
func (m *PathMeasure) TangentAt(d float64) (tx, ty float64) {
	_, _, tx, ty = m.PointAndTangentAt(d)
	return tx, ty
}

// PointAndTangentAt returns the point and the unit tangent at arc-length
// distance d. Both are evaluated at the same curve parameter, so they share
// one time base. d is clamped to [0, Length]; NaN is treated as 0.
func (m *PathMeasure) PointAndTangentAt(d float64) (x, y, tx, ty float64) {
	if len(m.segs) == 0 {
		return m.x0, m.y0, 1, 0
	}
	seg, t, idx := m.locate(clampMeasureDistance(d, m.length))
	x, y, dx, dy := seg.eval(t)
	var ok bool
	tx, ty, ok = unitVector(dx, dy)
	if !ok {
		// The derivative vanished (cusp) or was degenerate: fall back to
		// the direction of the lookup-table chord containing t.
		xa, ya, _, _ := seg.eval(seg.params[idx])
		xb, yb, _, _ := seg.eval(seg.params[idx+1])
		tx, ty, ok = unitVector(xb-xa, yb-ya)
		if !ok {
			tx, ty = 1, 0
		}
	}
	return x, y, tx, ty
}

// locate finds the segment containing arc-length distance d (already
// clamped to [0, m.length]) and maps d to a curve parameter on it.
func (m *PathMeasure) locate(d float64) (seg *measureSegment, t float64, idx int) {
	i := sort.Search(len(m.cum)-1, func(k int) bool { return m.cum[k+1] >= d })
	if i >= len(m.segs) {
		i = len(m.segs) - 1
	}
	seg = &m.segs[i]
	s := d - m.cum[i]
	if s < 0 {
		s = 0
	}
	if s > seg.length {
		s = seg.length
	}
	t, idx = seg.paramAt(s)
	return seg, t, idx
}

// clampMeasureDistance clamps d to [0, length]; NaN and negative distances
// map to 0, +Inf and over-long distances map to length.
func clampMeasureDistance(d, length float64) float64 {
	if math.IsNaN(d) || d <= 0 {
		return 0
	}
	if d >= length {
		return length
	}
	return d
}

// sanitizeMeasureTolerance replaces non-positive or non-finite tolerances
// with DefaultMeasureTolerance.
func sanitizeMeasureTolerance(tol float64) float64 {
	if tol <= 0 || math.IsNaN(tol) || math.IsInf(tol, 0) {
		return DefaultMeasureTolerance
	}
	return tol
}

// unitVector normalizes (dx, dy). ok is false when the vector is zero or
// non-finite.
func unitVector(dx, dy float64) (x, y float64, ok bool) {
	h := math.Hypot(dx, dy)
	if h <= 0 || math.IsNaN(h) || math.IsInf(h, 0) {
		return 0, 0, false
	}
	return dx / h, dy / h, true
}

// measureBuilder accumulates flattened segments while walking a Path.
type measureBuilder struct {
	tr       Matrix
	tol      float64
	segs     []measureSegment
	cum      []float64
	length   float64
	x0, y0   float64
	hasStart bool
}

// add appends a segment, dropping zero-length or non-finite ones so the
// lookup tables stay free of NaN.
func (b *measureBuilder) add(seg measureSegment) {
	if seg.length <= 0 || math.IsNaN(seg.length) || math.IsInf(seg.length, 0) {
		return
	}
	b.cum = append(b.cum, b.length)
	b.segs = append(b.segs, seg)
	b.length += seg.length
}

func (b *measureBuilder) addLine(x0, y0, x1, y1 float64) {
	ax, ay := b.tr.TransformPoint(x0, y0)
	bx, by := b.tr.TransformPoint(x1, y1)
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	b.add(measureSegment{
		params: []float64{0, 1},
		cum:    []float64{0, l},
		length: l,
		eval: func(t float64) (x, y, ddx, ddy float64) {
			return ax + t*dx, ay + t*dy, dx, dy
		},
	})
}

func (b *measureBuilder) addQuad(x0, y0, cx, cy, x1, y1 float64) {
	c := []float64{x0, y0, cx, cy, x1, y1}
	b.tr.Transform(c)
	tbl := &curveTable{}
	tbl.start(c[0], c[1], 0)
	subdivideQuadMeasure(tbl, c, 0, 1, 0, b.tol)
	b.add(measureSegment{
		params: tbl.params,
		cum:    tbl.cum,
		length: tbl.cum[len(tbl.cum)-1],
		eval:   quadMeasureEval(c),
	})
}

func (b *measureBuilder) addCubic(x0, y0, cx1, cy1, cx2, cy2, x1, y1 float64) {
	c := []float64{x0, y0, cx1, cy1, cx2, cy2, x1, y1}
	b.tr.Transform(c)
	tbl := &curveTable{}
	tbl.start(c[0], c[1], 0)
	subdivideCubicMeasure(tbl, c, 0, 1, 0, b.tol)
	b.add(measureSegment{
		params: tbl.params,
		cum:    tbl.cum,
		length: tbl.cum[len(tbl.cum)-1],
		eval:   cubicMeasureEval(c),
	})
}

func (b *measureBuilder) addArc(cx, cy, rx, ry, start, angle float64) {
	if angle == 0 || (rx == 0 && ry == 0) {
		return
	}
	tr := b.tr
	// dir makes the derivative point along the direction of travel, also
	// for arcs swept with a negative angle.
	dir := 1.0
	if angle < 0 {
		dir = -1
	}
	eval := func(theta float64) (x, y, dx, dy float64) {
		co, si := math.Cos(theta), math.Sin(theta)
		x, y = tr.TransformPoint(cx+co*rx, cy+si*ry)
		dx, dy = dir*-si*rx, dir*co*ry
		dx, dy = dx*tr[0]+dy*tr[2], dx*tr[1]+dy*tr[3]
		return x, y, dx, dy
	}
	// Pick an angular step whose chord deviates from the arc by at most
	// tol, using a conservative radius in transformed space.
	sx := math.Hypot(tr[0], tr[1])
	sy := math.Hypot(tr[2], tr[3])
	r := math.Max(math.Abs(rx), math.Abs(ry)) * math.Max(sx, sy)
	da := math.Pi / 2
	if r > b.tol {
		da = 2 * math.Acos(1-b.tol/r)
	}
	n := int(math.Ceil(math.Abs(angle) / da))
	if n < 1 {
		n = 1
	}
	if n > measureMaxArcSteps {
		n = measureMaxArcSteps
	}
	tbl := &curveTable{}
	x, y, _, _ := eval(start)
	tbl.start(x, y, start)
	for i := 1; i <= n; i++ {
		theta := start + angle*float64(i)/float64(n)
		x, y, _, _ := eval(theta)
		tbl.append(theta, x, y)
	}
	b.add(measureSegment{
		params: tbl.params,
		cum:    tbl.cum,
		length: tbl.cum[len(tbl.cum)-1],
		eval:   eval,
	})
}

// curveTable accumulates (parameter, cumulative arc length) samples while a
// curve is subdivided.
type curveTable struct {
	params       []float64
	cum          []float64
	lastX, lastY float64
}

func (t *curveTable) start(x, y, param float64) {
	t.params = append(t.params, param)
	t.cum = append(t.cum, 0)
	t.lastX, t.lastY = x, y
}

func (t *curveTable) append(param, x, y float64) {
	t.params = append(t.params, param)
	t.cum = append(t.cum, t.cum[len(t.cum)-1]+math.Hypot(x-t.lastX, y-t.lastY))
	t.lastX, t.lastY = x, y
}

// subdivideQuadMeasure recursively splits the quadratic Bézier c until the
// control point deviates from the chord by at most tol, appending the leaf
// end points to tbl. c holds x0, y0, cx, cy, x1, y1.
func subdivideQuadMeasure(tbl *curveTable, c []float64, t0, t1 float64, depth int, tol float64) {
	dx, dy := c[4]-c[0], c[5]-c[1]
	d := math.Abs((c[2]-c[4])*dy - (c[3]-c[5])*dx)
	if d*d <= tol*tol*(dx*dx+dy*dy) || depth >= measureMaxDepth {
		tbl.append(t1, c[4], c[5])
		return
	}
	var c1, c2 [6]float64
	c1[0], c1[1] = c[0], c[1]
	c2[4], c2[5] = c[4], c[5]
	c1[2] = (c[0] + c[2]) / 2
	c1[3] = (c[1] + c[3]) / 2
	c2[2] = (c[2] + c[4]) / 2
	c2[3] = (c[3] + c[5]) / 2
	c1[4] = (c1[2] + c2[2]) / 2
	c1[5] = (c1[3] + c2[3]) / 2
	c2[0], c2[1] = c1[4], c1[5]
	tm := (t0 + t1) / 2
	subdivideQuadMeasure(tbl, c1[:], t0, tm, depth+1, tol)
	subdivideQuadMeasure(tbl, c2[:], tm, t1, depth+1, tol)
}

// subdivideCubicMeasure recursively splits the cubic Bézier c until both
// control points deviate from the chord by at most tol, appending the leaf
// end points to tbl. c holds x0, y0, cx1, cy1, cx2, cy2, x1, y1.
func subdivideCubicMeasure(tbl *curveTable, c []float64, t0, t1 float64, depth int, tol float64) {
	dx, dy := c[6]-c[0], c[7]-c[1]
	d2 := math.Abs((c[2]-c[6])*dy - (c[3]-c[7])*dx)
	d3 := math.Abs((c[4]-c[6])*dy - (c[5]-c[7])*dx)
	if (d2+d3)*(d2+d3) <= tol*tol*(dx*dx+dy*dy) || depth >= measureMaxDepth {
		tbl.append(t1, c[6], c[7])
		return
	}
	var c1, c2 [8]float64
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
	tm := (t0 + t1) / 2
	subdivideCubicMeasure(tbl, c1[:], t0, tm, depth+1, tol)
	subdivideCubicMeasure(tbl, c2[:], tm, t1, depth+1, tol)
}

// quadMeasureEval returns point and derivative of the quadratic Bézier c at
// parameter t.
func quadMeasureEval(c []float64) func(t float64) (x, y, dx, dy float64) {
	x0, y0 := c[0], c[1]
	x1, y1 := c[2], c[3]
	x2, y2 := c[4], c[5]
	return func(t float64) (x, y, dx, dy float64) {
		mt := 1 - t
		x = mt*mt*x0 + 2*mt*t*x1 + t*t*x2
		y = mt*mt*y0 + 2*mt*t*y1 + t*t*y2
		dx = 2*mt*(x1-x0) + 2*t*(x2-x1)
		dy = 2*mt*(y1-y0) + 2*t*(y2-y1)
		return x, y, dx, dy
	}
}

// cubicMeasureEval returns point and derivative of the cubic Bézier c at
// parameter t.
func cubicMeasureEval(c []float64) func(t float64) (x, y, dx, dy float64) {
	x0, y0 := c[0], c[1]
	x1, y1 := c[2], c[3]
	x2, y2 := c[4], c[5]
	x3, y3 := c[6], c[7]
	return func(t float64) (x, y, dx, dy float64) {
		mt := 1 - t
		x = mt*mt*mt*x0 + 3*mt*mt*t*x1 + 3*mt*t*t*x2 + t*t*t*x3
		y = mt*mt*mt*y0 + 3*mt*mt*t*y1 + 3*mt*t*t*y2 + t*t*t*y3
		dx = 3*mt*mt*(x1-x0) + 6*mt*t*(x2-x1) + 3*t*t*(x3-x2)
		dy = 3*mt*mt*(y1-y0) + 6*mt*t*(y2-y1) + 3*t*t*(y3-y2)
		return x, y, dx, dy
	}
}
