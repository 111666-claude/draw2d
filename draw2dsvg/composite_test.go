// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg_test

import (
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dsvg"
)

func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	return img
}

// TestSupportedCompositeOps documents the svg backend support range.
func TestSupportedCompositeOps(t *testing.T) {
	gc := draw2dsvg.NewGraphicContext(draw2dsvg.NewSvg())
	ops := gc.SupportedCompositeOps()
	if len(ops) != 1 || ops[0] != draw2d.CompositeOpOver {
		t.Fatalf("svg supported ops = %v, want [over]", ops)
	}
}

// TestDrawImageUnsupportedOpDegrades checks that DrawImage with an
// unsupported current operation degrades to over instead of failing, and
// still emits the image element.
func TestDrawImageUnsupportedOpDegrades(t *testing.T) {
	svg := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(svg)
	gc.SetCompositeOp(draw2d.CompositeOpXor)
	gc.DrawImage(testImage())

	out, err := xml.Marshal(svg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<image") {
		t.Errorf("expected an <image> element in degraded output:\n%s", out)
	}
}

// TestDrawImageWithOp checks the explicit error entry point.
func TestDrawImageWithOp(t *testing.T) {
	svg := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(svg)

	if err := gc.DrawImageWithOp(testImage(), draw2d.CompositeOpOver); err != nil {
		t.Errorf("over should be supported, got %v", err)
	}
	for _, op := range []draw2d.CompositeOp{
		draw2d.CompositeOpSrc, draw2d.CompositeOpIn, draw2d.CompositeOpOut,
		draw2d.CompositeOpAtop, draw2d.CompositeOpXor,
	} {
		if err := gc.DrawImageWithOp(testImage(), op); !errors.Is(err, draw2d.ErrUnsupportedCompositeOp) {
			t.Errorf("op %s: got %v, want ErrUnsupportedCompositeOp", op, err)
		}
	}
	if err := gc.DrawImageWithOp(testImage(), draw2d.CompositeOp(42)); !errors.Is(err, draw2d.ErrUnsupportedCompositeOp) {
		t.Errorf("invalid op: got %v, want ErrUnsupportedCompositeOp", err)
	}
	// only the single supported draw may have produced an image element
	out, _ := xml.Marshal(svg)
	if n := strings.Count(string(out), "<image"); n != 1 {
		t.Errorf("expected exactly 1 <image> element, got %d", n)
	}
}

// TestDrawImageEmptyIgnored checks empty sources are a no-op.
func TestDrawImageEmptyIgnored(t *testing.T) {
	svg := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(svg)
	gc.DrawImage(image.NewRGBA(image.Rectangle{}))
	gc.DrawImage(nil)
	out, _ := xml.Marshal(svg)
	if strings.Contains(string(out), "<image") {
		t.Errorf("empty source should not emit an <image> element:\n%s", out)
	}
}
