// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/golang/freetype/raster"
	"github.com/llgcode/draw2d"
)

// checkerTexture returns a 2x2 checkerboard: red, green / blue, yellow. All
// texels are opaque, so any transparency in the output is either a path edge
// or a texture wrap bug.
func checkerTexture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	return img
}

// fillCanvas fills the whole destination with the current fill brush.
func fillCanvas(gc *GraphicContext, img image.Image) {
	b := img.Bounds()
	gc.MoveTo(float64(b.Min.X), float64(b.Min.Y))
	gc.LineTo(float64(b.Max.X), float64(b.Min.Y))
	gc.LineTo(float64(b.Max.X), float64(b.Max.Y))
	gc.LineTo(float64(b.Min.X), float64(b.Max.Y))
	gc.Close()
	gc.Fill()
}

func TestFillImageTexturePhaseIdentity(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)

	// The texture is anchored at user coordinate 0, which is also device 0,
	// so the 2x2 checker tiles across the canvas.
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, color.RGBA{R: 255, A: 255}},
		{1, 0, color.RGBA{G: 255, A: 255}},
		{0, 1, color.RGBA{B: 255, A: 255}},
		{1, 1, color.RGBA{R: 255, G: 255, A: 255}},
		{2, 0, color.RGBA{R: 255, A: 255}},
		{3, 1, color.RGBA{R: 255, G: 255, A: 255}},
		{7, 7, color.RGBA{R: 255, G: 255, A: 255}},
	} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("(%d,%d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestFillImageTextureNoSeams(t *testing.T) {
	// A repeated opaque checker filled over a large area must not show any
	// transparent or dark seam line at tile boundaries.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)

	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if a := img.RGBAAt(x, y).A; a != 255 {
				t.Fatalf("seam at (%d,%d): alpha = %d", x, y, a)
			}
		}
	}
}

func TestFillImageTextureNonZeroBoundsPhase(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 12, 22))
	src.SetRGBA(10, 20, color.RGBA{R: 255, A: 255})
	src.SetRGBA(11, 20, color.RGBA{G: 255, A: 255})
	src.SetRGBA(10, 21, color.RGBA{B: 255, A: 255})
	src.SetRGBA(11, 21, color.RGBA{R: 255, G: 255, A: 255})

	img := image.NewRGBA(image.Rect(0, 0, 6, 6))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(src, draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)

	// The tile is anchored at Bounds.Min (10, 20), so device (0,0) starts at
	// the first texel and repeats with the 2x2 period.
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, color.RGBA{R: 255, A: 255}},
		{1, 0, color.RGBA{G: 255, A: 255}},
		{0, 1, color.RGBA{B: 255, A: 255}},
		{1, 1, color.RGBA{R: 255, G: 255, A: 255}},
		{2, 2, color.RGBA{R: 255, A: 255}},
		{5, 5, color.RGBA{R: 255, G: 255, A: 255}},
	} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("(%d,%d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestFillImageTextureLocalScale(t *testing.T) {
	// Scale image pixel space by 4 and verify every sampled pixel against the
	// inverse mapping, which is independent of the painter's incremental math.
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	gc := NewGraphicContext(img)
	local := draw2d.NewScaleMatrix(4, 4)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), local, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)

	sampler := draw2d.NewTextureSampler(draw2d.NewImageTexture(checkerTexture(), local, draw2d.TextureRepeatBoth))
	for _, p := range []struct{ x, y int }{{0, 0}, {1, 0}, {3, 7}, {8, 8}, {15, 4}, {23, 23}} {
		dx, dy := float64(p.x)+0.5, float64(p.y)+0.5
		ux, uy := local.InverseTransformPoint(dx, dy)
		want := color.RGBAModel.Convert(sampler.Sample(ux, uy)).(color.RGBA)
		if got := img.RGBAAt(p.x, p.y); !rgbaNear(got, want, 3) {
			t.Errorf("(%d,%d): got %v, want %v", p.x, p.y, got, want)
		}
	}
}

