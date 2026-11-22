// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 23/11/2026 by draw2d contributors

package draw2d_test

import (
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dkit"
)

// flattenBounds flattens p with draw2dbase (the same code path used by the
// backends) and returns the axis-aligned box of the produced vertices.
func flattenBounds(t *testing.T, p *draw2d.Path, scale float64) (minX, minY, maxX, maxY float64) {
	t.Helper()
	var seg draw2dbase.SegmentedPath
	draw2dbase.Flatten(p, &seg, scale)
	if len(seg.Points) == 0 {
		t.Fatal("flattening produced no points")
	}
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for i := 0; i+1 < len(seg.Points); i += 2 {
		x, y := seg.Points[i], seg.Points[i+1]
		minX = math.Min(minX, x)
		minY = math.Min(minY, y)
		maxX = math.Max(maxX, x)
		maxY = math.Max(maxY, y)
	}
	return
}

// checkContainsFlattened verifies that the analytic box contains every vertex
// produced by the flattener, and that it is not much larger than the polygon
// bound (the flattener only approximates the curve from inside).
func checkContainsFlattened(t *testing.T, name string, p *draw2d.Path, scale float64) {
	t.Helper()
	ax0, ay0, ax1, ay1 := p.Bounds()
	fx0, fy0, fx1, fy1 := flattenBounds(t, p, scale)

	eps := 1e-9
	if fx0 < ax0-eps || fy0 < ay0-eps || fx1 > ax1+eps || fy1 > ay1+eps {
		t.Errorf("%s: flattened box (%v,%v,%v,%v) is not contained in analytic box (%v,%v,%v,%v)",
			name, fx0, fy0, fx1, fy1, ax0, ay0, ax1, ay1)
	}

	// The gap is bounded by the flattening error; with a fine scale it must
	// be small relative to the size of the figure.
	size := math.Max(math.Max(ax1-ax0, ay1-ay0), 1)
	tol := 1e-3 * size * scale
	for _, d := range []struct {
		label string
		a, f  float64
	}{
		{"minX", ax0, fx0}, {"minY", ay0, fy0}, {"maxX", ax1, fx1}, {"maxY", ay1, fy1},
	} {
		if math.Abs(d.a-d.f) > tol {
			t.Errorf("%s: %s analytic %v vs flattened %v differs by more than %g", name, d.label, d.a, d.f, tol)
		}
	}
}

func TestBoundsMatchesFlattenedCurves(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(10, 10)
	p.LineTo(90, 30)
	p.QuadCurveTo(150, 120, 80, 160)
	p.CubicCurveTo(20, 200, 0, 120, 10, 10)
	p.Close()
	checkContainsFlattened(t, "mixed", p, 8)

	p2 := new(draw2d.Path)
	p2.MoveTo(0, 0)
	p2.CubicCurveTo(0, 200, 200, 200, 200, 0)
	checkContainsFlattened(t, "cubic", p2, 8)
}

func TestBoundsArcMatchesFlattened(t *testing.T) {
	p := new(draw2d.Path)
	p.ArcTo(50, 50, 40, 25, 0.4, 2.2)
	checkContainsFlattened(t, "arc", p, 8*40)
}

func TestBoundsKitRectangle(t *testing.T) {
	var p draw2d.Path
	draw2dkit.Rectangle(&p, 10, 20, 110, 70)
	x0, y0, x1, y1 := p.Bounds()
	if x0 != 10 || y0 != 20 || x1 != 110 || y1 != 70 {
		t.Errorf("Rectangle bounds = (%v,%v,%v,%v), want (10,20,110,70)", x0, y0, x1, y1)
	}
}

func TestBoundsKitRectangleNegative(t *testing.T) {
	var p draw2d.Path
	draw2dkit.Rectangle(&p, -10, -20, -110, -70)
	x0, y0, x1, y1 := p.Bounds()
	if x0 != -110 || y0 != -70 || x1 != -10 || y1 != -20 {
		t.Errorf("negative Rectangle bounds = (%v,%v,%v,%v), want (-110,-70,-10,-20)", x0, y0, x1, y1)
	}
}

func TestBoundsKitCircle(t *testing.T) {
	var p draw2d.Path
	draw2dkit.Circle(&p, 100, 100, 50)
	x0, y0, x1, y1 := p.Bounds()
	for _, d := range []struct {
		label     string
		got, want float64
	}{
		{"minX", x0, 50}, {"minY", y0, 50}, {"maxX", x1, 150}, {"maxY", y1, 150},
	} {
		if math.Abs(d.got-d.want) > 1e-9 {
			t.Errorf("Circle %s = %v, want %v", d.label, d.got, d.want)
		}
	}
}

