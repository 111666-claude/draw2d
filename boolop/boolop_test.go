// Copyright 2026 The draw2d Authors. All rights reserved.

package boolop

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/llgcode/draw2d"
)

// --- helpers ---------------------------------------------------------------

func squarePath(x0, y0, x1, y1 float64) *draw2d.Path {
	p := new(draw2d.Path)
	p.MoveTo(x0, y0)
	p.LineTo(x1, y0)
	p.LineTo(x1, y1)
	p.LineTo(x0, y1)
	p.Close()
	return p
}

func circlePath(cx, cy, r float64) *draw2d.Path {
	p := new(draw2d.Path)
	p.ArcTo(cx, cy, r, r, 0, 2*math.Pi)
	p.Close()
	return p
}

func ringsOf(t *testing.T, p *draw2d.Path) [][]point {
	t.Helper()
	rings, err := flattenPath(p, resolveOptions(nil))
	if err != nil {
		t.Fatalf("flatten result: %v", err)
	}
	return rings
}

// areaOf returns the net signed area of a path (outer rings positive).
func areaOf(t *testing.T, p *draw2d.Path) float64 {
	t.Helper()
	var sum float64
	for _, r := range ringsOf(t, p) {
		sum += signedArea(r)
	}
	return sum
}

func boolArea(t *testing.T, op Op, a, b *draw2d.Path, opts *Options) float64 {
	t.Helper()
	res, err := Boolean(op, a, b, opts)
	if err != nil {
		t.Fatalf("Boolean(%v): %v", op, err)
	}
	return math.Abs(areaOf(t, res))
}

func almostEq(got, want, relTol float64) bool {
	if want == 0 {
		return math.Abs(got) < relTol
	}
	return math.Abs(got-want)/math.Abs(want) < relTol
}

// circleLens is the analytic overlap area of two circles of radius r whose
// centers are d apart.
func circleLens(d, r float64) float64 {
	return 2*r*r*math.Acos(d/(2*r)) - (d/2)*math.Sqrt(4*r*r-d*d)
}

// --- intersecting circles --------------------------------------------------

func TestIntersectingCirclesArea(t *testing.T) {
	r, d := 40.0, 40.0
	a := circlePath(80, 100, r)
	b := circlePath(80+d, 100, r)
	lens := circleLens(d, r)
	circle := math.Pi * r * r

	cases := []struct {
		op   Op
		want float64
	}{
		{Union, 2*circle - lens},
		{Intersection, lens},
		{Difference, circle - lens},
		{XOR, 2 * (circle - lens)},
	}
	for _, c := range cases {
		got := boolArea(t, c.op, a, b, nil)
		if !almostEq(got, c.want, 0.01) {
			t.Errorf("%v area = %v, want %v (1%% tolerance)", c.op, got, c.want)
		}
	}
}

