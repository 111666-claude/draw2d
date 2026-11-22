// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
	"golang.org/x/image/draw"
)

func newPdfTestSprite() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 0x80, 0xff})
		}
	}
	return img
}

// TestDrawImageRect checks that DrawImageRect draws a scaled and cropped
// image region into the pdf document.
func TestDrawImageRect(t *testing.T) {
	pdf := NewPdf("P", "mm", "A4")
	gc := NewGraphicContext(pdf)
	sprite := newPdfTestSprite()

	if err := gc.DrawImageRect(sprite, image.Rect(4, 4, 8, 8), image.Rect(10, 10, 30, 30), draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	// A source rectangle partially outside the bounds is clipped, not an
	// error.
	if err := gc.DrawImageRect(sprite, image.Rect(12, 12, 20, 20), image.Rect(50, 50, 66, 66), draw2d.FilterLinear, draw.Over); err != nil {
		t.Fatalf("DrawImageRect with clipped source failed: %v", err)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("pdf output is empty")
	}
}

// TestDrawImageRectErrors checks that invalid parameters report errors.
func TestDrawImageRectErrors(t *testing.T) {
	sprite := newPdfTestSprite()
	rect := image.Rect(0, 0, 4, 4)

	tests := []struct {
		name    string
		src     image.Image
		srcRect image.Rectangle
		dstRect image.Rectangle
		wantErr error
	}{
		{"nil image", nil, rect, rect, draw2d.ErrNilImage},
		{"empty source", sprite, image.Rectangle{}, rect, draw2d.ErrEmptySourceRect},
		{"source outside", sprite, image.Rect(100, 100, 104, 104), rect, draw2d.ErrSourceRectOutside},
		{"negative dest", sprite, rect, image.Rectangle{Min: image.Pt(10, 0), Max: image.Pt(0, 4)}, draw2d.ErrNegativeDestRect},
		{"huge scale", sprite, image.Rect(0, 0, 1, 1), image.Rect(0, 0, 100000, 4), draw2d.ErrImageScaleTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdf := NewPdf("P", "mm", "A4")
			gc := NewGraphicContext(pdf)
			err := gc.DrawImageRect(tt.src, tt.srcRect, tt.dstRect, draw2d.FilterNearest, draw.Over)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestDrawImageRectNoOp checks that a zero-sized destination draws nothing
// without failing.
func TestDrawImageRectNoOp(t *testing.T) {
	pdf := NewPdf("P", "mm", "A4")
	gc := NewGraphicContext(pdf)
	if err := gc.DrawImageRect(newPdfTestSprite(), image.Rect(0, 0, 4, 4), image.Rect(4, 4, 4, 8), draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
