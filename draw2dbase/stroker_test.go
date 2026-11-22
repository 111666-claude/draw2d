// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dbase_test

import (
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
)

// recordFlattener records every point of the stroked outline
type recordFlattener struct {
	points []float64
}

func (r *recordFlattener) MoveTo(x, y float64) { r.points = append(r.points, x, y) }
func (r *recordFlattener) LineTo(x, y float64) { r.points = append(r.points, x, y) }
func (r *recordFlattener) LineJoin()           {}
func (r *recordFlattener) Close()              {}
func (r *recordFlattener) End()                {}

// strokeOutline strokes the polyline defined by pts and returns the points
// of the generated outline.
func strokeOutline(join draw2d.LineJoin, miterLimit, halfLineWidth float64, pts ...float64) []float64 {
	rec := &recordFlattener{}
	stroker := draw2dbase.NewLineStroker(draw2d.ButtCap, join, rec)
	stroker.HalfLineWidth = halfLineWidth
	stroker.MiterLimit = miterLimit
	stroker.MoveTo(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		stroker.LineTo(pts[i], pts[i+1])
	}
	stroker.End()
	return rec.points
}

// maxJoinDistance returns the largest distance from the join point among
// outline points located in its neighborhood (radius 8*halfLineWidth*ratio).
func maxJoinDistance(points []float64, jx, jy, radius float64) float64 {
	max := 0.0
	for i := 0; i+1 < len(points); i += 2 {
		d := math.Hypot(points[i]-jx, points[i+1]-jy)
		if d < radius && d > max {
			max = d
		}
	}
	return max
}

func assertFinite(t *testing.T, points []float64) {
	t.Helper()
	for i, v := range points {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("outline point #%d is not finite: %v (all points: %v)", i, v, points)
		}
	}
}

const (
	testHalfWidth = 10.0
	joinX, joinY  = 100.0, 0.0
)

// anglePath builds a two-segment path with a join at (joinX, joinY):
// a horizontal segment followed by a segment turning left by turnAngle.
func anglePath(turnAngle float64) []float64 {
	return []float64{
		0, 0,
		joinX, joinY,
		joinX + 100*math.Cos(turnAngle), joinY + 100*math.Sin(turnAngle),
	}
}

func TestMiterJoinWithinLimit(t *testing.T) {
	// 60° turn: miter ratio = 1/cos(30°) = 1.1547
	turn := math.Pi / 3
	ratio := 1 / math.Cos(turn/2)
	outline := strokeOutline(draw2d.MiterJoin, 10, testHalfWidth, anglePath(turn)...)
	assertFinite(t, outline)
	got := maxJoinDistance(outline, joinX, joinY, 5*testHalfWidth)
	want := testHalfWidth * ratio
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("miter tip distance = %v, want %v", got, want)
	}
}

func TestMiterJoinObtuseAngle(t *testing.T) {
	// 120° turn: miter ratio = 1/cos(60°) = 2
	turn := 2 * math.Pi / 3
	ratio := 2.0
	// limit above the ratio: miter applied
	outline := strokeOutline(draw2d.MiterJoin, 10, testHalfWidth, anglePath(turn)...)
	assertFinite(t, outline)
	if got := maxJoinDistance(outline, joinX, joinY, 5*testHalfWidth); math.Abs(got-testHalfWidth*ratio) > 1e-6 {
		t.Errorf("obtuse miter tip distance = %v, want %v", got, testHalfWidth*ratio)
	}
	// limit below the ratio: bevel fallback
	outline = strokeOutline(draw2d.MiterJoin, 1.5, testHalfWidth, anglePath(turn)...)
	assertFinite(t, outline)
	if got := maxJoinDistance(outline, joinX, joinY, 5*testHalfWidth); math.Abs(got-testHalfWidth) > 1e-9 {
		t.Errorf("beveled join distance = %v, want half line width %v", got, testHalfWidth)
	}
	// limit exactly equal to the ratio: miter still applied
	outline = strokeOutline(draw2d.MiterJoin, ratio, testHalfWidth, anglePath(turn)...)
	assertFinite(t, outline)
	if got := maxJoinDistance(outline, joinX, joinY, 5*testHalfWidth); math.Abs(got-testHalfWidth*ratio) > 1e-6 {
		t.Errorf("miter at exact limit distance = %v, want %v", got, testHalfWidth*ratio)
	}
}

func TestMiterLimitBevelFallback(t *testing.T) {
	turn := math.Pi / 3
	path := anglePath(turn)
	bevel := strokeOutline(draw2d.BevelJoin, 10, testHalfWidth, path...)
	cases := map[string]float64{
		"limit one":      1,
		"limit zero":     0,
		"negative limit": -5,
	}
	for name, limit := range cases {
		outline := strokeOutline(draw2d.MiterJoin, limit, testHalfWidth, path...)
		assertFinite(t, outline)
		if len(outline) != len(bevel) {
			t.Fatalf("%s: outline has %d points, want %d", name, len(outline), len(bevel))
		}
		for i := range outline {
			if outline[i] != bevel[i] {
				t.Fatalf("%s: outline[%d] = %v, want bevel join value %v", name, i, outline[i], bevel[i])
			}
		}
	}
}

