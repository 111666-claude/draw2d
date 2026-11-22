// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"errors"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"golang.org/x/image/draw"
)

// newCheckerSprite returns a 16x16 sprite sheet of 4x4 cells of 4x4 pixels
// each. The pixel at (x, y) has color R=x*16, G=y*16, B=0x80, A=0xff, so
// every pixel is uniquely identified by its color.
func newCheckerSprite() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 0x80, 0xff})
		}
	}
	return img
}

func spriteColor(x, y int) color.RGBA {
	return color.RGBA{uint8(x * 16), uint8(y * 16), 0x80, 0xff}
}

func assertPixel(t *testing.T, img *image.RGBA, x, y int, want color.RGBA) {
	t.Helper()
	got := img.RGBAAt(x, y)
	if got != want {
		t.Errorf("pixel (%d, %d) = %v, want %v", x, y, got, want)
	}
}

func assertTransparent(t *testing.T, img *image.RGBA, x, y int) {
	t.Helper()
	assertPixel(t, img, x, y, color.RGBA{})
}

// TestDrawImageRectSpriteCell draws a single cell of a checkerboard sprite
// sheet scaled by 2 and checks every destination pixel.
func TestDrawImageRectSpriteCell(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := NewGraphicContext(dest)

	// Cell (1, 2): pixels (4..8, 8..12), drawn at 2x into (10,20)-(18,28).
	srcRect := image.Rect(4, 8, 8, 12)
	dstRect := image.Rect(10, 20, 18, 28)
	if err := gc.DrawImageRect(sprite, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}

	for y := 20; y < 28; y++ {
		for x := 10; x < 18; x++ {
			sx := 4 + (x-10)/2
			sy := 8 + (y-20)/2
			assertPixel(t, dest, x, y, spriteColor(sx, sy))
		}
	}
	// Pixels just outside the destination rectangle stay transparent.
	assertTransparent(t, dest, 9, 20)
	assertTransparent(t, dest, 10, 19)
	assertTransparent(t, dest, 18, 27)
	assertTransparent(t, dest, 17, 28)
}

// TestDrawImageRectNonZeroBounds draws from a source image whose bounds do
// not start at the origin.
func TestDrawImageRectNonZeroBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(5, 7, 13, 15))
	for y := 7; y < 15; y++ {
		for x := 5; x < 13; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 0x40, 0xff})
		}
	}
	dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := NewGraphicContext(dest)

	srcRect := image.Rect(6, 8, 9, 12) // 3x4
	dstRect := image.Rect(2, 4, 8, 12) // 6x8, scale 2
	if err := gc.DrawImageRect(src, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	for y := 4; y < 12; y++ {
		for x := 2; x < 8; x++ {
			sx := 6 + (x-2)/2
			sy := 8 + (y-4)/2
			assertPixel(t, dest, x, y, color.RGBA{uint8(sx * 16), uint8(sy * 16), 0x40, 0xff})
		}
	}
	assertTransparent(t, dest, 1, 4)
	assertTransparent(t, dest, 2, 3)
	assertTransparent(t, dest, 8, 11)
	assertTransparent(t, dest, 7, 12)
}