func TestConcentricCirclesDifferenceMakesHole(t *testing.T) {
	a := circlePath(100, 100, 60)
	b := circlePath(100, 100, 30)
	res, err := Boolean(Difference, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	rings := ringsOf(t, res)
	if len(rings) != 2 {
		t.Fatalf("expected 2 rings (outer + hole), got %d", len(rings))
	}
	var outer, hole float64
	for _, r := range rings {
		if a := signedArea(r); a > 0 {
			outer = a
		} else {
			hole = a
		}
	}
	if outer <= 0 || hole >= 0 {
		t.Errorf("outer ring must be CCW (positive) and hole CW (negative), got outer=%v hole=%v", outer, hole)
	}
	if got, want := outer+hole, math.Pi*(60*60-30*30); !almostEq(got, want, 0.01) {
		t.Errorf("ring area = %v, want %v", got, want)
	}
}

// --- polygons: disjoint, tangent, coincident -------------------------------

func TestDisjointSquares(t *testing.T) {
	a := squarePath(0, 0, 10, 10)
	b := squarePath(20, 0, 30, 10)

	res, err := Boolean(Union, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rings := ringsOf(t, res); len(rings) != 2 {
		t.Errorf("union of disjoint squares: expected 2 rings, got %d", len(rings))
	}
	if got := math.Abs(areaOf(t, res)); got != 200 {
		t.Errorf("union area = %v, want 200", got)
	}

	res, err = Boolean(Intersection, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsEmpty() {
		t.Errorf("intersection of disjoint squares should be empty, got %v", res)
	}

	if got := boolArea(t, Difference, a, b, nil); got != 100 {
		t.Errorf("difference area = %v, want 100", got)
	}
	if got := boolArea(t, Difference, b, a, nil); got != 100 {
		t.Errorf("reversed difference area = %v, want 100", got)
	}
}

func TestTangentEdgeSquares(t *testing.T) {
	a := squarePath(0, 0, 10, 10)
	b := squarePath(10, 0, 20, 10) // shares the edge x=10

	res, err := Boolean(Union, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rings := ringsOf(t, res); len(rings) != 1 {
		t.Errorf("union of edge-sharing squares: expected 1 ring, got %d", len(rings))
	}
	if got := math.Abs(areaOf(t, res)); !almostEq(got, 200, 1e-9) {
		t.Errorf("union area = %v, want 200", got)
	}

	res, err = Boolean(Intersection, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsEmpty() {
		t.Errorf("intersection of edge-tangent squares should be empty, got area %v", areaOf(t, res))
	}

	if got := boolArea(t, Difference, a, b, nil); !almostEq(got, 100, 1e-9) {
		t.Errorf("difference area = %v, want 100", got)
	}
}

func TestTangentPointSquares(t *testing.T) {
	a := squarePath(0, 0, 10, 10)
	b := squarePath(10, 10, 20, 20) // touches at the single point (10,10)

	res, err := Boolean(Union, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rings := ringsOf(t, res); len(rings) != 2 {
		t.Errorf("union of point-tangent squares: expected 2 rings, got %d", len(rings))
	}
	if got := math.Abs(areaOf(t, res)); !almostEq(got, 200, 1e-9) {
		t.Errorf("union area = %v, want 200", got)
	}

	res, err = Boolean(Intersection, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsEmpty() {
		t.Errorf("intersection of point-tangent squares should be empty, got area %v", areaOf(t, res))
	}
}

func TestCoincidentPartialEdges(t *testing.T) {
	a := squarePath(0, 0, 100, 100)
	b := squarePath(50, 0, 150, 100) // top and bottom edges partially coincide

	if got := boolArea(t, Union, a, b, nil); !almostEq(got, 15000, 1e-9) {
		t.Errorf("union area = %v, want 15000", got)
	}
	res, err := Boolean(Union, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rings := ringsOf(t, res); len(rings) != 1 {
		t.Errorf("union: expected 1 ring, got %d", len(rings))
	}
	if got := boolArea(t, Intersection, a, b, nil); !almostEq(got, 5000, 1e-9) {
		t.Errorf("intersection area = %v, want 5000", got)
	}
	if got := boolArea(t, Difference, a, b, nil); !almostEq(got, 5000, 1e-9) {
		t.Errorf("difference area = %v, want 5000", got)
	}
	if got := boolArea(t, Difference, b, a, nil); !almostEq(got, 5000, 1e-9) {
		t.Errorf("reversed difference area = %v, want 5000", got)
	}
}

func TestTangentCircles(t *testing.T) {
	r := 40.0
	a := circlePath(100, 100, r)
	b := circlePath(100+2*r, 100, r) // externally tangent

	// Union keeps essentially both circles.
	if got := boolArea(t, Union, a, b, nil); !almostEq(got, 2*math.Pi*r*r, 0.01) {
		t.Errorf("union area = %v, want %v", got, 2*math.Pi*r*r)
	}
	// Intersection of tangent circles has no area; flattening may leave a
	// sliver well below 2% of a circle.
	res, err := Boolean(Intersection, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := math.Abs(areaOf(t, res)); got > 0.02*math.Pi*r*r {
		t.Errorf("intersection of tangent circles = %v, expected near zero", got)
	}
}

// --- containment and holes -------------------------------------------------

func TestContainmentNoCrossings(t *testing.T) {
	a := squarePath(0, 0, 100, 100)
	b := squarePath(20, 20, 40, 40) // fully inside a, no edge crossings

	if got := boolArea(t, Union, a, b, nil); !almostEq(got, 10000, 1e-9) {
		t.Errorf("union area = %v, want 10000", got)
	}
	if got := boolArea(t, Intersection, a, b, nil); !almostEq(got, 400, 1e-9) {
		t.Errorf("intersection area = %v, want 400", got)
	}
	res, err := Boolean(Difference, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	rings := ringsOf(t, res)
	if len(rings) != 2 {
		t.Fatalf("difference with contained square: expected 2 rings, got %d", len(rings))
	}
	if signedArea(rings[0]) <= 0 || signedArea(rings[1]) >= 0 {
		t.Errorf("expected outer ring positive then hole negative, got %v, %v",
			signedArea(rings[0]), signedArea(rings[1]))
	}
	if got := math.Abs(areaOf(t, res)); !almostEq(got, 9600, 1e-9) {
		t.Errorf("difference area = %v, want 9600", got)
	}
}

func holedSquare() *draw2d.Path {
	p := new(draw2d.Path)
	// outer ring CCW
	p.MoveTo(20, 20)
	p.LineTo(180, 20)
	p.LineTo(180, 180)
	p.LineTo(20, 180)
	p.Close()
	// hole CW (reversed)
	p.MoveTo(60, 60)
	p.LineTo(60, 140)
	p.LineTo(140, 140)
	p.LineTo(140, 60)
	p.Close()
	return p
}

func TestHoledPolygonFillRules(t *testing.T) {
	holed := holedSquare()
	areaHoled := 25600.0 - 6400.0

	// With the winding rule and a reversed hole the inner square is empty.
	if got := math.Abs(areaOf(t, holed)); !almostEq(got, areaHoled, 1e-9) {
		t.Fatalf("flattened holed square area = %v, want %v", got, areaHoled)
	}

	// Union with a disjoint square adds its area under both rules.
	sq := squarePath(200, 200, 260, 260)
	for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
		opts := &Options{FillRule: rule}
		if got := boolArea(t, Union, holed, sq, opts); !almostEq(got, areaHoled+3600, 1e-9) {
			t.Errorf("rule %v: union area = %v, want %v", rule, got, areaHoled+3600)
		}
		if got := boolArea(t, Intersection, holed, sq, opts); got != 0 {
			t.Errorf("rule %v: intersection area = %v, want 0", rule, got)
		}
	}

	// A shape dropped into the hole becomes an island under union.
	island := squarePath(80, 80, 120, 120)
	res, err := Boolean(Union, holed, island, &Options{FillRule: draw2d.FillRuleWinding})
	if err != nil {
		t.Fatal(err)
	}
	rings := ringsOf(t, res)
	if len(rings) != 3 {
		t.Fatalf("union with island in hole: expected 3 rings, got %d", len(rings))
	}
	pos, neg := 0, 0
	for _, r := range rings {
		if signedArea(r) > 0 {
			pos++
		} else {
			neg++
		}
	}
	if pos != 2 || neg != 1 {
		t.Errorf("expected 2 outer rings and 1 hole, got %d outer %d holes", pos, neg)
	}
	if got := math.Abs(areaOf(t, res)); !almostEq(got, areaHoled+1600, 1e-9) {
		t.Errorf("union area = %v, want %v", got, areaHoled+1600)
	}
}

func TestOrientationStableUnderInputReversal(t *testing.T) {
	a := squarePath(0, 0, 100, 100)
	b := squarePath(20, 20, 40, 40)

	// Same shapes with every ring reversed.
	rev := func(p *draw2d.Path) *draw2d.Path {
		rings := ringsOf(t, p)
		out := new(draw2d.Path)
		for _, ring := range rings {
			out.MoveTo(ring[0].x, ring[0].y)
			for i := len(ring) - 1; i >= 1; i-- {
				out.LineTo(ring[i].x, ring[i].y)
			}
			out.Close()
		}
		return out
	}
	for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
		opts := &Options{FillRule: rule}
		res1, err := Boolean(Difference, a, b, opts)
		if err != nil {
			t.Fatal(err)
		}
		res2, err := Boolean(Difference, rev(a), rev(b), opts)
		if err != nil {
			t.Fatal(err)
		}
		r1, r2 := ringsOf(t, res1), ringsOf(t, res2)
		if len(r1) != len(r2) {
			t.Fatalf("rule %v: ring count differs after reversal: %d vs %d", rule, len(r1), len(r2))
		}
		for i := range r1 {
			if math.Signbit(signedArea(r1[i])) != math.Signbit(signedArea(r2[i])) {
				t.Errorf("rule %v: ring %d orientation flipped after input reversal", rule, i)
			}
		}
	}
}

// --- degenerate and invalid input ------------------------------------------

func TestDegenerateInputs(t *testing.T) {
	empty := new(draw2d.Path)
	sq := squarePath(0, 0, 10, 10)

	if got := boolArea(t, Union, empty, sq, nil); got != 100 {
		t.Errorf("union with empty path = %v, want 100", got)
	}
	res, err := Boolean(Intersection, empty, sq, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsEmpty() {
		t.Error("intersection with empty path should be empty")
	}
	if got := boolArea(t, Difference, sq, empty, nil); got != 100 {
		t.Errorf("difference with empty path = %v, want 100", got)
	}
	res, err = Boolean(Union, empty, empty, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsEmpty() {
		t.Error("union of two empty paths should be empty")
	}

	// An open stroke (no area) behaves as an empty region.
	line := new(draw2d.Path)
	line.MoveTo(0, 0)
	line.LineTo(50, 50)
	if got := boolArea(t, Union, line, sq, nil); got != 100 {
		t.Errorf("union with open line = %v, want 100", got)
	}

	// A collinear, zero-area ring is discarded.
	flat := new(draw2d.Path)
	flat.MoveTo(0, 0)
	flat.LineTo(10, 0)
	flat.LineTo(5, 0)
	flat.Close()
	if got := boolArea(t, Union, flat, sq, nil); got != 100 {
		t.Errorf("union with zero-area ring = %v, want 100", got)
	}
}

func TestNonFiniteCoordinates(t *testing.T) {
	sq := squarePath(0, 0, 10, 10)
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		p := new(draw2d.Path)
		p.MoveTo(0, 0)
		p.LineTo(bad, 10)
		p.LineTo(10, 10)
		p.Close()
		if _, err := Boolean(Union, p, sq, nil); !errors.Is(err, ErrNonFinite) {
			t.Errorf("expected ErrNonFinite, got %v", err)
		}
		if _, err := Boolean(Union, sq, p, nil); !errors.Is(err, ErrNonFinite) {
			t.Errorf("expected ErrNonFinite for second operand, got %v", err)
		}
	}
}

func TestHugeCoordinates(t *testing.T) {
	// Squares of side 1e11 near 1e12: representable and well conditioned.
	a := squarePath(1e12, 1e12, 1e12+1e11, 1e12+1e11)
	b := squarePath(1e12+5e10, 1e12, 1e12+1.5e11, 1e12+1e11)
	got := boolArea(t, Union, a, b, nil)
	want := 1.5e22
	if !almostEq(got, want, 1e-6) {
		t.Errorf("union area at 1e12 scale = %v, want %v", got, want)
	}

	// Beyond MaxCoordinate the operation is rejected explicitly.
	big := squarePath(0, 0, 2e15, 10)
	if _, err := Boolean(Union, big, a, nil); !errors.Is(err, ErrCoordinateRange) {
		t.Errorf("expected ErrCoordinateRange, got %v", err)
	}
}

func TestComplexityLimits(t *testing.T) {
	a := circlePath(100, 100, 50)
	b := circlePath(150, 100, 50)

	if _, err := Boolean(Union, a, b, &Options{MaxSegments: 4}); !errors.Is(err, ErrTooComplex) {
		t.Errorf("expected ErrTooComplex for MaxSegments=4, got %v", err)
	}
	if _, err := Boolean(Union, a, b, &Options{MaxAtomicSegments: 2}); !errors.Is(err, ErrTooComplex) {
		t.Errorf("expected ErrTooComplex for MaxAtomicSegments=2, got %v", err)
	}
	// A generous limit succeeds.
	if _, err := Boolean(Union, a, b, &Options{MaxSegments: 10000}); err != nil {
		t.Errorf("unexpected error with generous limit: %v", err)
	}
}

// --- immutability and serializability --------------------------------------

func snapshot(p *draw2d.Path) *draw2d.Path { return p.Copy() }

func samePath(a, b *draw2d.Path) bool {
	return reflect.DeepEqual(a.Components, b.Components) && reflect.DeepEqual(a.Points, b.Points)
}

func TestInputsUnmodified(t *testing.T) {
	a := circlePath(80, 100, 40)
	b := holedSquare()
	snapA, snapB := snapshot(a), snapshot(b)
	for _, op := range []Op{Union, Intersection, Difference, XOR} {
		if _, err := Boolean(op, a, b, nil); err != nil {
			t.Fatal(err)
		}
	}
	if !samePath(a, snapA) || !samePath(b, snapB) {
		t.Error("Boolean modified an input path")
	}
}

func TestResultSerializable(t *testing.T) {
	a := circlePath(80, 100, 40)
	b := circlePath(120, 100, 40)
	res, err := Boolean(Union, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back draw2d.Path
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(res.Components, back.Components) || !reflect.DeepEqual(res.Points, back.Points) {
		t.Error("JSON round-trip changed the result path")
	}
}

// --- Bézier operands -------------------------------------------------------

// blob returns a closed four-cubic approximation of a rounded shape.
func blobPath(cx, cy, r float64) *draw2d.Path {
	const k = 0.5522847498 // circle cubic control constant
	p := new(draw2d.Path)
	p.MoveTo(cx+r, cy)
	p.CubicCurveTo(cx+r, cy-k*r, cx+k*r, cy-r, cx, cy-r)
	p.CubicCurveTo(cx-k*r, cy-r, cx-r, cy-k*r, cx-r, cy)
	p.CubicCurveTo(cx-r, cy+k*r, cx-k*r, cy+r, cx, cy+r)
	p.CubicCurveTo(cx+k*r, cy+r, cx+r, cy+k*r, cx+r, cy)
	p.Close()
	return p
}

func quadBlobPath(cx, cy, r float64) *draw2d.Path {
	p := new(draw2d.Path)
	p.MoveTo(cx+r, cy)
	p.QuadCurveTo(cx+r, cy-r, cx, cy-r)
	p.QuadCurveTo(cx-r, cy-r, cx-r, cy)
	p.QuadCurveTo(cx-r, cy+r, cx, cy+r)
	p.QuadCurveTo(cx+r, cy+r, cx+r, cy)
	p.Close()
	return p
}

func TestOverlappingBeziersAreaIdentity(t *testing.T) {
	a := blobPath(80, 100, 50)
	b := quadBlobPath(130, 100, 45)

	areaA := math.Abs(areaOf(t, a))
	areaB := math.Abs(areaOf(t, b))
	union := boolArea(t, Union, a, b, nil)
	inter := boolArea(t, Intersection, a, b, nil)
	diff := boolArea(t, Difference, a, b, nil)

	if !almostEq(union+inter, areaA+areaB, 0.005) {
		t.Errorf("area identity violated: union(%v)+intersection(%v) != a(%v)+b(%v)",
			union, inter, areaA, areaB)
	}
	if !almostEq(diff, areaA-inter, 0.005) {
		t.Errorf("difference %v != areaA %v - intersection %v", diff, areaA, inter)
	}
	if inter <= 0 || inter >= areaB {
		t.Errorf("intersection %v should be between 0 and areaB %v", inter, areaB)
	}
}
