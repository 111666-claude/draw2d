// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

const angleTol = 1e-9

func checkVertex(t *testing.T, v MarkerVertex, x, y, angle float64) {
	t.Helper()
	if math.Abs(v.X-x) > 1e-9 || math.Abs(v.Y-y) > 1e-9 {
		t.Errorf("vertex position = (%v, %v), want (%v, %v)", v.X, v.Y, x, y)
	}
	if math.Abs(v.Angle-angle) > angleTol {
		t.Errorf("vertex angle = %v, want %v", v.Angle, angle)
	}
}

// Polyline fixture: an L shaped route, going east then south (y-down).
func TestComputeMarkerVerticesPolyline(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)

	verts := ComputeMarkerVertices(p)
	if !verts.HasStart || !verts.HasEnd {
		t.Fatal("expected start and end vertices")
	}
	checkVertex(t, verts.Start, 0, 0, 0)
	checkVertex(t, verts.End, 100, 100, math.Pi/2)
	if len(verts.Mids) != 1 {
		t.Fatalf("expected 1 mid vertex, got %d", len(verts.Mids))
	}
	// bisector of east and south is south-east
	checkVertex(t, verts.Mids[0], 100, 0, math.Pi/4)
}

// Bezier fixture: the end tangents point along the control polygon.
func TestComputeMarkerVerticesBezier(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(0, 50, 100, 50, 100, 100)

	verts := ComputeMarkerVertices(p)
	if !verts.HasStart || !verts.HasEnd {
		t.Fatal("expected start and end vertices")
	}
	// start tangent points at the first control point (0, 50): south
	if math.Abs(verts.Start.Angle-math.Pi/2) > 0.1 {
		t.Errorf("start angle = %v, want ~pi/2", verts.Start.Angle)
	}
	// end tangent comes from the second control point (100, 50): south
	if math.Abs(verts.End.Angle-math.Pi/2) > 0.1 {
		t.Errorf("end angle = %v, want ~pi/2", verts.End.Angle)
	}
	if verts.Start.X != 0 || verts.Start.Y != 0 {
		t.Errorf("start position = (%v, %v), want (0, 0)", verts.Start.X, verts.Start.Y)
	}
	if math.Abs(verts.End.X-100) > 1e-9 || math.Abs(verts.End.Y-100) > 1e-9 {
		t.Errorf("end position = (%v, %v), want (100, 100)", verts.End.X, verts.End.Y)
	}
}

// Closed ring fixture: a rectangle. Start and end markers coincide at the
// first vertex; every other corner gets a mid marker.
func TestComputeMarkerVerticesClosedRing(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(0, 100)
	p.Close()

	verts := ComputeMarkerVertices(p)
	if !verts.HasStart || !verts.HasEnd {
		t.Fatal("expected start and end vertices")
	}
	checkVertex(t, verts.Start, 0, 0, 0)
	// the closing segment goes north (y-down)
	checkVertex(t, verts.End, 0, 0, -math.Pi/2)
	if len(verts.Mids) != 3 {
		t.Fatalf("expected 3 mid vertices, got %d", len(verts.Mids))
	}
	checkVertex(t, verts.Mids[0], 100, 0, math.Pi/4)
	checkVertex(t, verts.Mids[1], 100, 100, 3*math.Pi/4)
	checkVertex(t, verts.Mids[2], 0, 100, -3*math.Pi/4)
}

// Degenerate fixtures: lone MoveTo, zero-length segments, hairpin turn.
func TestComputeMarkerVerticesDegenerate(t *testing.T) {
	// a lone MoveTo has no determinable tangent: no vertices at all
	lone := new(draw2d.Path)
	lone.MoveTo(10, 10)
	verts := ComputeMarkerVertices(lone)
	if verts.HasStart || verts.HasEnd || len(verts.Mids) != 0 {
		t.Error("lone MoveTo should produce no marker vertices")
	}

	// zero-length segments are collapsed
	zero := new(draw2d.Path)
	zero.MoveTo(0, 0)
	zero.LineTo(0, 0)
	zero.LineTo(50, 0)
	verts = ComputeMarkerVertices(zero)
	if !verts.HasStart || !verts.HasEnd {
		t.Fatal("expected start and end vertices")
	}
	checkVertex(t, verts.Start, 0, 0, 0)
	checkVertex(t, verts.End, 50, 0, 0)
	if len(verts.Mids) != 0 {
		t.Errorf("zero-length segment should not add a mid vertex, got %d", len(verts.Mids))
	}

	// 180° hairpin turn: the bisector is indeterminate, the incoming
	// direction is used as a stable fallback
	hairpin := new(draw2d.Path)
	hairpin.MoveTo(0, 0)
	hairpin.LineTo(50, 0)
	hairpin.LineTo(0, 0)
	verts = ComputeMarkerVertices(hairpin)
	if len(verts.Mids) != 1 {
		t.Fatalf("expected 1 mid vertex, got %d", len(verts.Mids))
	}
	checkVertex(t, verts.Mids[0], 50, 0, 0)
}