// TestDrawImageRectMirror draws through a horizontal mirror matrix and
// checks that the source is sampled flipped.
func TestDrawImageRectMirror(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := NewGraphicContext(dest)
	gc.SetMatrixTransform(draw2d.Matrix{-1, 0, 0, 1, 40, 0})

	srcRect := image.Rect(0, 0, 4, 4)
	dstRect := image.Rect(0, 0, 4, 4)
	if err := gc.DrawImageRect(sprite, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	// User space (0..4, 0..4) maps to device (36..40, 0..4), mirrored in x:
	// device pixel (36, y) samples source pixel (3, y).
	for y := 0; y < 4; y++ {
		for dx := 36; dx < 40; dx++ {
			assertPixel(t, dest, dx, y, spriteColor(39-dx, y))
		}
	}
	assertTransparent(t, dest, 35, 0)
}

// TestDrawImageRectRotated draws through a 90 degree rotation composed with
// a translation and checks the pixel mapping.
func TestDrawImageRectRotated(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := NewGraphicContext(dest)
	// Rotation by 90 degrees: (x, y) -> (-y, x), then translate by (20, 0).
	gc.SetMatrixTransform(draw2d.Matrix{0, 1, -1, 0, 20, 0})

	srcRect := image.Rect(0, 0, 4, 4)
	dstRect := image.Rect(0, 0, 4, 4)
	if err := gc.DrawImageRect(sprite, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	// Device (dx, dy) corresponds to user (dy, 20-dx), so device pixel
	// (dx, dy) samples source pixel (dy, 19-dx).
	for dy := 0; dy < 4; dy++ {
		for dx := 16; dx < 20; dx++ {
			assertPixel(t, dest, dx, dy, spriteColor(dy, 19-dx))
		}
	}
	assertTransparent(t, dest, 15, 0)
	assertTransparent(t, dest, 20, 0)
	assertTransparent(t, dest, 16, 4)
}

// TestDrawImageRectAlpha checks alpha compositing of a sprite cell with a
// semi-transparent border over an opaque background, with draw.Over and
// draw.Src.
func TestDrawImageRectAlpha(t *testing.T) {
	// 4x4 sprite: opaque red 2x2 center, 50% blue border.
	newBorderedSprite := func() *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				img.SetRGBA(x, y, color.RGBA{0, 0, 0x80, 0x80})
			}
		}
		img.SetRGBA(1, 1, color.RGBA{0xff, 0, 0, 0xff})
		img.SetRGBA(2, 1, color.RGBA{0xff, 0, 0, 0xff})
		img.SetRGBA(1, 2, color.RGBA{0xff, 0, 0, 0xff})
		img.SetRGBA(2, 2, color.RGBA{0xff, 0, 0, 0xff})
		return img
	}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	t.Run("over", func(t *testing.T) {
		dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
		draw.Draw(dest, dest.Bounds(), image.NewUniform(white), image.ZP, draw.Src)
		gc := NewGraphicContext(dest)
		dstRect := image.Rect(10, 10, 18, 18) // scale 2
		err := gc.DrawImageRect(newBorderedSprite(), image.Rect(0, 0, 4, 4), dstRect, draw2d.FilterNearest, draw.Over)
		if err != nil {
			t.Fatalf("DrawImageRect failed: %v", err)
		}
		// Border: 50% blue over white.
		assertPixel(t, dest, 10, 10, color.RGBA{0x7f, 0x7f, 0xff, 0xff})
		assertPixel(t, dest, 11, 11, color.RGBA{0x7f, 0x7f, 0xff, 0xff})
		assertPixel(t, dest, 17, 16, color.RGBA{0x7f, 0x7f, 0xff, 0xff})
		// Center: opaque red replaces the background.
		assertPixel(t, dest, 13, 14, color.RGBA{0xff, 0, 0, 0xff})
		assertPixel(t, dest, 12, 12, color.RGBA{0xff, 0, 0, 0xff})
		// Outside: untouched white.
		assertPixel(t, dest, 9, 9, white)
	})

	t.Run("src", func(t *testing.T) {
		dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
		draw.Draw(dest, dest.Bounds(), image.NewUniform(white), image.ZP, draw.Src)
		gc := NewGraphicContext(dest)
		dstRect := image.Rect(10, 10, 18, 18)
		err := gc.DrawImageRect(newBorderedSprite(), image.Rect(0, 0, 4, 4), dstRect, draw2d.FilterNearest, draw.Src)
		if err != nil {
			t.Fatalf("DrawImageRect failed: %v", err)
		}
		// draw.Src replaces the destination, keeping the source alpha.
		assertPixel(t, dest, 10, 10, color.RGBA{0, 0, 0x80, 0x80})
		assertPixel(t, dest, 13, 13, color.RGBA{0xff, 0, 0, 0xff})
		assertPixel(t, dest, 9, 9, white)
	})
}

// TestDrawImageRectClippedByCanvas draws a destination rectangle that
// extends beyond the canvas and checks that only the visible part is drawn.
func TestDrawImageRectClippedByCanvas(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := NewGraphicContext(dest)

	srcRect := image.Rect(0, 0, 8, 8)
	dstRect := image.Rect(-4, -4, 4, 4) // only (0..4, 0..4) visible
	if err := gc.DrawImageRect(sprite, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	// Device (0,0) shows source pixel (4,4).
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			assertPixel(t, dest, x, y, spriteColor(x+4, y+4))
		}
	}
	assertTransparent(t, dest, 4, 0)
	assertTransparent(t, dest, 0, 4)
}

// TestDrawImageRectComposedTransform checks that the destination rectangle
// is positioned by the current transformation.
func TestDrawImageRectComposedTransform(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := NewGraphicContext(dest)
	gc.Translate(3, 7)

	srcRect := image.Rect(0, 0, 4, 4)
	dstRect := image.Rect(0, 0, 4, 4)
	if err := gc.DrawImageRect(sprite, srcRect, dstRect, draw2d.FilterNearest, draw.Over); err != nil {
		t.Fatalf("DrawImageRect failed: %v", err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			assertPixel(t, dest, 3+x, 7+y, spriteColor(x, y))
		}
	}
	assertTransparent(t, dest, 2, 7)
	assertTransparent(t, dest, 3, 6)
}

