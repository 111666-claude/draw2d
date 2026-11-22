// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
	"golang.org/x/image/draw"
)

// newSvgTestSprite returns an opaque 16x16 test image.
func newSvgTestSprite() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 0x80, 0xff})
		}
	}
	return img
}

// TestDrawImageRectXml checks that DrawImageRect emits a nested svg
// viewport whose viewBox selects the source region of the image.
func TestDrawImageRectXml(t *testing.T) {
	sprite := newSvgTestSprite()
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	err := gc.DrawImageRect(sprite, image.Rect(4, 8, 8, 12), image.Rect(20, 30, 28, 38), draw2d.FilterNearest, draw.Over)
	if err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g>
    <svg x="20" y="30" width="8" height="8" viewBox="4 8 4 4" preserveAspectRatio="none">
      <image width="16" height="16" href="` + imageToSvgHref(sprite) + `"></image>
    </svg>
  </g>
</svg>`

	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != expectedOut {
		t.Errorf("svg output is not as expected\n"+
			"got:\n%s\n\n"+
			"want:\n%s\n",
			string(out),
			expectedOut,
		)
	}
}

// TestDrawImageRectXmlTransform checks that the current transformation is
// applied to the group wrapping the viewport, and that a source image with
// non-zero bounds is positioned correctly inside the viewBox.
func TestDrawImageRectXmlTransform(t *testing.T) {
	sprite := image.NewRGBA(image.Rect(5, 7, 13, 15))
	for y := 7; y < 15; y++ {
		for x := 5; x < 13; x++ {
			sprite.SetRGBA(x, y, color.RGBA{0x10, 0x20, 0x30, 0xff})
		}
	}
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.Translate(10, 5)

	err := gc.DrawImageRect(sprite, image.Rect(6, 8, 9, 12), image.Rect(0, 0, 6, 8), draw2d.FilterLinear, draw.Over)
	if err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g transform="translate(10,5)">
    <svg width="6" height="8" viewBox="6 8 3 4" preserveAspectRatio="none">
      <image x="5" y="7" width="8" height="8" href="` + imageToSvgHref(sprite) + `"></image>
    </svg>
  </g>
</svg>`

	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != expectedOut {
		t.Errorf("svg output is not as expected\n"+
			"got:\n%s\n\n"+
			"want:\n%s\n",
			string(out),
			expectedOut,
		)
	}
}

// TestDrawImageRectErrors checks that invalid parameters report errors and
// emit no svg elements.
func TestDrawImageRectErrors(t *testing.T) {
	sprite := newSvgTestSprite()
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
			svg := NewSvg()
			gc := NewGraphicContext(svg)
			err := gc.DrawImageRect(tt.src, tt.srcRect, tt.dstRect, draw2d.FilterNearest, draw.Over)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if len(svg.Groups) != 0 {
				t.Errorf("expected no groups after failed draw, got %d", len(svg.Groups))
			}
		})
	}
}

// TestDrawImageRectNoOp checks that a zero-sized destination draws nothing
// without failing.
func TestDrawImageRectNoOp(t *testing.T) {
	sprite := newSvgTestSprite()
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	err := gc.DrawImageRect(sprite, image.Rect(0, 0, 4, 4), image.Rect(4, 4, 4, 8), draw2d.FilterNearest, draw.Over)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svg.Groups) != 0 {
		t.Errorf("expected no groups for no-op draw, got %d", len(svg.Groups))
	}
}

// TestDrawImageXmlUnchanged checks that the legacy DrawImage output is
// unchanged.
func TestDrawImageXmlUnchanged(t *testing.T) {
	sprite := newSvgTestSprite()
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.DrawImage(sprite)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g>
    <image width="16" height="16" href="` + imageToSvgHref(sprite) + `"></image>
  </g>
</svg>`

	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != expectedOut {
		t.Errorf("svg output is not as expected\n"+
			"got:\n%s\n\n"+
			"want:\n%s\n",
			string(out),
			expectedOut,
		)
	}
}
