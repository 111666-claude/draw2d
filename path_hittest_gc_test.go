// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 2026

package draw2d_test

import (
	"image"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
)

// TestHitTestGraphicContextEntry exercises the convenience entry point that
// answers hit tests against the current path, matrix and fill rule of a
// concrete graphic context. Every backend embeds draw2dbase.
// StackGraphicContext, so the promoted method is available on each GC.
func TestHitTestGraphicContextEntry(t *testing.T) {
	const size = 140
	tr := draw2d.NewTranslationMatrix(8, 12)
	tr.Compose(draw2d.NewScaleMatrix(1.1, 1.1))
	badge := badgeWithHole()
	poly := refFlatten(badge)

	// Paint the reference image with the image backend.
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetMatrixTransform(tr)
	gc.SetFillRule(draw2d.FillRuleEvenOdd)
	gc.Fill(badge.Copy())

	// A second context carries the same path as its *current* path, so the
	// hit test goes through the promoted StackGraphicContext methods.
	probe := draw2dimg.NewGraphicContext(image.NewRGBA(image.Rect(0, 0, size, size)))
	probe.SetMatrixTransform(tr)
	probe.SetFillRule(draw2d.FillRuleEvenOdd)
	probe.BeginPath()
	probe.Current.Path = badge.Copy()

	for px := 0; px < size; px++ {
		for py := 0; py < size; py++ {
			alpha := img.Pix[py*img.Stride+px*4+3]
			if alpha != 0 && alpha != 255 {
				continue // partial coverage boundary pixel
			}
			x, y := float64(px)+0.5, float64(py)+0.5
			ux, uy := inversePoint(tr, x, y)
			if d := poly.distanceToPolygon(ux, uy); d < 1.0 {
				continue
			}
			got := probe.PathContains(x, y)
			want := alpha == 255
			if got != want {
				t.Fatalf("PathContains pixel (%d,%d) alpha=%d: got %v want %v", px, py, alpha, got, want)
			}
		}
	}

	// The explicit-rule variant observes a rule switch while the context's
	// current fill rule stays untouched. Probe the lens center in device
	// space through the context matrix.
	circles := intersectingCircles()
	probe.BeginPath()
	probe.Current.Path = circles.Copy()
	lensX, lensY := tr.TransformPoint(50, 50)
	if probe.PathContains(lensX, lensY) {
		t.Error("current even-odd rule must leave the shared lens outside")
	}
	if !probe.PathContainsWithRule(lensX, lensY, draw2d.FillRuleWinding) {
		t.Error("explicit non-zero rule must fill the shared lens")
	}
	if probe.Current.FillRule != draw2d.FillRuleEvenOdd {
		t.Error("PathContainsWithRule must not change the context fill rule")
	}
}