// TestDrawImageRectFilters checks that all filters draw and that a bilinear
// upscale of a uniform area stays uniform in its center.
func TestDrawImageRectFilters(t *testing.T) {
	red := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(red, red.Bounds(), image.NewUniform(color.RGBA{0xff, 0, 0, 0xff}), image.ZP, draw.Src)

	for _, filter := range []draw2d.Filter{draw2d.FilterNearest, draw2d.FilterLinear, draw2d.FilterBicubic} {
		dest := image.NewRGBA(image.Rect(0, 0, 64, 64))
		gc := NewGraphicContext(dest)
		if err := gc.DrawImageRect(red, red.Bounds(), image.Rect(0, 0, 64, 64), filter, draw.Over); err != nil {
			t.Fatalf("DrawImageRect(filter=%v) failed: %v", filter, err)
		}
		// The center of an 8x upscale of a uniform image is uniform.
		assertPixel(t, dest, 32, 32, color.RGBA{0xff, 0, 0, 0xff})
	}
}

// TestDrawImageRectErrors checks that invalid parameters report errors and
// leave the canvas untouched.
func TestDrawImageRectErrors(t *testing.T) {
	sprite := newCheckerSprite()
	rect := image.Rect(0, 0, 4, 4)
	nan := draw2d.NewIdentityMatrix()
	nan[2] = math.NaN()

	tests := []struct {
		name    string
		src     image.Image
		srcRect image.Rectangle
		dstRect image.Rectangle
		tr      draw2d.Matrix
		wantErr error
	}{
		{"nil image", nil, rect, rect, draw2d.NewIdentityMatrix(), draw2d.ErrNilImage},
		{"empty source", sprite, image.Rectangle{}, rect, draw2d.NewIdentityMatrix(), draw2d.ErrEmptySourceRect},
		{"source outside", sprite, image.Rect(100, 100, 104, 104), rect, draw2d.NewIdentityMatrix(), draw2d.ErrSourceRectOutside},
		{"negative dest", sprite, rect, image.Rectangle{Min: image.Pt(10, 0), Max: image.Pt(0, 4)}, draw2d.NewIdentityMatrix(), draw2d.ErrNegativeDestRect},
		{"non-finite matrix", sprite, rect, rect, nan, draw2d.ErrNonFiniteMatrix},
		{"huge scale", sprite, image.Rect(0, 0, 1, 1), image.Rect(0, 0, 100000, 4), draw2d.NewIdentityMatrix(), draw2d.ErrImageScaleTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
			gc := NewGraphicContext(dest)
			gc.SetMatrixTransform(tt.tr)
			err := gc.DrawImageRect(tt.src, tt.srcRect, tt.dstRect, draw2d.FilterNearest, draw.Over)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			assertTransparent(t, dest, 0, 0)
		})
	}
}

// TestDrawImageRectNoOp checks the cases that must draw nothing without
// failing.
func TestDrawImageRectNoOp(t *testing.T) {
	sprite := newCheckerSprite()
	srcRect := image.Rect(0, 0, 4, 4)

	for _, tt := range []struct {
		name    string
		dstRect image.Rectangle
		tr      draw2d.Matrix
	}{
		{"zero width dest", image.Rect(4, 0, 4, 4), draw2d.NewIdentityMatrix()},
		{"zero height dest", image.Rect(0, 4, 4, 4), draw2d.NewIdentityMatrix()},
		{"singular matrix", srcRect, draw2d.NewScaleMatrix(0, 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dest := image.NewRGBA(image.Rect(0, 0, 20, 20))
			gc := NewGraphicContext(dest)
			gc.SetMatrixTransform(tt.tr)
			if err := gc.DrawImageRect(sprite, srcRect, tt.dstRect, draw2d.FilterNearest, draw.Over); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for i, v := range dest.Pix {
				if v != 0 {
					t.Fatalf("dest.Pix[%d] = %d, want 0 (nothing drawn)", i, v)
				}
			}
		})
	}
}

// TestDrawImageUnchanged checks that the legacy DrawImage still draws the
// whole image at its bounds with the current matrix.
func TestDrawImageUnchanged(t *testing.T) {
	sprite := newCheckerSprite()
	dest := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := NewGraphicContext(dest)
	gc.Translate(2, 3)
	gc.DrawImage(sprite)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			assertPixel(t, dest, 2+x, 3+y, spriteColor(x, y))
		}
	}
	assertTransparent(t, dest, 1, 3)
	assertTransparent(t, dest, 2, 2)
}