func TestMiterLimitNonFiniteFallsBackToDefault(t *testing.T) {
	turn := math.Pi / 3
	path := anglePath(turn)
	def := strokeOutline(draw2d.MiterJoin, draw2d.DefaultMiterLimit, testHalfWidth, path...)
	for name, limit := range map[string]float64{
		"NaN":          math.NaN(),
		"+Inf":         math.Inf(1),
		"-Inf":         math.Inf(-1),
		"huge (bevel)": 1e300, // huge but finite: kept, still bounded by the reversal guard
	} {
		outline := strokeOutline(draw2d.MiterJoin, limit, testHalfWidth, path...)
		assertFinite(t, outline)
		if limit == 1e300 {
			continue // finite huge limit is honored, not replaced by the default
		}
		if len(outline) != len(def) {
			t.Fatalf("%s: outline has %d points, want %d", name, len(outline), len(def))
		}
		for i := range outline {
			if outline[i] != def[i] {
				t.Fatalf("%s: outline[%d] = %v, want default limit value %v", name, i, outline[i], def[i])
			}
		}
	}
}

func TestMiterJoinNearlyReversingSegments(t *testing.T) {
	// turn angle of almost 180°: the unbounded miter must be beveled,
	// even with an enormous miter limit
	for _, limit := range []float64{10, 1e300, math.Inf(1)} {
		outline := strokeOutline(draw2d.MiterJoin, limit, testHalfWidth, 0, 0, 100, 0, 0, 1e-6)
		assertFinite(t, outline)
		if got := maxJoinDistance(outline, joinX, joinY, 4*testHalfWidth); got > 1.5*testHalfWidth {
			t.Errorf("limit %v: reversing join extends to %v, want <= %v", limit, got, 1.5*testHalfWidth)
		}
	}
}

func TestMiterJoinDegenerateInput(t *testing.T) {
	cases := map[string]struct {
		halfWidth float64
		pts       []float64
	}{
		"zero length segments": {testHalfWidth, []float64{0, 0, 100, 0, 100, 0, 100, 100}},
		"all points equal":     {testHalfWidth, []float64{5, 5, 5, 5, 5, 5}},
		"tiny line width":      {1e-300, anglePath(math.Pi / 3)},
		"zero line width":      {0, anglePath(math.Pi / 3)},
		"negative line width":  {-10, anglePath(math.Pi / 3)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			outline := strokeOutline(draw2d.MiterJoin, 10, c.halfWidth, c.pts...)
			assertFinite(t, outline)
		})
	}
}

func TestMiterLimitDoesNotAffectOtherJoins(t *testing.T) {
	path := anglePath(2 * math.Pi / 3)
	for _, join := range []draw2d.LineJoin{draw2d.BevelJoin, draw2d.RoundJoin} {
		a := strokeOutline(join, 1, testHalfWidth, path...)
		b := strokeOutline(join, 1000, testHalfWidth, path...)
		if len(a) != len(b) {
			t.Fatalf("join %v: outline length changed with miter limit", join)
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("join %v: outline[%d] changed with miter limit: %v != %v", join, i, a[i], b[i])
			}
		}
	}
}

func TestMiterJoinClosedStarStaysBounded(t *testing.T) {
	// closed 5-point star: every tip is a sharp turn of 144°
	const n = 5
	cx, cy, r1, r2 := 0.0, 0.0, 100.0, 40.0
	pts := make([]float64, 0, 4*n+2)
	for i := 0; i < 2*n; i++ {
		r := r1
		if i%2 == 1 {
			r = r2
		}
		a := math.Pi/2 + float64(i)*math.Pi/n
		pts = append(pts, cx+r*math.Cos(a), cy-r*math.Sin(a))
	}
	rec := &recordFlattener{}
	stroker := draw2dbase.NewLineStroker(draw2d.ButtCap, draw2d.MiterJoin, rec)
	stroker.HalfLineWidth = testHalfWidth
	stroker.MiterLimit = 2 // star tips have ratio ~3.24: they must be beveled
	stroker.MoveTo(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		stroker.LineTo(pts[i], pts[i+1])
	}
	stroker.Close()
	stroker.End()
	assertFinite(t, rec.points)
	// every outline point must stay within the outer radius plus twice the
	// half line width (beveled joins cannot extend further)
	for i := 0; i+1 < len(rec.points); i += 2 {
		if d := math.Hypot(rec.points[i]-cx, rec.points[i+1]-cy); d > r1+2*testHalfWidth+1e-9 {
			t.Fatalf("star outline point escapes bevel bound: distance %v > %v", d, r1+2*testHalfWidth)
		}
	}
}

func TestMiterLimitSaveRestore(t *testing.T) {
	gc := draw2dbase.NewStackGraphicContext()
	if got := gc.GetMiterLimit(); got != draw2d.DefaultMiterLimit {
		t.Fatalf("default miter limit = %v, want %v", got, draw2d.DefaultMiterLimit)
	}
	gc.SetMiterLimit(3.5)
	gc.Save()
	gc.SetMiterLimit(7)
	if got := gc.GetMiterLimit(); got != 7 {
		t.Fatalf("miter limit after set = %v, want 7", got)
	}
	gc.Restore()
	if got := gc.GetMiterLimit(); got != 3.5 {
		t.Fatalf("miter limit after restore = %v, want 3.5", got)
	}
}