func TestFillImageTextureCurrentMatrixPhase(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	gc.Translate(1, 1)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)

	// The path is specified in user coordinates and translated by (1,1), so
	// it covers device (1,1)-(9,9). Device (1,1) maps back to user (0,0),
	// which is the first texel: red. The device pixel before it is outside
	// the path.
	if got := img.RGBAAt(1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("translated phase (1,1) = %v, want red", got)
	}
	if got := img.RGBAAt(2, 1); got != (color.RGBA{G: 255, A: 255}) {
		t.Errorf("translated phase (2,1) = %v, want green", got)
	}
	if got := img.RGBAAt(1, 2); got != (color.RGBA{B: 255, A: 255}) {
		t.Errorf("translated phase (1,2) = %v, want blue", got)
	}
	if got := img.RGBAAt(0, 0); got.A != 0 {
		t.Errorf("pixel outside the translated path = %v, want transparent", got)
	}
}

func TestFillImageTextureClampTransparentOutside(t *testing.T) {
	// A 2x2 texture with no repeat: the pixels beyond the image stay
	// transparent instead of repeating.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureClamp))
	fillCanvas(gc, img)

	if got := img.RGBAAt(0, 0); got.A != 255 {
		t.Errorf("inside pixel alpha = %d, want 255", got.A)
	}
	if got := img.RGBAAt(4, 4); got.A != 0 {
		t.Errorf("outside pixel = %v, want transparent", got)
	}
}

func TestFillImageTextureAlphaBorder(t *testing.T) {
	// 4x4 image: opaque red centre, 1px border at 50% alpha, transparent
	// corners. The border must composite as half transparent red, not dark.
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 1; y < 3; y++ {
		for x := 1; x < 3; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for i := 0; i < 4; i++ {
		src.SetNRGBA(i, 0, color.NRGBA{R: 255, A: 128})
		src.SetNRGBA(i, 3, color.NRGBA{R: 255, A: 128})
		src.SetNRGBA(0, i, color.NRGBA{R: 255, A: 128})
		src.SetNRGBA(3, i, color.NRGBA{R: 255, A: 128})
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(src, draw2d.Matrix{}, draw2d.TextureClamp))
	fillCanvas(gc, img)

	// Device column 0 maps to the left border texel (x=0), which is 50%
	// alpha red. Premultiplied compositing must keep its hue.
	border := img.RGBAAt(0, 2)
	if border.A < 120 || border.A > 136 {
		t.Errorf("border alpha = %d, want about 128", border.A)
	}
	if border.R < 120 || border.R > 136 {
		t.Errorf("border red = %d, premultiplied red should follow alpha", border.R)
	}
	// The centre is opaque red.
	if got := img.RGBAAt(2, 2); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("centre = %v, want opaque red", got)
	}
}

func TestFillImageTextureMatchesInverseTransform(t *testing.T) {
	// Rotate the current matrix and fill a quadrilateral that covers the
	// canvas. The colour of every pixel must equal the texture sampled at the
	// device position mapped back through the current matrix; this checks the
	// device to image mapping, including its translation term.
	const size = 48
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	gc := NewGraphicContext(img)
	gc.Translate(size/2, size/2)
	gc.Rotate(math.Pi / 5)
	gc.Translate(-size/2, -size/2)
	tr := gc.GetMatrixTransform()
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.MoveTo(-float64(size), -float64(size))
	gc.LineTo(2*size, -float64(size))
	gc.LineTo(2*size, 2*size)
	gc.LineTo(-float64(size), 2*size)
	gc.Close()
	gc.Fill()

	sampler := draw2d.NewTextureSampler(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	for _, p := range []struct{ x, y int }{{0, 0}, {5, 17}, {size / 2, size / 2}, {23, 40}, {47, 47}} {
		dx, dy := float64(p.x)+0.5, float64(p.y)+0.5
		ux, uy := tr.InverseTransformPoint(dx, dy)
		want := color.RGBAModel.Convert(sampler.Sample(ux, uy)).(color.RGBA)
		got := img.RGBAAt(p.x, p.y)
		if !rgbaNear(got, want, 3) {
			t.Errorf("(%d,%d): got %v, want %v (user %.3f,%.3f)", p.x, p.y, got, want, ux, uy)
		}
	}
	// A rotated opaque repeated fill must still have no holes.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if img.RGBAAt(x, y).A == 0 {
				t.Fatalf("rotated fill has a hole at (%d,%d)", x, y)
			}
		}
	}
}

