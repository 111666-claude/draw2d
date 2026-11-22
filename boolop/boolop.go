// Copyright 2026 The draw2d Authors. All rights reserved.

// Package boolop provides bounded boolean operations (union, intersection,
// difference, xor) on closed draw2d paths, so that several outlines can be
// merged into a single path that can be filled again — for example to cut
// holes out of a badge shape without hand-splitting contours.
//
// Operands may contain any mix of lines, quadratic and cubic Bézier curves
// and arcs, organized in one or more closed subpaths. Curves are flattened
// with the same routines and tolerance the rendering backends use, so the
// result keeps the approximation precision of a drawn path. The result is an
// ordinary serializable draw2d.Path whose rings carry a stable orientation
// (outer rings counter-clockwise, holes clockwise), and which fills
// identically under the even-odd and the non-zero winding rule. Input paths
// are never modified.
//
// Scope and limits: the engine works on the flattened operands with a
// relative tolerance of 1e-9 of the bounding box diagonal. Non-finite
// coordinates, coordinates beyond MaxCoordinate, and inputs exceeding the
// configured complexity limits are rejected with explicit errors. Features
// thinner than about 1e-6 of the bounding box diagonal may be classified
// unreliably; exactly tangent or coincident configurations are handled.
package boolop

import (
	"errors"
	"math"
	"sort"

	"github.com/llgcode/draw2d"
)

// Op identifies a boolean set operation on two filled regions.
type Op int

const (
	// Union keeps everything inside either operand.
	Union Op = iota
	// Intersection keeps everything inside both operands.
	Intersection
	// Difference keeps what is inside the first operand but not the second.
	Difference
	// XOR keeps what is inside exactly one operand.
	XOR
)

func (op Op) String() string {
	switch op {
	case Union:
		return "Union"
	case Intersection:
		return "Intersection"
	case Difference:
		return "Difference"
	case XOR:
		return "XOR"
	}
	return "Unknown"
}

func (op Op) apply(inA, inB bool) bool {
	switch op {
	case Union:
		return inA || inB
	case Intersection:
		return inA && inB
	case Difference:
		return inA && !inB
	case XOR:
		return inA != inB
	}
	return false
}

// Errors returned by Boolean.
var (
	// ErrNonFinite is returned when an operand contains NaN or Inf points.
	ErrNonFinite = errors.New("boolop: path contains non-finite coordinates")
	// ErrCoordinateRange is returned when a coordinate magnitude exceeds
	// MaxCoordinate.
	ErrCoordinateRange = errors.New("boolop: coordinate magnitude exceeds MaxCoordinate")
	// ErrTooComplex is returned when the flattened inputs or the computed
	// arrangement exceed the configured complexity limits.
	ErrTooComplex = errors.New("boolop: operation exceeds configured complexity limit")
	// ErrOpenBoundary is returned when the result boundary cannot be
	// stitched into closed rings; this indicates numerically ambiguous
	// input such as features below the working tolerance.
	ErrOpenBoundary = errors.New("boolop: result boundary could not be closed")
)

// MaxCoordinate bounds the absolute magnitude of accepted coordinates.
// Beyond it double precision no longer guarantees meaningful relative
// tolerances for the boolean engine.
const MaxCoordinate = 1 << 50

// Default complexity limits.
const (
	// DefaultMaxSegments caps the number of flattened segments per operand.
	DefaultMaxSegments = 4096
	// DefaultMaxAtomicSegments caps the number of segments after splitting
	// at intersections.
	DefaultMaxAtomicSegments = 1 << 16
)

// Options configures a boolean operation. The zero value is valid and
// matches the draw2d defaults (even-odd fill rule, unit flattening scale).
type Options struct {
	// FillRule determines how the input paths are interpreted (and is the
	// rule the result is optimized for). The zero value,
	// draw2d.FillRuleEvenOdd, matches the draw2d default.
	FillRule draw2d.FillRule
	// Scale is the flattening scale forwarded to the curve flattener, like
	// the scale of a transformation matrix. Default 1.
	Scale float64
	// MaxSegments caps the number of flattened segments per operand.
	// Default DefaultMaxSegments.
	MaxSegments int
	// MaxAtomicSegments caps the number of segments after splitting at
	// intersections. Default DefaultMaxAtomicSegments.
	MaxAtomicSegments int
}

type options struct {
	fillRule  draw2d.FillRule
	scale     float64
	maxSegs   int
	maxAtomic int
}

func resolveOptions(o *Options) options {
	r := options{
		fillRule:  draw2d.FillRuleEvenOdd,
		scale:     1,
		maxSegs:   DefaultMaxSegments,
		maxAtomic: DefaultMaxAtomicSegments,
	}
	if o != nil {
		r.fillRule = o.FillRule
		if o.Scale != 0 {
			r.scale = o.Scale
		}
		if o.MaxSegments > 0 {
			r.maxSegs = o.MaxSegments
		}
		if o.MaxAtomicSegments > 0 {
			r.maxAtomic = o.MaxAtomicSegments
		}
	}
	return r
}

