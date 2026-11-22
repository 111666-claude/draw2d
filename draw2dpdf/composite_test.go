// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dpdf"
)

func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	return img
}

// TestSupportedCompositeOps documents the pdf backend support range.
func TestSupportedCompositeOps(t *testing.T) {
	gc := draw2dpdf.NewGraphicContext(draw2dpdf.NewPdf("L", "mm", "A4"))
	ops := gc.SupportedCompositeOps()
	if len(ops) != 1 || ops[0] != draw2d.CompositeOpOver {
		t.Fatalf("pdf supported ops = %v, want [over]", ops)
	}
}

// TestDrawImageUnsupportedOpDegrades checks that DrawImage with an
// unsupported current operation degrades to over and still produces a valid
// document containing the image.
func TestDrawImageUnsupportedOpDegrades(t *testing.T) {
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)
	gc.SetCompositeOp(draw2d.CompositeOpXor)
	gc.DrawImage(testImage())

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output failed: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Subtype /Image")) {
		t.Errorf("expected an image object in degraded pdf output")
	}
}

// TestDrawImageWithOp checks the explicit error entry point.
func TestDrawImageWithOp(t *testing.T) {
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)

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
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output failed: %v", err)
	}
}

// TestDrawImageEmptyIgnored checks empty sources are a no-op.
func TestDrawImageEmptyIgnored(t *testing.T) {
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)
	gc.DrawImage(image.NewRGBA(image.Rectangle{}))
	gc.DrawImage(nil)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output failed: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("/Subtype /Image")) {
		t.Errorf("empty source should not emit an image object")
	}
}