func rgbaNear(a, b color.RGBA, tol int) bool {
	near := func(x, y uint8) bool {
		d := int(x) - int(y)
		return d <= tol && d >= -tol
	}
	return near(a.R, b.R) && near(a.G, b.G) && near(a.B, b.B) && near(a.A, b.A)
}

func TestFillImageTextureFallbacks(t *testing.T) {
	solid := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		tex  *draw2d.ImageTexture
	}{
		{"nil", nil},
		{"nil image", draw2d.NewImageTexture(nil, draw2d.Matrix{}, draw2d.TextureClamp)},
		{"empty image", draw2d.NewImageTexture(image.NewRGBA(image.Rect(0, 0, 0, 0)), draw2d.Matrix{}, draw2d.TextureClamp)},
		{"non-finite transform", draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{nan, 0, 0, 1, 0, 0}, draw2d.TextureClamp)},
		{"singular transform", draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{1, 2, 2, 4, 0, 0}, draw2d.TextureClamp)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 4, 4))
			gc := NewGraphicContext(img)
			gc.SetFillColor(solid)
			gc.SetFillImageTexture(tc.tex)
			fillCanvas(gc, img)
			for _, p := range []struct{ x, y int }{{0, 0}, {2, 2}, {3, 3}} {
				if got := img.RGBAAt(p.x, p.y); got != solid {
					t.Errorf("(%d,%d) = %v, want solid fallback %v", p.x, p.y, got, solid)
				}
			}
		})
	}
}

func TestFillImageTextureSolidFillUnchanged(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	gc := NewGraphicContext(img)
	solid := color.RGBA{R: 10, G: 20, B: 30, A: 255}
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureClamp))
	gc.SetFillColor(solid) // installing a solid colour must drop the texture
	fillCanvas(gc, img)
	if got := img.RGBAAt(1, 1); got != solid {
		t.Errorf("solid fill after SetFillColor = %v, want %v", got, solid)
	}
}

func TestFillStrokeUsesTextureForFillOnly(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	gc := NewGraphicContext(img)
	gc.SetStrokeColor(color.RGBA{A: 255})
	gc.SetLineWidth(2)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.MoveTo(2, 2)
	gc.LineTo(14, 2)
	gc.LineTo(14, 14)
	gc.LineTo(2, 14)
	gc.Close()
	gc.FillStroke()
	// The interior is textured and the stroke edge is black.
	if got := img.RGBAAt(8, 8); got == (color.RGBA{}) {
		t.Errorf("interior is empty")
	}
	if got := img.RGBAAt(2, 8); got.R != 0 || got.G != 0 || got.B != 0 {
		t.Errorf("stroke pixel = %v, want black", got)
	}
}

func TestTextureSaveRestore(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	tex := draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth)
	gc.SetFillImageTexture(tex)
	gc.Save()
	gc.SetFillColor(color.RGBA{R: 9, G: 9, B: 9, A: 255})
	gc.Restore()
	fillCanvas(gc, img)
	if got := img.RGBAAt(1, 1); got == (color.RGBA{R: 9, G: 9, B: 9, A: 255}) {
		t.Errorf("restore lost the texture brush")
	}
	if got := img.RGBAAt(1, 1); got != (color.RGBA{R: 255, G: 255, A: 255}) {
		t.Errorf("restored texture sample = %v, want yellow", got)
	}
}

