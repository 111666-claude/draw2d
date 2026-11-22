// Copyright 2026 The draw2d Authors. All rights reserved.

package boolop

import "math"

// point is a 2D point used by the boolean engine.
type point struct {
	x, y float64
}

func padd(a, b point) point         { return point{a.x + b.x, a.y + b.y} }
func psub(a, b point) point         { return point{a.x - b.x, a.y - b.y} }
func pmul(a point, k float64) point { return point{a.x * k, a.y * k} }

// cross returns the z component of the 3D cross product a x b.
func cross(a, b point) float64 { return a.x*b.y - a.y*b.x }

func dot(a, b point) float64   { return a.x*b.x + a.y*b.y }
func plen(a point) float64     { return math.Hypot(a.x, a.y) }
func pdist(a, b point) float64 { return plen(psub(a, b)) }

// signedArea returns the signed area of a closed ring using the shoelace
// formula. Positive area means the ring is wound counter-clockwise in a
// y-up interpretation of the coordinate plane.
func signedArea(ring []point) float64 {
	var a float64
	for i, n := 0, len(ring); i < n; i++ {
		p, q := ring[i], ring[(i+1)%n]
		a += p.x*q.y - q.x*p.y
	}
	return a / 2
}

// windingNumber computes the winding number of p with respect to the given
// rings using the standard crossing rule.
func windingNumber(rings [][]point, p point) int {
	wn := 0
	for _, ring := range rings {
		for i, n := 0, len(ring); i < n; i++ {
			a, b := ring[i], ring[(i+1)%n]
			if a.y <= p.y {
				if b.y > p.y && cross(psub(b, a), psub(p, a)) > 0 {
					wn++
				}
			} else {
				if b.y <= p.y && cross(psub(b, a), psub(p, a)) < 0 {
					wn--
				}
			}
		}
	}
	return wn
}

// qpoint is a point quantized to the tolerance grid, used as a map key so
// that intersection points computed from different segment pairs meet.
type qpoint struct{ x, y int64 }

func quantize(p point, eps float64) qpoint {
	return qpoint{int64(math.Round(p.x / eps)), int64(math.Round(p.y / eps))}
}

// clockwiseAngle returns the clockwise angle from direction r to direction d
// in the range [0, 2*pi). It is used by the face-walking stitching rule.
func clockwiseAngle(r, d point) float64 {
	ang := math.Atan2(cross(r, d), dot(r, d)) // CCW angle from r to d
	cw := -ang
	if cw < 0 {
		cw += 2 * math.Pi
	}
	return cw
}

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// segment is a straight edge of a flattened operand.
type segment struct {
	p0, p1 point
}

func bboxOverlap(s1, s2 segment, eps float64) bool {
	return math.Min(s1.p0.x, s1.p1.x) <= math.Max(s2.p0.x, s2.p1.x)+eps &&
		math.Min(s2.p0.x, s2.p1.x) <= math.Max(s1.p0.x, s1.p1.x)+eps &&
		math.Min(s1.p0.y, s1.p1.y) <= math.Max(s2.p0.y, s2.p1.y)+eps &&
		math.Min(s2.p0.y, s2.p1.y) <= math.Max(s1.p0.y, s1.p1.y)+eps
}

// intersectParams returns the parameter pairs (t on s1, u on s2) at which the
// two segments meet. Proper crossings yield one pair; collinear overlaps yield
// the two pairs delimiting the overlap. eps is an absolute distance tolerance.
func intersectParams(s1, s2 segment, eps float64) [][2]float64 {
	d1 := psub(s1.p1, s1.p0)
	d2 := psub(s2.p1, s2.p0)
	l1 := plen(d1)
	l2 := plen(d2)
	if l1 == 0 || l2 == 0 {
		return nil
	}
	denom := cross(d1, d2)
	w := psub(s2.p0, s1.p0)
	if math.Abs(denom) > 1e-12*l1*l2 {
		// Proper (non-parallel) intersection.
		t := cross(w, d2) / denom
		u := cross(w, d1) / denom
		et := eps / l1
		eu := eps / l2
		if t >= -et && t <= 1+et && u >= -eu && u <= 1+eu {
			return [][2]float64{{clamp01(t), clamp01(u)}}
		}
		return nil
	}
	// Parallel: keep only collinear pairs.
	if math.Abs(cross(w, d1)) > eps*l1 {
		return nil
	}
	l1sq := dot(d1, d1)
	t3 := dot(psub(s2.p0, s1.p0), d1) / l1sq
	t4 := dot(psub(s2.p1, s1.p0), d1) / l1sq
	lo, hi := t3, t4
	if lo > hi {
		lo, hi = hi, lo
	}
	et := eps / l1
	if lo > 1+et || hi < -et {
		return nil
	}
	lo = clamp01(lo)
	hi = clamp01(hi)
	l2sq := dot(d2, d2)
	toU := func(t float64) float64 {
		p := padd(s1.p0, pmul(d1, t))
		return clamp01(dot(psub(p, s2.p0), d2) / l2sq)
	}
	return [][2]float64{{lo, toU(lo)}, {hi, toU(hi)}}
}
