// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
)

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

func isBlack(img *image.RGBA, x, y int) bool {
	r, g, b, a := img.At(x, y).RGBA()
	return a > 0 && r>>8 < 128 && g>>8 < 128 && b>>8 < 128
}

// Raster fallback: an arrow head is drawn at the end vertex of a horizontal
// route, pointing east.
func TestMarkerRasterFallback(t *testing.T) {
	newCtx := func() (*GraphicContext, *image.RGBA) {
		dest := image.NewRGBA(image.Rect(0, 0, 200, 200))
		gc := NewGraphicContext(dest)
		gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xFF})
		gc.SetLineWidth(2)
		return gc, dest
	}

	// route without marker
	gc, plain := newCtx()
	gc.MoveTo(20, 100)
	gc.LineTo(180, 100)
	gc.Stroke()

	// same route with an arrow end marker
	gc, marked := newCtx()
	if err := gc.SetMarker(draw2d.MarkerEnd, arrowMarker()); err != nil {
		t.Fatal(err)
	}
	gc.MoveTo(20, 100)
	gc.LineTo(180, 100)
	gc.Stroke()

	// the arrow head scales with the 2px line width: it spans 20 units
	// behind the tip and 20 units in height, covering pixels above/below
	// the bare 2px wide line
	checked := 0
	for _, pt := range [][2]int{{174, 98}, {174, 102}, {164, 94}, {164, 106}} {
		x, y := pt[0], pt[1]
		if isBlack(plain, x, y) {
			t.Errorf("pixel (%d, %d) unexpectedly covered without marker", x, y)
		}
		if !isBlack(marked, x, y) {
			t.Errorf("pixel (%d, %d) should be covered by the arrow head", x, y)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no pixels checked")
	}
	// the tip itself is on the vertex
	if !isBlack(marked, 179, 100) {
		t.Error("arrow tip should be at the end vertex (180, 100)")
	}
}

// The marker inherits the stroke color when it has no fill of its own, and
// the parent transform applies to marker vertices.
func TestMarkerRasterFallbackTransform(t *testing.T) {
	dest := image.NewRGBA(image.Rect(0, 0, 200, 200))
	gc := NewGraphicContext(dest)
	gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xFF})
	gc.SetLineWidth(2)
	gc.SetMarker(draw2d.MarkerEnd, arrowMarker())
	gc.Translate(0, 50)
	gc.MoveTo(20, 50)
	gc.LineTo(180, 50)
	gc.Stroke()

	// the route is translated from y=50 to y=100, so the arrow head covers
	// (174, 98) and nothing is drawn at the untransformed position
	if !isBlack(dest, 174, 98) {
		t.Error("arrow head should follow the translated route")
	}
	if isBlack(dest, 174, 48) {
		t.Error("arrow head should not be drawn at the untransformed position")
	}
}

// DrawMarkers reports a stable error for circular references instead of
// recursing.
func TestDrawMarkersCircular(t *testing.T) {
	dest := image.NewRGBA(image.Rect(0, 0, 100, 100))
	gc := NewGraphicContext(dest)
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.LineTo(50, 0)
	m := arrowMarker()
	m.Path = p
	err := draw2dbase.DrawMarkers(gc, map[draw2d.MarkerPosition]*draw2d.Marker{draw2d.MarkerEnd: m}, color.Black, 1, p)
	if err != draw2d.ErrMarkerCircular {
		t.Errorf("err = %v, want %v", err, draw2d.ErrMarkerCircular)
	}
}

// Invalid markers are rejected by SetMarker before any drawing happens.
func TestMarkerSetMarkerError(t *testing.T) {
	dest := image.NewRGBA(image.Rect(0, 0, 100, 100))
	gc := NewGraphicContext(dest)
	if err := gc.SetMarker(draw2d.MarkerEnd, &draw2d.Marker{}); err != draw2d.ErrMarkerEmpty {
		t.Errorf("err = %v, want %v", err, draw2d.ErrMarkerEmpty)
	}
}