func TestFillImageTextureMatchesReferenceAllPolicies(t *testing.T) {
	// For every repeat policy, the device pixel must equal the texture sampled
	// at the point mapped back through device -> user -> image. This exercises
	// the CTM composition with a rotation, translation and non-uniform local
	// scale that are not aligned with the pixel grid.
	tr := draw2d.NewRotationMatrix(0.7)
	tr.Translate(5, 7)
	local := draw2d.NewScaleMatrix(1.3, 0.9)
	local.Translate(3, -2)

	for _, rep := range []draw2d.TextureRepeat{
		draw2d.TextureClamp, draw2d.TextureRepeatX, draw2d.TextureRepeatY, draw2d.TextureRepeatBoth,
	} {
		t.Run(rep.String(), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 40, 40))
			gc := NewGraphicContext(img)
			gc.SetMatrixTransform(tr)
			gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), local, rep))
			gc.MoveTo(-100, -100)
			gc.LineTo(100, -100)
			gc.LineTo(100, 100)
			gc.LineTo(-100, 100)
			gc.Close()
			gc.Fill()

			sampler := draw2d.NewTextureSampler(draw2d.NewImageTexture(checkerTexture(), local, rep))
			for y := 0; y < 40; y += 3 {
				for x := 0; x < 40; x += 3 {
					ux, uy := tr.InverseTransformPoint(float64(x)+0.5, float64(y)+0.5)
					ix, iy := local.InverseTransformPoint(ux, uy)
					want := color.RGBAModel.Convert(sampler.Sample(ix, iy)).(color.RGBA)
					if got := img.RGBAAt(x, y); !rgbaNear(got, want, 3) {
						t.Fatalf("(%d,%d): got %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

func TestFillImageTextureUserSpacePhase(t *testing.T) {
	// The texture is anchored in user space, not to each path's local origin:
	// two paths at different user positions sample the same texels at equal
	// user coordinates.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	gc := NewGraphicContext(img)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.LineTo(10, 20)
	gc.LineTo(0, 20)
	gc.Close()
	gc.Fill()
	gc.MoveTo(20, 0)
	gc.LineTo(30, 0)
	gc.LineTo(30, 20)
	gc.LineTo(20, 20)
	gc.Close()
	gc.Fill()
	// x = 0 and x = 20 are both even multiples of the 2-unit tile, so their
	// phases match.
	for _, p := range []struct{ x, y int }{{1, 1}, {0, 0}, {5, 10}, {9, 19}} {
		a := img.RGBAAt(p.x, p.y)
		b := img.RGBAAt(p.x+20, p.y)
		if a != b {
			t.Errorf("x=%d: %v vs x=%d: %v", p.x, a, p.x+20, b)
		}
	}
}

func TestFillImageTextureNonFiniteCTM(t *testing.T) {
	// A non-finite current matrix makes the composed device mapping singular.
	// The fill must fall back to solid painting and must not panic.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := NewGraphicContext(img)
	gc.SetFillColor(color.RGBA{R: 5, G: 6, B: 7, A: 255})
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.SetMatrixTransform(draw2d.Matrix{math.NaN(), 0, 0, 1, 0, 0})
	gc.MoveTo(0, 0)
	gc.LineTo(8, 0)
	gc.LineTo(8, 8)
	gc.LineTo(0, 8)
	gc.Close()
	gc.Fill() // must not panic
}

func TestTexturePainterRejectsCustomPainter(t *testing.T) {
	// NewGraphicContextWithPainter installs a caller-supplied painter. A
	// texture brush must not silently bypass it; the custom painter keeps
	// receiving the spans.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	painter := &recordingPainter{}
	gc := NewGraphicContextWithPainter(img, painter)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillCanvas(gc, img)
	if painter.painted == 0 {
		t.Error("custom painter was bypassed by the texture brush")
	}
}

type recordingPainter struct {
	painted int
}

func (p *recordingPainter) Paint(ss []raster.Span, done bool) { p.painted += len(ss) }

func (p *recordingPainter) SetColor(color.Color) {}
