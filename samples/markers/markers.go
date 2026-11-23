// Copyright 2026 The draw2d Authors. All rights reserved.

// Package markers draws a map route with arrow and start/end markers.
// It exercises the path marker support of the svg backend (native marker
// elements) and of the image backend (rasterized fallback). Backends
// without marker support (pdf, gl) ignore the markers and draw only the
// routes.
package markers

import (
	"image/color"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/samples"
)

// Main draws the sample and returns the filename. This should only be
// used during testing.
func Main(gc draw2d.GraphicContext, ext string) (string, error) {
	Draw(gc, 297, 210)
	return samples.Output("markers", ext), nil
}

// arrow returns a triangular arrow head marker pointing in +x direction,
// with its tip at the reference point.
func arrow() *draw2d.Marker {
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

// dot returns a circular start/end marker.
func dot() *draw2d.Marker {
	p := new(draw2d.Path)
	p.ArcTo(5, 5, 4, 4, 0, 7) // full circle
	p.Close()
	return &draw2d.Marker{
		Path:            p,
		ViewBox:         [4]float64{0, 0, 10, 10},
		RefX:            5,
		RefY:            5,
		Width:           8,
		Height:          8,
		OrientAuto:      true,
		ScaleWithStroke: true,
	}
}

// Draw renders a polyline route with corner arrows, a bezier route, a closed
// ring and a rotated/scaled route, each with markers.
func Draw(gc draw2d.GraphicContext, width, height float64) {
	mgc, ok := gc.(draw2d.MarkerGraphicContext)
	if !ok {
		return
	}
	blue := color.NRGBA{0x1A, 0x5C, 0xD7, 0xFF}
	green := color.NRGBA{0x1E, 0x8E, 0x3E, 0xFF}

	// polyline map route: arrow at every corner and at the destination,
	// dot at the start
	gc.SetStrokeColor(blue)
	gc.SetLineWidth(3)
	mgc.SetMarker(draw2d.MarkerStart, dot())
	mgc.SetMarker(draw2d.MarkerMid, arrow())
	mgc.SetMarker(draw2d.MarkerEnd, arrow())
	gc.MoveTo(20, 40)
	gc.LineTo(120, 40)
	gc.LineTo(120, 90)
	gc.LineTo(200, 90)
	gc.Stroke()
	mgc.ClearMarkers()

	// bezier route with an arrow at its end
	gc.SetStrokeColor(green)
	gc.SetLineWidth(2)
	mgc.SetMarker(draw2d.MarkerEnd, arrow())
	gc.MoveTo(20, 120)
	gc.CubicCurveTo(80, 100, 140, 160, 200, 130)
	gc.Stroke()
	mgc.ClearMarkers()

	// closed ring with arrows on every vertex
	gc.SetStrokeColor(color.NRGBA{0xB0, 0x30, 0x30, 0xFF})
	gc.SetLineWidth(2)
	mgc.SetMarker(draw2d.MarkerMid, arrow())
	gc.MoveTo(230, 30)
	gc.LineTo(280, 40)
	gc.LineTo(270, 80)
	gc.LineTo(225, 70)
	gc.Close()
	gc.Stroke()
	mgc.ClearMarkers()

	// rotated and scaled route: the markers follow the parent transform
	gc.Save()
	gc.Translate(60, 180)
	gc.Rotate(0.3)
	gc.Scale(1.5, 1.5)
	gc.SetStrokeColor(blue)
	gc.SetLineWidth(2)
	mgc.SetMarker(draw2d.MarkerEnd, arrow())
	gc.MoveTo(0, 0)
	gc.LineTo(60, 0)
	gc.LineTo(90, -20)
	gc.Stroke()
	gc.Restore()
}
