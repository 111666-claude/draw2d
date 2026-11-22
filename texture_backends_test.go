// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// checker2x2 builds the checker fixture shared by the backend consistency
// tests.
func checker2x2() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	return img
}

// TestTextureBackendsAgree checks that the image and SVG backends describe the
// same brush: both anchor the tile at the image origin, both use the same local
// transform, and both fall back to the same solid fill when the texture is
// unusable.
func TestTextureBackendsAgree(t *testing.T) {
	solid := color.RGBA{R: 10, G: 20, B: 30, A: 255}
	local := draw2d.NewRotationMatrix(math.Pi / 6)
	local.Translate(4, 5)

	draw := func(gc draw2d.GraphicContext, tex *draw2d.ImageTexture) {
		gc.SetFillColor(solid)
		gc.SetFillImageTexture(tex)
		gc.MoveTo(0, 0)
		gc.LineTo(50, 0)
		gc.LineTo(50, 50)
		gc.LineTo(0, 50)
		gc.Close()
		gc.Fill()
	}

	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	imgGC := draw2dimg.NewGraphicContext(img)
	draw(imgGC, draw2d.NewImageTexture(checker2x2(), local, draw2d.TextureRepeatBoth))

	svg := draw2dsvg.NewSvg()
	svgGC := draw2dsvg.NewGraphicContext(svg)
	draw(svgGC, draw2d.NewImageTexture(checker2x2(), local, draw2d.TextureRepeatBoth))

	if len(svg.Patterns) != 1 {
		t.Fatalf("svg emitted %d patterns, want 1", len(svg.Patterns))
	}
	// Rotation by pi/6 then a translation of (4,5) gives this matrix.
	if got, want := svg.Patterns[0].PatternTransform,
		"matrix(0.866,0.5,-0.5,0.866,0.964,6.33)"; got != want {
		t.Errorf("pattern transform = %q, want %q", got, want)
	}
	// The image backend must actually sample that local transform, so the
	// pixel at the centre matches the independently computed sample.
	ux, uy := local.InverseTransformPoint(25.5, 25.5)
	want := color.RGBAModel.Convert(draw2d.NewTextureSampler(
		draw2d.NewImageTexture(checker2x2(), local, draw2d.TextureRepeatBoth)).Sample(ux, uy)).(color.RGBA)
	if got := img.RGBAAt(25, 25); !nearRGBA(got, want, 3) {
		t.Errorf("image sample = %v, want %v", got, want)
	}

	// Both backends must fall back to the same solid fill for an unusable
	// texture.
	img2 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	imgGC2 := draw2dimg.NewGraphicContext(img2)
	draw(imgGC2, nil)
	if got := img2.RGBAAt(1, 1); got != solid {
		t.Errorf("image fallback = %v, want %v", got, solid)
	}

	svg2 := draw2dsvg.NewSvg()
	svgGC2 := draw2dsvg.NewGraphicContext(svg2)
	draw(svgGC2, nil)
	if len(svg2.Patterns) != 0 {
		t.Errorf("svg emitted %d patterns for a nil texture", len(svg2.Patterns))
	}
	if got, want := svg2.Groups[0].Fill, "#0A141E"; got != want {
		t.Errorf("svg fallback fill = %q, want %q", got, want)
	}
}

func nearRGBA(a, b color.RGBA, tol int) bool {
	near := func(x, y uint8) bool {
		d := int(x) - int(y)
		return d <= tol && d >= -tol
	}
	return near(a.R, b.R) && near(a.G, b.G) && near(a.B, b.B) && near(a.A, b.A)
}
