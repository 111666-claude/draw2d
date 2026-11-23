// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"image/color"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
)

// arrowMarker is a simple triangular arrow head pointing in +x direction,
// with its tip at the reference point.
func arrowMarker() *draw2d.Marker {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(10, 5)
	p.LineTo(0, 10)
	p.Close()
	return &draw2d.Marker{
		Path:            p,
		ViewBox:         [4]float64{0, 0, 10, 10},
		RefX:            10,
		RefY:            5,
		Width:           10,
		Height:          10,
		OrientAuto:      true,
		ScaleWithStroke: true,
	}
}

func marshal(t *testing.T, svg *Svg) string {
	t.Helper()
	out, err := xml.Marshal(svg)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Polyline fixture: an L shaped route with an arrow at its end.
func TestMarkerPolylineXml(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetStrokeColor(color.RGBA{0x00, 0x00, 0xFF, 0xFF})
	gc.SetLineWidth(2)
	if err := gc.SetMarker(draw2d.MarkerEnd, arrowMarker()); err != nil {
		t.Fatal(err)
	}
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.LineTo(100, 100)
	gc.Stroke()

	out := marshal(t, svg)
	for _, want := range []string{
		`<marker id="marker-1"`,
		`viewBox="0 0 10 10"`,
		`refX="10"`,
		`refY="5"`,
		`markerWidth="10"`,
		`markerHeight="10"`,
		`markerUnits="strokeWidth"`,
		`orient="auto"`,
		`marker-end="url(#marker-1)"`,
		// the marker artwork inherits the stroke color of the route
		`<path fill="#0000FF"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
	if strings.Contains(out, "marker-start") || strings.Contains(out, "marker-mid") {
		t.Errorf("only the end marker should be attached\ngot:\n%s", out)
	}
	// exactly one marker definition
	if n := strings.Count(out, "<marker "); n != 1 {
		t.Errorf("expected 1 marker definition, got %d\ngot:\n%s", n, out)
	}
}

// The same marker reused on several paths shares a single defs entry;
// a different marker gets a fresh, unique id.
func TestMarkerDedupAndUniqueIds(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetMarker(draw2d.MarkerEnd, arrowMarker())

	gc.MoveTo(0, 0)
	gc.LineTo(50, 0)
	gc.Stroke()
	gc.MoveTo(0, 10)
	gc.LineTo(50, 10)
	gc.Stroke()

	// a different marker (other size) must not collide
	other := arrowMarker()
	other.Width, other.Height = 20, 20
	gc.SetMarker(draw2d.MarkerEnd, other)
	gc.MoveTo(0, 20)
	gc.LineTo(50, 20)
	gc.Stroke()

	out := marshal(t, svg)
	if n := strings.Count(out, "<marker "); n != 2 {
		t.Errorf("expected 2 marker definitions, got %d\ngot:\n%s", n, out)
	}
	if n := strings.Count(out, `id="marker-1"`); n != 1 {
		t.Errorf("marker-1 defined %d times, want 1", n)
	}
	if n := strings.Count(out, `id="marker-2"`); n != 1 {
		t.Errorf("marker-2 defined %d times, want 1", n)
	}
	if n := strings.Count(out, `marker-end="url(#marker-1)"`); n != 2 {
		t.Errorf("expected 2 references to marker-1, got %d\ngot:\n%s", n, out)
	}
	if n := strings.Count(out, `marker-end="url(#marker-2)"`); n != 1 {
		t.Errorf("expected 1 reference to marker-2, got %d\ngot:\n%s", n, out)
	}
}

// Closed ring fixture: start, mid and end markers on a rectangle.
func TestMarkerClosedRingXml(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetMarker(draw2d.MarkerStart, arrowMarker())
	gc.SetMarker(draw2d.MarkerMid, arrowMarker())
	gc.SetMarker(draw2d.MarkerEnd, arrowMarker())

	gc.MoveTo(0, 0)
	gc.LineTo(100, 0)
	gc.LineTo(100, 100)
	gc.LineTo(0, 100)
	gc.Close()
	gc.Stroke()

	out := marshal(t, svg)
	for _, want := range []string{
		`marker-start="url(#marker-1)"`,
		`marker-mid="url(#marker-1)"`,
		`marker-end="url(#marker-1)"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
	// identical markers at all positions share one definition
	if n := strings.Count(out, "<marker "); n != 1 {
		t.Errorf("expected 1 marker definition, got %d\ngot:\n%s", n, out)
	}
}

// Rotated/scaled fixture: the parent transform lands on the group and the
// marker references stay intact inside it.
func TestMarkerTransformXml(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetMarker(draw2d.MarkerEnd, arrowMarker())
	gc.Translate(50, 50)
	gc.Rotate(0.5)
	gc.Scale(2, 2)
	gc.MoveTo(0, 0)
	gc.LineTo(40, 0)
	gc.Stroke()

	out := marshal(t, svg)
	if !strings.Contains(out, "transform=\"matrix(") {
		t.Errorf("expected a transformed group\ngot:\n%s", out)
	}
	if !strings.Contains(out, `marker-end="url(#marker-1)"`) {
		t.Errorf("marker reference lost under transform\ngot:\n%s", out)
	}
}

// Bezier fixture with a custom fill and userSpaceOnUse units.
func TestMarkerBezierCustomStyleXml(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	m := arrowMarker()
	m.Fill = color.RGBA{0xFF, 0x00, 0x00, 0xFF}
	m.ScaleWithStroke = false
	gc.SetMarker(draw2d.MarkerMid, m)
	gc.MoveTo(0, 0)
	gc.CubicCurveTo(0, 50, 100, 50, 100, 100)
	gc.Stroke()

	out := marshal(t, svg)
	for _, want := range []string{
		`markerUnits="userSpaceOnUse"`,
		`marker-mid="url(#marker-1)"`,
		`<path fill="#FF0000"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestMarkerSetMarkerErrors(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	if err := gc.SetMarker(draw2d.MarkerEnd, &draw2d.Marker{}); err != draw2d.ErrMarkerEmpty {
		t.Errorf("empty marker: err = %v, want %v", err, draw2d.ErrMarkerEmpty)
	}
	oversized := arrowMarker()
	oversized.Width = draw2d.MaxMarkerSize * 2
	if err := gc.SetMarker(draw2d.MarkerEnd, oversized); err != draw2d.ErrMarkerSize {
		t.Errorf("oversized marker: err = %v, want %v", err, draw2d.ErrMarkerSize)
	}
	if err := gc.SetMarker(draw2d.MarkerStart|draw2d.MarkerEnd, arrowMarker()); err != draw2d.ErrMarkerPosition {
		t.Errorf("combined position: err = %v, want %v", err, draw2d.ErrMarkerPosition)
	}

	// failed SetMarker calls store nothing: no markers in the output
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.Stroke()
	if out := marshal(t, svg); strings.Contains(out, "<marker") {
		t.Errorf("invalid markers must not be stored\ngot:\n%s", out)
	}

	// setting and clearing
	if err := gc.SetMarker(draw2d.MarkerEnd, arrowMarker()); err != nil {
		t.Fatal(err)
	}
	gc.ClearMarkers()
	gc.Stroke()
	if out := marshal(t, svg); strings.Contains(out, "<marker") {
		t.Errorf("cleared markers must not be drawn\ngot:\n%s", out)
	}
}

// A marker whose path is the decorated path itself is a circular reference:
// it is stably omitted from the output instead of recursing.
func TestMarkerCircularReference(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(50, 0)
	m := arrowMarker()
	m.Path = p
	if err := gc.SetMarker(draw2d.MarkerEnd, m); err != nil {
		t.Fatal(err)
	}
	gc.Stroke(p)

	if out := marshal(t, svg); strings.Contains(out, "<marker") {
		t.Errorf("circular marker must be omitted\ngot:\n%s", out)
	}
}

// Markers participate in Save/Restore like the other context state.
func TestMarkerSaveRestore(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetMarker(draw2d.MarkerEnd, arrowMarker())
	gc.Save()
	gc.ClearMarkers()
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.Stroke()
	gc.Restore()
	gc.MoveTo(0, 10)
	gc.LineTo(10, 10)
	gc.Stroke()

	out := marshal(t, svg)
	if n := strings.Count(out, `marker-end="url(#marker-1)"`); n != 1 {
		t.Errorf("expected the restored marker on exactly 1 path, got %d\ngot:\n%s", n, out)
	}
}
