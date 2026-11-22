// Copyright 2026 The draw2d Authors. All rights reserved.

package boolop

import (
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dpdf"
	"github.com/llgcode/draw2d/draw2dsvg"
)

const rasterSize = 220

// render fills the paths into an RGBA image using the given fill rule.
func render(rule draw2d.FillRule, paths ...*draw2d.Path) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, rasterSize, rasterSize))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetFillRule(rule)
	gc.SetFillColor(color.White)
	gc.Fill(paths...)
	return img
}

func filledPx(img *image.RGBA, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r+g+b > 3*0x8000
}

// reference combines the rasterized operands pixel by pixel: an independent
// raster model of the boolean operation.
func reference(op Op, a, b *image.RGBA) *image.RGBA {
	out := image.NewRGBA(a.Bounds())
	for y := 0; y < rasterSize; y++ {
		for x := 0; x < rasterSize; x++ {
			if op.apply(filledPx(a, x, y), filledPx(b, x, y)) {
				out.Set(x, y, color.White)
			}
		}
	}
	return out
}

func countFilled(img *image.RGBA) int {
	n := 0
	for y := 0; y < rasterSize; y++ {
		for x := 0; x < rasterSize; x++ {
			if filledPx(img, x, y) {
				n++
			}
		}
	}
	return n
}

// checkRaster renders the boolean result and compares it against the
// pixel-combined reference, allowing a small tolerance along anti-aliased
// boundaries.
func checkRaster(t *testing.T, op Op, a, b *draw2d.Path, opts *Options) *draw2d.Path {
	t.Helper()
	res, err := Boolean(op, a, b, opts)
	if err != nil {
		t.Fatalf("Boolean(%v): %v", op, err)
	}
	rule := draw2d.FillRuleEvenOdd
	if opts != nil {
		rule = opts.FillRule
	}
	got := render(rule, res)
	want := reference(op, render(rule, a), render(rule, b))

	diff, filledGot, filledWant := 0, countFilled(got), countFilled(want)
	for y := 0; y < rasterSize; y++ {
		for x := 0; x < rasterSize; x++ {
			if filledPx(got, x, y) != filledPx(want, x, y) {
				diff++
			}
		}
	}
	const total = rasterSize * rasterSize
	if diff > total/50 { // 2%
		t.Errorf("%v: %d pixels differ from raster reference (%.2f%%)", op, diff, 100*float64(diff)/total)
	}
	if filledWant > 0 && !almostEq(float64(filledGot), float64(filledWant), 0.02) {
		t.Errorf("%v: filled pixel count %d, reference %d", op, filledGot, filledWant)
	}
	return res
}

func TestRasterIntersectingCircles(t *testing.T) {
	a := circlePath(85, 110, 55)
	b := circlePath(135, 110, 55)
	for _, op := range []Op{Union, Intersection, Difference, XOR} {
		checkRaster(t, op, a, b, nil)
	}
}

func TestRasterOverlappingBeziers(t *testing.T) {
	a := blobPath(85, 110, 55)
	b := quadBlobPath(135, 110, 50)
	for _, op := range []Op{Union, Intersection, Difference, XOR} {
		checkRaster(t, op, a, b, nil)
	}
}

func TestRasterHoledPolygon(t *testing.T) {
	holed := holedSquare()
	cutter := squarePath(100, 20, 210, 180)
	for _, op := range []Op{Union, Intersection, Difference, XOR} {
		checkRaster(t, op, holed, cutter, nil)
	}
	// Also under the non-zero winding rule.
	winding := &Options{FillRule: draw2d.FillRuleWinding}
	for _, op := range []Op{Union, Intersection, Difference} {
		checkRaster(t, op, holed, cutter, winding)
	}
}

// TestResultFillsSameUnderBothRules renders a boolean result with even-odd
// and with winding: the stable ring orientations must make them identical.
func TestResultFillsSameUnderBothRules(t *testing.T) {
	a := circlePath(100, 100, 60)
	b := circlePath(100, 100, 30)
	res, err := Boolean(Difference, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	eo := render(draw2d.FillRuleEvenOdd, res)
	wn := render(draw2d.FillRuleWinding, res)
	diff := 0
	for y := 0; y < rasterSize; y++ {
		for x := 0; x < rasterSize; x++ {
			if filledPx(eo, x, y) != filledPx(wn, x, y) {
				diff++
			}
		}
	}
	if diff > 0 {
		t.Errorf("result fills differently under even-odd and winding: %d pixels", diff)
	}
}

// TestResultDrawableByBackends feeds a boolean result to the image, SVG and
// PDF backends.
func TestResultDrawableByBackends(t *testing.T) {
	a := holedSquare()
	b := circlePath(100, 100, 90)
	res, err := Boolean(Difference, a, b, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Image backend.
	img := image.NewRGBA(image.Rect(0, 0, rasterSize, rasterSize))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetFillColor(color.White)
	gc.FillStroke(res)
	if countFilled(render(draw2d.FillRuleEvenOdd, res)) == 0 {
		t.Error("image backend produced no output for the boolean result")
	}

	// SVG backend.
	svg := draw2dsvg.NewSvg()
	svgGc := draw2dsvg.NewGraphicContext(svg)
	svgGc.Fill(res)
	if len(svg.Groups) == 0 {
		t.Error("svg backend produced no groups for the boolean result")
	}

	// PDF backend.
	pdf := draw2dpdf.NewPdf("P", "mm", "A4")
	pdf.AddPage()
	pdfGc := draw2dpdf.NewGraphicContext(pdf)
	pdfGc.SetFillColor(color.Black)
	pdfGc.Fill(res)
	if err := pdf.Error(); err != nil {
		t.Errorf("pdf backend error: %v", err)
	}
}