func TestBoundsKitEllipse(t *testing.T) {
	var p draw2d.Path
	draw2dkit.Ellipse(&p, -30, 40, 70, 20)
	x0, y0, x1, y1 := p.Bounds()
	for _, d := range []struct {
		label     string
		got, want float64
	}{
		{"minX", x0, -100}, {"minY", y0, 20}, {"maxX", x1, 40}, {"maxY", y1, 60},
	} {
		if math.Abs(d.got-d.want) > 1e-9 {
			t.Errorf("Ellipse %s = %v, want %v", d.label, d.got, d.want)
		}
	}
}

func TestBoundsKitRoundedRectangle(t *testing.T) {
	var p draw2d.Path
	draw2dkit.RoundedRectangle(&p, 0, 0, 100, 60, 20, 20)
	x0, y0, x1, y1 := p.Bounds()
	// The rounded corners are quarter-ish quadratic corners that stay
	// inside the rectangle; the straight edges give the full box.
	if x0 != 0 || y0 != 0 || x1 != 100 || y1 != 60 {
		t.Errorf("RoundedRectangle bounds = (%v,%v,%v,%v), want (0,0,100,60)", x0, y0, x1, y1)
	}
}

func TestBoundsKitCircleWithMatrix(t *testing.T) {
	var p draw2d.Path
	draw2dkit.Circle(&p, 0, 0, 10)
	// Scaling by (2,3) gives an ellipse with radii 20 and 30.
	tr := draw2d.NewScaleMatrix(2, 3)
	x0, y0, x1, y1 := p.BoundsWithMatrix(tr)
	for _, d := range []struct {
		label     string
		got, want float64
	}{
		{"minX", x0, -20}, {"minY", y0, -30}, {"maxX", x1, 20}, {"maxY", y1, 30},
	} {
		if math.Abs(d.got-d.want) > 1e-9 {
			t.Errorf("scaled Circle %s = %v, want %v", d.label, d.got, d.want)
		}
	}
}

// checkContainsTransformedFlattened verifies BoundsWithMatrix against the
// flattener run through draw2dbase.Transformer, which is the exact path used
// by the backends: the path is flattened first and each vertex is transformed.
func checkContainsTransformedFlattened(t *testing.T, name string, p *draw2d.Path, tr draw2d.Matrix, scale float64) {
	t.Helper()
	ax0, ay0, ax1, ay1 := p.BoundsWithMatrix(tr)

	var seg draw2dbase.SegmentedPath
	draw2dbase.Flatten(p, draw2dbase.Transformer{Tr: tr, Flattener: &seg}, scale)
	if len(seg.Points) == 0 {
		t.Fatalf("%s: flattening produced no points", name)
	}
	fx0, fy0, fx1, fy1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := 0; i+1 < len(seg.Points); i += 2 {
		fx0 = math.Min(fx0, seg.Points[i])
		fy0 = math.Min(fy0, seg.Points[i+1])
		fx1 = math.Max(fx1, seg.Points[i])
		fy1 = math.Max(fy1, seg.Points[i+1])
	}

	eps := 1e-6
	if fx0 < ax0-eps || fy0 < ay0-eps || fx1 > ax1+eps || fy1 > ay1+eps {
		t.Errorf("%s: transformed flattened box (%v,%v,%v,%v) is not contained in analytic box (%v,%v,%v,%v)",
			name, fx0, fy0, fx1, fy1, ax0, ay0, ax1, ay1)
	}
	size := math.Max(math.Max(ax1-ax0, ay1-ay0), 1)
	tol := 1e-2 * size * scale
	for _, d := range []struct {
		label string
		a, f  float64
	}{
		{"minX", ax0, fx0}, {"minY", ay0, fy0}, {"maxX", ax1, fx1}, {"maxY", ay1, fy1},
	} {
		if math.Abs(d.a-d.f) > tol {
			t.Errorf("%s: %s analytic %v vs transformed flattened %v differs by more than %g", name, d.label, d.a, d.f, tol)
		}
	}
}

func TestBoundsTransformedMatchesFlattened(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(-40, 10)
	p.QuadCurveTo(120, 180, 40, -60)
	p.CubicCurveTo(-80, -140, 160, 40, 0, 0)
	p.LineTo(70, 90)
	p.Close()
	p.MoveTo(-30, -30)
	p.ArcTo(20, 20, 45, 15, 0.2, 2.4)

	for _, tr := range []draw2d.Matrix{
		draw2d.NewRotationMatrix(0.6),
		func() draw2d.Matrix {
			m := draw2d.NewScaleMatrix(2, -3)
			m.Translate(15, 7)
			return m
		}(),
		func() draw2d.Matrix {
			m := draw2d.NewRotationMatrix(1.2)
			m.Scale(0.5, 2.5)
			m.Translate(-40, 60)
			return m
		}(),
	} {
		checkContainsTransformedFlattened(t, "mixed", p, tr, 8*tr.GetScale())
	}
}