// Boolean computes the boolean operation op between the filled regions
// described by paths a and b and returns the boundary of the result as a new
// path. The inputs are not modified. The result consists of closed subpaths
// (lines only, since curves are flattened with rendering precision) whose
// outer rings are oriented counter-clockwise and whose holes are oriented
// clockwise; it fills identically under both fill rules and can be drawn by
// any draw2d backend or serialized like any other path.
//
// A result with no area (for example the intersection of disjoint shapes) is
// returned as an empty, non-nil path.
func Boolean(op Op, a, b *draw2d.Path, opts *Options) (*draw2d.Path, error) {
	o := resolveOptions(opts)
	ringsA, err := flattenPath(a, o)
	if err != nil {
		return nil, err
	}
	ringsB, err := flattenPath(b, o)
	if err != nil {
		return nil, err
	}
	rings, err := booleanRings(op, ringsA, ringsB, o)
	if err != nil {
		return nil, err
	}
	result := &draw2d.Path{}
	for _, r := range rings {
		result.MoveTo(r[0].x, r[0].y)
		for _, p := range r[1:] {
			result.LineTo(p.x, p.y)
		}
		result.Close()
	}
	return result, nil
}

// booleanRings runs the core engine and returns the result rings, filtered
// and canonically ordered.
func booleanRings(op Op, a, b [][]point, o options) ([][]point, error) {
	segsA := ringSegments(a)
	segsB := ringSegments(b)
	if len(segsA) > o.maxSegs || len(segsB) > o.maxSegs {
		return nil, ErrTooComplex
	}
	all := make([]segment, 0, len(segsA)+len(segsB))
	all = append(all, segsA...)
	all = append(all, segsB...)
	if len(all) == 0 {
		return nil, nil
	}

	// Working tolerances, relative to the bounding box diagonal.
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, s := range all {
		for _, p := range [2]point{s.p0, s.p1} {
			minX = math.Min(minX, p.x)
			minY = math.Min(minY, p.y)
			maxX = math.Max(maxX, p.x)
			maxY = math.Max(maxY, p.y)
		}
	}
	diag := math.Hypot(maxX-minX, maxY-minY)
	if diag == 0 {
		return nil, nil
	}
	eps := diag * 1e-9   // vertex coincidence tolerance
	delta := diag * 1e-6 // classification sampling offset

	// Split every segment at every crossing with the other operand. Split
	// points found on one segment can force further splits of a collinear
	// overlapping counterpart, so iterate to a fixpoint.
	for iter := 0; ; iter++ {
		if iter > 16 {
			return nil, ErrTooComplex
		}
		nA := len(segsA)
		all := make([]segment, 0, nA+len(segsB))
		all = append(all, segsA...)
		all = append(all, segsB...)
		splits := make([][]float64, len(all))
		for i := 0; i < nA; i++ {
			for j := nA; j < len(all); j++ {
				if !bboxOverlap(all[i], all[j], eps) {
					continue
				}
				for _, tu := range intersectParams(all[i], all[j], eps) {
					splits[i] = append(splits[i], tu[0])
					splits[j] = append(splits[j], tu[1])
				}
			}
		}
		newA := splitSegments(segsA, splits[:nA], eps)
		newB := splitSegments(segsB, splits[nA:], eps)
		if len(newA)+len(newB) > o.maxAtomic {
			return nil, ErrTooComplex
		}
		if len(newA) == len(segsA) && len(newB) == len(segsB) {
			break // fixpoint: no new split points
		}
		segsA, segsB = newA, newB
	}
	atomic := make([]segment, 0, len(segsA)+len(segsB))
	atomic = append(atomic, segsA...)
	atomic = append(atomic, segsB...)

	// Keep the atomic segments that bound the result region, oriented so
	// the result interior lies on their left.
	var kept []segment
	for _, s := range atomic {
		d := psub(s.p1, s.p0)
		l := plen(d)
		if l == 0 {
			continue
		}
		n := point{-d.y / l, d.x / l} // left normal
		mid := pmul(padd(s.p0, s.p1), 0.5)
		pl := padd(mid, pmul(n, delta))
		pr := psub(mid, pmul(n, delta))
		resL := op.apply(inside(windingNumber(a, pl), o.fillRule),
			inside(windingNumber(b, pl), o.fillRule))
		resR := op.apply(inside(windingNumber(a, pr), o.fillRule),
			inside(windingNumber(b, pr), o.fillRule))
		if resL == resR {
			continue
		}
		if resL {
			kept = append(kept, s)
		} else {
			kept = append(kept, segment{s.p1, s.p0})
		}
	}
	kept = dedupSegments(kept, eps)

	rings, err := stitch(kept, eps)
	if err != nil {
		return nil, err
	}

	// Drop degenerate slivers and order rings canonically: largest signed
	// area first, so outer rings (positive area) precede holes.
	type outRing struct {
		pts  []point
		area float64
	}
	var outs []outRing
	for _, r := range rings {
		area := signedArea(r)
		if math.Abs(area) <= eps*eps {
			continue
		}
		outs = append(outs, outRing{r, area})
	}
	sort.SliceStable(outs, func(i, j int) bool { return outs[i].area > outs[j].area })
	result := make([][]point, len(outs))
	for i, o := range outs {
		result[i] = o.pts
	}
	return result, nil
}