func TestMarkerTransform(t *testing.T) {
	arrow := &draw2d.Marker{
		ViewBox: [4]float64{0, 0, 10, 10},
		RefX:    10, RefY: 5,
		Width: 10, Height: 10,
		OrientAuto: true,
	}
	// vertex at (100, 100), tangent pointing east
	tr := MarkerTransform(arrow, MarkerVertex{X: 100, Y: 100, Angle: 0}, 2)
	// the reference point lands exactly on the vertex
	if x, y := tr.TransformPoint(10, 5); x != 100 || y != 100 {
		t.Errorf("ref point maps to (%v, %v), want (100, 100)", x, y)
	}
	// unscaled (no stroke scaling): the tail stays 10 units behind the tip
	if x, y := tr.TransformPoint(0, 5); x != 90 || y != 100 {
		t.Errorf("tail maps to (%v, %v), want (90, 100)", x, y)
	}

	// rotated 90° (south, y-down): the tail is above the tip
	tr = MarkerTransform(arrow, MarkerVertex{X: 100, Y: 100, Angle: math.Pi / 2}, 2)
	if x, y := tr.TransformPoint(0, 5); math.Abs(x-100) > 1e-9 || math.Abs(y-90) > 1e-9 {
		t.Errorf("rotated tail maps to (%v, %v), want (100, 90)", x, y)
	}

	// strokeWidth units: the artwork scales with the line width
	arrow.ScaleWithStroke = true
	tr = MarkerTransform(arrow, MarkerVertex{X: 100, Y: 100, Angle: 0}, 2)
	if x, y := tr.TransformPoint(10, 5); x != 100 || y != 100 {
		t.Errorf("ref point maps to (%v, %v), want (100, 100)", x, y)
	}
	if x, y := tr.TransformPoint(0, 5); x != 80 || y != 100 {
		t.Errorf("scaled tail maps to (%v, %v), want (80, 100)", x, y)
	}

	// fixed orientation ignores the vertex tangent
	arrow.OrientAuto = false
	arrow.OrientAngle = 90
	arrow.ScaleWithStroke = false
	tr = MarkerTransform(arrow, MarkerVertex{X: 100, Y: 100, Angle: 0}, 2)
	if x, y := tr.TransformPoint(0, 5); math.Abs(x-100) > 1e-9 || math.Abs(y-90) > 1e-9 {
		t.Errorf("fixed orient tail maps to (%v, %v), want (100, 90)", x, y)
	}
}

func TestMarkerValidate(t *testing.T) {
	validPath := func() *draw2d.Path {
		p := new(draw2d.Path)
		p.MoveTo(0, 0)
		p.LineTo(10, 5)
		p.LineTo(0, 10)
		p.Close()
		return p
	}
	valid := func() *draw2d.Marker {
		return &draw2d.Marker{Path: validPath(), ViewBox: [4]float64{0, 0, 10, 10}, Width: 10, Height: 10}
	}

	cases := []struct {
		name   string
		mutate func(m *draw2d.Marker) *draw2d.Marker
		want   error
	}{
		{"nil marker", func(m *draw2d.Marker) *draw2d.Marker { return nil }, draw2d.ErrMarkerEmpty},
		{"nil path", func(m *draw2d.Marker) *draw2d.Marker { m.Path = nil; return m }, draw2d.ErrMarkerEmpty},
		{"empty path", func(m *draw2d.Marker) *draw2d.Marker { m.Path = new(draw2d.Path); return m }, draw2d.ErrMarkerEmpty},
		{"zero width", func(m *draw2d.Marker) *draw2d.Marker { m.Width = 0; return m }, draw2d.ErrMarkerSize},
		{"negative height", func(m *draw2d.Marker) *draw2d.Marker { m.Height = -1; return m }, draw2d.ErrMarkerSize},
		{"NaN width", func(m *draw2d.Marker) *draw2d.Marker { m.Width = math.NaN(); return m }, draw2d.ErrMarkerSize},
		{"Inf height", func(m *draw2d.Marker) *draw2d.Marker { m.Height = math.Inf(1); return m }, draw2d.ErrMarkerSize},
		{"oversized", func(m *draw2d.Marker) *draw2d.Marker { m.Width = draw2d.MaxMarkerSize + 1; return m }, draw2d.ErrMarkerSize},
		{"NaN refX", func(m *draw2d.Marker) *draw2d.Marker { m.RefX = math.NaN(); return m }, draw2d.ErrMarkerSize},
		{"bad viewBox", func(m *draw2d.Marker) *draw2d.Marker { m.ViewBox = [4]float64{0, 0, -10, 10}; return m }, draw2d.ErrMarkerSize},
		{"valid", func(m *draw2d.Marker) *draw2d.Marker { return m }, nil},
		{"valid no viewBox", func(m *draw2d.Marker) *draw2d.Marker { m.ViewBox = [4]float64{}; return m }, nil},
	}
	for _, c := range cases {
		if err := c.mutate(valid()).Validate(); err != c.want {
			t.Errorf("%s: Validate() = %v, want %v", c.name, err, c.want)
		}
	}
}
