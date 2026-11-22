// Copyright 2026 The draw2d Authors. All rights reserved.
// FillRule invariants and fixed-seed robustness tests for the raster image
// graphic context. The references are the geometric definitions of the two
// fill rules, not implementation details.

package draw2dimg

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/llgcode/draw2d"
)

var (
	fillRuleBg   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	fillRuleFill = color.RGBA{0x00, 0x00, 0x00, 0xff}
)

// rectPath traces a rectangle; dir selects the winding orientation.
func rectPath(p *draw2d.Path, x0, y0, x1, y1 float64, ccw bool) {
	if !ccw {
		p.MoveTo(x0, y0)
		p.LineTo(x1, y0)
		p.LineTo(x1, y1)
		p.LineTo(x0, y1)
	} else {
		p.MoveTo(x0, y0)
		p.LineTo(x0, y1)
		p.LineTo(x1, y1)
		p.LineTo(x1, y0)
	}
	p.Close()
}

func fillScene(t *testing.T, rule draw2d.FillRule, innerCCW bool) *image.RGBA {
	t.Helper()
	dest := image.NewRGBA(image.Rect(0, 0, 100, 100))
	gc := NewGraphicContext(dest)
	gc.SetFillColor(fillRuleBg)
	gc.Clear()
	gc.SetFillColor(fillRuleFill)
	gc.SetFillRule(rule)
	p := new(draw2d.Path)
	rectPath(p, 10, 10, 90, 90, false) // outer, clockwise
	rectPath(p, 30, 30, 70, 70, innerCCW)
	gc.Fill(p)
	return dest
}

func pixelIs(img *image.RGBA, x, y int, c color.RGBA) bool {
	r, g, b, a := img.At(x, y).RGBA()
	cr, cg, cb, ca := c.RGBA()
	return r == cr && g == cg && b == cb && a == ca
}

// TestFillRuleInvariants checks the defining difference between the two
// fill rules on nested rectangles, plus the orientation invariance of
// EvenOdd and the orientation sensitivity of Winding.
func TestFillRuleInvariants(t *testing.T) {
	// Same-orientation nested rectangles:
	// - EvenOdd: the inner rectangle is crossed twice -> hole.
	// - Winding: winding number 2 -> filled.
	evenOdd := fillScene(t, draw2d.FillRuleEvenOdd, false)
	winding := fillScene(t, draw2d.FillRuleWinding, false)
	if !pixelIs(evenOdd, 50, 50, fillRuleBg) {
		t.Fatal("EvenOdd: center of nested same-orientation rectangles must be a hole")
	}
	if !pixelIs(winding, 50, 50, fillRuleFill) {
		t.Fatal("Winding: center of nested same-orientation rectangles must be filled")
	}
	// Both rules fill the ring between the rectangles.
	for name, img := range map[string]*image.RGBA{"EvenOdd": evenOdd, "Winding": winding} {
		if !pixelIs(img, 20, 20, fillRuleFill) {
			t.Fatalf("%s: ring between the rectangles must be filled", name)
		}
		if !pixelIs(img, 5, 5, fillRuleBg) {
			t.Fatalf("%s: outside the outer rectangle must stay empty", name)
		}
	}

	// Opposite-orientation inner rectangle: the winding numbers cancel, so
	// both rules leave a hole. EvenOdd does not depend on orientation.
	windingCCW := fillScene(t, draw2d.FillRuleWinding, true)
	if !pixelIs(windingCCW, 50, 50, fillRuleBg) {
		t.Fatal("Winding: center of nested opposite-orientation rectangles must be a hole")
	}
	evenOddCCW := fillScene(t, draw2d.FillRuleEvenOdd, true)
	if !pixelIs(evenOddCCW, 50, 50, fillRuleBg) {
		t.Fatal("EvenOdd must not depend on path orientation")
	}
}

// TestFillRuleDeterministic: rendering the same scene twice must produce
// identical images for both fill rules.
func TestFillRuleDeterministic(t *testing.T) {
	for _, rule := range []draw2d.FillRule{draw2d.FillRuleEvenOdd, draw2d.FillRuleWinding} {
		a := fillScene(t, rule, false)
		b := fillScene(t, rule, false)
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				if a.At(x, y) != b.At(x, y) {
					t.Fatalf("rule %v: non-deterministic pixel at (%d,%d)", rule, x, y)
				}
			}
		}
	}
}

// TestFillStrokeHostilePaths pushes fixed-seed hostile paths (NaN/Inf, huge
// coordinates, degenerate curves, negative line widths, bad dashes) through
// Fill and Stroke with both fill rules: no panic, bounded time.
func TestFillStrokeHostilePaths(t *testing.T) {
	r := rand.New(rand.NewSource(660))
	pool := []float64{0, 1, -1, 10, 50, 99, math.NaN(), math.Inf(1), math.Inf(-1), 1e308, -1e308, 1e-300}
	v := func() float64 { return pool[r.Intn(len(pool))] }
	for sample := 0; sample < 100; sample++ {
		p := new(draw2d.Path)
		for i := 0; i < 1+r.Intn(12); i++ {
			switch r.Intn(6) {
			case 0:
				p.MoveTo(v(), v())
			case 1:
				p.LineTo(v(), v())
			case 2:
				p.QuadCurveTo(v(), v(), v(), v())
			case 3:
				p.CubicCurveTo(v(), v(), v(), v(), v(), v())
			case 4:
				p.ArcTo(v(), v(), v(), v(), v(), v())
			case 5:
				p.Close()
			}
		}
		rule := draw2d.FillRuleEvenOdd
		if sample%2 == 1 {
			rule = draw2d.FillRuleWinding
		}
		width := v()
		done := make(chan any, 1)
		go func() {
			defer func() { done <- recover() }()
			dest := image.NewRGBA(image.Rect(0, 0, 64, 64))
			gc := NewGraphicContext(dest)
			gc.SetFillRule(rule)
			gc.SetLineWidth(width)
			gc.Fill(p)
			gc.Stroke(p)
			gc.FillStroke(p)
		}()
		select {
		case rec := <-done:
			if rec != nil {
				t.Fatalf("sample %d: Fill/Stroke panicked: %v", sample, rec)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("sample %d: Fill/Stroke did not finish in time", sample)
		}
	}
}

// TestStrokeBadDashPattern is a regression sample at the graphic context
// level: a zero dash pattern used to hang Stroke forever.
func TestStrokeBadDashPattern(t *testing.T) {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		dest := image.NewRGBA(image.Rect(0, 0, 64, 64))
		gc := NewGraphicContext(dest)
		gc.SetLineDash([]float64{0}, 0) // invalid: zero length
		p := new(draw2d.Path)
		p.MoveTo(1, 1)
		p.LineTo(60, 60)
		gc.Stroke(p)
	}()
	select {
	case rec := <-done:
		if rec != nil {
			t.Fatalf("Stroke with a zero dash pattern panicked: %v", rec)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stroke with a zero dash pattern did not finish in time")
	}
}