// ringSegments flattens rings into a segment list.
func ringSegments(rings [][]point) []segment {
	var segs []segment
	for _, ring := range rings {
		for i, n := 0, len(ring); i < n; i++ {
			segs = append(segs, segment{ring[i], ring[(i+1)%n]})
		}
	}
	return segs
}

// splitSegments cuts each segment at the given parameter values, dropping
// pieces shorter than the tolerance.
func splitSegments(segs []segment, splits [][]float64, eps float64) []segment {
	out := make([]segment, 0, len(segs))
	for i, s := range segs {
		d := psub(s.p1, s.p0)
		l := plen(d)
		if l < eps {
			continue
		}
		et := eps / l
		ts := append(splits[i], 0, 1)
		sort.Float64s(ts)
		prev := ts[0]
		for k := 1; k < len(ts); k++ {
			t := ts[k]
			if t-prev < et {
				continue
			}
			out = append(out, segment{
				padd(s.p0, pmul(d, prev)),
				padd(s.p0, pmul(d, t)),
			})
			prev = t
		}
	}
	return out
}

// inside converts a winding number to a boolean according to the fill rule.
func inside(wn int, rule draw2d.FillRule) bool {
	if rule == draw2d.FillRuleWinding {
		return wn != 0
	}
	return wn%2 != 0
}

// dedupSegments merges coincident boundary segments contributed by both
// operands: duplicates with the same direction collapse into one, duplicates
// with opposite directions cancel out.
func dedupSegments(segs []segment, eps float64) []segment {
	type edgeKey struct{ a, b qpoint }
	var out []segment
	open := make(map[edgeKey]int) // directed key -> index in out
	cancelled := make([]bool, 0, len(segs))
	for _, s := range segs {
		k := edgeKey{quantize(s.p0, eps), quantize(s.p1, eps)}
		kr := edgeKey{k.b, k.a}
		if _, dup := open[k]; dup {
			continue
		}
		if pos, rev := open[kr]; rev {
			cancelled[pos] = true
			delete(open, kr)
			continue
		}
		open[k] = len(out)
		out = append(out, s)
		cancelled = append(cancelled, false)
	}
	result := out[:0]
	for i, s := range out {
		if !cancelled[i] {
			result = append(result, s)
		}
	}
	return result
}

// stitch chains directed boundary segments into closed rings. At each vertex
// the walk continues with the unused segment that forms the smallest
// clockwise angle from the reverse of the incoming direction, which keeps the
// result interior consistently on the left of the walk.
func stitch(segs []segment, eps float64) ([][]point, error) {
	byStart := make(map[qpoint][]int)
	for i, s := range segs {
		q := quantize(s.p0, eps)
		byStart[q] = append(byStart[q], i)
	}
	used := make([]bool, len(segs))
	var rings [][]point
	for i := range segs {
		if used[i] {
			continue
		}
		used[i] = true
		ring := []point{segs[i].p0}
		start := quantize(segs[i].p0, eps)
		cur := i
		for {
			v := segs[cur].p1
			if quantize(v, eps) == start {
				break
			}
			ring = append(ring, v)
			r := psub(segs[cur].p0, v) // reverse of the incoming direction
			best, bestScore := -1, math.Inf(1)
			try := func(j int) {
				if used[j] {
					return
				}
				d := psub(segs[j].p1, segs[j].p0)
				if score := clockwiseAngle(r, d); score < bestScore {
					bestScore, best = score, j
				}
			}
			for _, j := range byStart[quantize(v, eps)] {
				try(j)
			}
			if best < 0 {
				// Tolerance fallback: scan for a nearby start point.
				for j := range segs {
					if !used[j] && pdist(segs[j].p0, v) <= 4*eps {
						try(j)
					}
				}
			}
			if best < 0 {
				return nil, ErrOpenBoundary
			}
			used[best] = true
			cur = best
		}
		if len(ring) >= 3 {
			rings = append(rings, ring)
		}
	}
	return rings, nil
}
