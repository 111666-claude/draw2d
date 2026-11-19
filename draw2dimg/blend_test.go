// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dkit"

	"github.com/golang/freetype/raster"
)

// fillRect paints an axis-aligned rectangle with the current fill color.
func fillRect(gc *draw2dimg.GraphicContext, c color.Color, x0, y0, x1, y1 float64) {
	gc.SetFillColor(c)
	draw2dkit.Rectangle(gc, x0, y0, x1, y1)
	gc.Fill()
}

// overlapScene draws an opaque green square and, on top of it, a red square at
// 50% alpha that partially overlaps. The picture has three regions:
//   - red only:      x in [10, 40)  with no green underneath
//   - overlap:       x in [50, 70)  where the translucent red covers green
//   - green only:    x in [70, 100)
func overlapScene(t *testing.T, gc *draw2dimg.GraphicContext) {
	t.Helper()
	fillRect(gc, color.NRGBA{0, 200, 0, 255}, 40, 10, 100, 90) // green backdrop
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 10, 30, 70, 70)  // translucent red
}

func near(got, want uint8, tolerance int) bool {
	d := int(got) - int(want)
	return d >= -tolerance && d <= tolerance
}

func TestBlendNormalDefaultUnchanged(t *testing.T) {
	// The default context mode is normal and produces the historical output.
	img := image.NewRGBA(image.Rect(0, 0, 120, 100))
	gc := draw2dimg.NewGraphicContext(img)
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Fatalf("default blend mode = %v, want normal", gc.GetBlendMode())
	}
	overlapScene(t, gc)

	// Region without a backdrop: translucent red over transparent black,
	// premultiplied -> (128, 0, 0, 128).
	if got := img.RGBAAt(20, 50); !near(got.R, 128, 1) || got.G != 0 || got.B != 0 || !near(got.A, 128, 1) {
		t.Errorf("red-only pixel = %v, want premultiplied 50%% red", got)
	}
	// Overlap region: 50% red over opaque green -> (128, 100, 0, 255).
	if got := img.RGBAAt(60, 50); !near(got.R, 128, 1) || !near(got.G, 100, 2) || got.B != 0 || got.A != 255 {
		t.Errorf("overlap pixel = %v, want (128,100,0,255)", got)
	}
	// Green-only region is untouched.
	if got := img.RGBAAt(90, 50); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("green-only pixel = %v, want unchanged green", got)
	}
}

func TestBlendModeMultiplyPerOperation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 100))
	gc := draw2dimg.NewGraphicContext(img)

	fillRect(gc, color.NRGBA{0, 200, 0, 255}, 40, 10, 100, 90)
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 10, 30, 70, 70)

	// Red-only region: multiply against a transparent backdrop behaves like
	// normal compositing, giving 50% premultiplied red.
	if got := img.RGBAAt(20, 50); !near(got.R, 128, 1) || got.G != 0 || got.B != 0 || !near(got.A, 128, 1) {
		t.Errorf("multiply red-only pixel = %v, want premultiplied 50%% red", got)
	}
	// Overlap: r = (1-ab)*csr + csr*cbr + (1-as)*cbr with ab=1, cbr=0 -> 0.
	// g = (1-ab)*csg + csg*cbg + (1-as)*cbg = (1-0.5) * 0.784 = 0.392 -> 100.
	if got := img.RGBAAt(60, 50); got.R != 0 || !near(got.G, 100, 2) || got.B != 0 || got.A != 255 {
		t.Errorf("multiply overlap pixel = %v, want (0,100,0,255)", got)
	}
	// Green-only region is untouched.
	if got := img.RGBAAt(90, 50); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("multiply green-only pixel = %v, want unchanged green", got)
	}
}

func TestBlendModeScreenPerOperation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 100))
	gc := draw2dimg.NewGraphicContext(img)

	fillRect(gc, color.NRGBA{0, 200, 0, 255}, 40, 10, 100, 90)
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 10, 30, 70, 70)

	// Red-only region: same as normal against a transparent backdrop.
	if got := img.RGBAAt(20, 50); !near(got.R, 128, 1) || got.G != 0 || got.B != 0 || !near(got.A, 128, 1) {
		t.Errorf("screen red-only pixel = %v, want premultiplied 50%% red", got)
	}
	// Overlap: co = cs + cb - cs*cb.
	// r = 0.5 + 0 - 0 = 0.5 -> 128; g = 0 + 0.784 - 0 = 0.784 -> 200.
	if got := img.RGBAAt(60, 50); !near(got.R, 128, 1) || !near(got.G, 200, 1) || got.B != 0 || got.A != 255 {
		t.Errorf("screen overlap pixel = %v, want (128,200,0,255)", got)
	}
}

func TestBlendModeResetWithRestore(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 100))
	gc := draw2dimg.NewGraphicContext(img)
	fillRect(gc, color.NRGBA{0, 200, 0, 255}, 40, 10, 100, 90)

	gc.Save()
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	if gc.GetBlendMode() != draw2d.BlendMultiply {
		t.Fatal("SetBlendMode did not take effect")
	}
	gc.Restore()
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Errorf("Restore did not restore blend mode, got %v", gc.GetBlendMode())
	}
}

func TestSetBlendModeInvalid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.SetBlendMode(draw2d.BlendMode(99)); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("SetBlendMode(invalid) error = %v, want ErrInvalidBlendMode", err)
	}
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Errorf("invalid mode changed the current mode to %v", gc.GetBlendMode())
	}
	// The interface exposes the same contract.
	var _ draw2d.GraphicContext = gc
}

func TestLayerMultiply(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 100))
	gc := draw2dimg.NewGraphicContext(img)
	fillRect(gc, color.NRGBA{0, 200, 0, 255}, 40, 10, 100, 90)

	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	// Two overlapping translucent reds inside the layer combine normally with
	// each other, then the group is multiplied with the backdrop.
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 10, 30, 70, 70)
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 50, 30, 70, 70)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}

	// Green-only region untouched.
	if got := img.RGBAAt(90, 50); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("green-only pixel = %v, want unchanged green", got)
	}
	// Overlap region: source alpha inside the layer reaches ~0.75, so the
	// green channel is (1-0.75)*200 = 50 and red is 0.
	if got := img.RGBAAt(60, 50); got.R != 0 || !near(got.G, 50, 2) || got.A != 255 {
		t.Errorf("layer multiply overlap = %v, want (0,~50,0,255)", got)
	}
	// Red-only region on a transparent backdrop keeps red.
	if got := img.RGBAAt(20, 50); !near(got.R, 128, 1) || got.A < 100 {
		t.Errorf("layer multiply red-only = %v, want translucent red", got)
	}
}

func TestLayerOpacity(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 0.5); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, 0, 0, 40, 40)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(20, 20); !near(got.R, 128, 1) || !near(got.A, 128, 1) {
		t.Errorf("50%% layer opacity pixel = %v, want ~(128,0,0,128)", got)
	}
}

func TestLayerEmptyIsNoOp(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 77 // arbitrary backdrop alpha
	}
	before := append([]byte(nil), img.Pix...)
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if img.Pix[i] != before[i] {
			t.Fatalf("empty layer modified pixel %d: %d -> %d", i, before[i], img.Pix[i])
		}
	}
}

func TestLayerNested(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := draw2dimg.NewGraphicContext(img)

	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{0, 0, 255, 255}, 0, 0, 40, 20) // blue top half
	if err := gc.BeginLayer(draw2d.BlendScreen, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, 0, 10, 40, 30) // red middle band
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	// Inside the outer layer, the inner screen layer drew red over blue in the
	// overlap: screen(blue, red) = (1, 0, 1) magenta.
	if got := img.RGBAAt(20, 15); got != (color.RGBA{255, 0, 255, 255}) {
		t.Errorf("nested overlap = %v, want magenta screen", got)
	}
	// Bottom red band on its own is red.
	if got := img.RGBAAt(20, 25); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("bottom band = %v, want red", got)
	}
}

func TestLayerErrors(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := draw2dimg.NewGraphicContext(img)

	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("EndLayer without BeginLayer = %v, want ErrLayerNotOpen", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMode(99), 1); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("BeginLayer invalid mode = %v, want ErrInvalidBlendMode", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMultiply, math.NaN()); !errors.Is(err, draw2d.ErrInvalidOpacity) {
		t.Errorf("BeginLayer NaN opacity = %v, want ErrInvalidOpacity", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMultiply, math.Inf(1)); !errors.Is(err, draw2d.ErrInvalidOpacity) {
		t.Errorf("BeginLayer +Inf opacity = %v, want ErrInvalidOpacity", err)
	}
	// Failed BeginLayer calls must not leave a layer open.
	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("EndLayer after failed BeginLayer = %v, want ErrLayerNotOpen", err)
	}
}

func TestLayerTooLarge(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	gc := draw2dimg.NewGraphicContext(img)
	gc.MaxLayerPixels = 100 // less than 32*32=1024
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); !errors.Is(err, draw2d.ErrLayerTooLarge) {
		t.Errorf("BeginLayer over limit = %v, want ErrLayerTooLarge", err)
	}
	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("rejected layer left a layer open: %v", err)
	}
	// A limit at or above the size succeeds.
	gc.MaxLayerPixels = 1024
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatalf("BeginLayer at limit: %v", err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
}

func TestLayerDepthLimit(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	gc := draw2dimg.NewGraphicContext(img)
	for i := 0; i < draw2d.MaxLayerDepth; i++ {
		if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
			t.Fatalf("BeginLayer %d: %v", i, err)
		}
	}
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); !errors.Is(err, draw2d.ErrLayerDepthExceeded) {
		t.Errorf("BeginLayer past depth limit = %v, want ErrLayerDepthExceeded", err)
	}
	for i := 0; i < draw2d.MaxLayerDepth; i++ {
		if err := gc.EndLayer(); err != nil {
			t.Fatalf("EndLayer %d: %v", i, err)
		}
	}
}

func TestLayerClipsToCanvas(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	// Draw far outside the canvas; the layer must clip instead of growing.
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, -1000, -1000, 2000, 2000)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(10, 10); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("clipped layer pixel = %v, want red", got)
	}
}

func TestLayerStateRestored(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{0, 0, 255, 255})
	gc.SetLineWidth(3)

	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	gc.SetFillColor(color.NRGBA{255, 0, 0, 255})
	gc.SetLineWidth(9)
	gc.Translate(5, 5)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}

	if r, g, b, a := gc.Current.FillColor.RGBA(); r != 0 || g != 0 || b != 0xffff || a != 0xffff {
		t.Errorf("fill color after layer = (%d,%d,%d,%d), want opaque blue", r, g, b, a)
	}
	if gc.Current.LineWidth != 3 {
		t.Errorf("line width after layer = %v, want 3", gc.Current.LineWidth)
	}
	if !gc.Current.Tr.IsIdentity() {
		t.Errorf("transform after layer = %v, want identity", gc.Current.Tr)
	}
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Errorf("blend mode after layer = %v, want normal", gc.GetBlendMode())
	}
}

func TestLayerDestinationNonRGBA(t *testing.T) {
	// Layers must work when the destination is a custom draw.Image rather than
	// *image.RGBA, which is the documented requirement of
	// NewGraphicContextWithPainter.
	dst := &nonRGBAImage{image.NewNRGBA(image.Rect(0, 0, 20, 20))}
	gc := draw2dimg.NewGraphicContextWithPainter(dst, raster.NewRGBAPainter(image.NewRGBA(dst.Bounds())))
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{0, 255, 0, 255}, 0, 0, 20, 20)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := dst.NRGBAAt(10, 10); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Errorf("generic destination pixel = %v, want green", got)
	}
}

func TestDrawImageBlendMode(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 0; i < len(canvas.Pix); i += 4 {
		canvas.Pix[i+1], canvas.Pix[i+3] = 255, 255 // opaque green
	}
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+3] = 255, 255 // opaque red
	}
	gc := draw2dimg.NewGraphicContext(canvas)
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	gc.DrawImage(src)

	if got := canvas.RGBAAt(10, 10); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("multiplied image pixel = %v, want black", got)
	}
}

func TestBlendModeTransparentBackground(t *testing.T) {
	// On a fully transparent canvas, normal/over of translucent colors keeps
	// their premultiplied hue; multiply and screen do the same.
	for _, mode := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen} {
		img := image.NewRGBA(image.Rect(0, 0, 20, 20))
		gc := draw2dimg.NewGraphicContext(img)
		if err := gc.SetBlendMode(mode); err != nil {
			t.Fatal(err)
		}
		fillRect(gc, color.NRGBA{255, 0, 0, 128}, 0, 0, 20, 20)
		got := img.RGBAAt(10, 10)
		if !near(got.R, 128, 1) || got.G != 0 || got.B != 0 || !near(got.A, 128, 1) {
			t.Errorf("%v on transparent canvas = %v, want premultiplied 50%% red", mode, got)
		}
	}
}

func TestBlendModeClippedDrawingStable(t *testing.T) {
	// Blend drawing must clip to the canvas exactly like normal drawing: a
	// shape that extends past the edges affects only the visible pixels.
	for _, mode := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen} {
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		gc := draw2dimg.NewGraphicContext(img)
		if err := gc.SetBlendMode(mode); err != nil {
			t.Fatal(err)
		}
		fillRect(gc, color.NRGBA{255, 0, 0, 255}, -50, -50, 60, 5)
		if got := img.RGBAAt(5, 2); got != (color.RGBA{255, 0, 0, 255}) {
			t.Errorf("%v clipped pixel = %v, want red", mode, got)
		}
		if got := img.RGBAAt(5, 9); got != (color.RGBA{}) {
			t.Errorf("%v pixel outside shape = %v, want transparent", mode, got)
		}
	}
}

func TestBlendModeOverlapBothDrawCalls(t *testing.T) {
	// Both drawing operations use the blend mode, so their overlap is blended
	// twice rather than the second one being a plain over.
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 255, 255, 255}, 0, 0, 40, 40)
	fillRect(gc, color.NRGBA{100, 100, 100, 255}, 20, 0, 60, 40)
	// Multiply of grey(100) and white is grey(100).
	if got := img.RGBAAt(10, 20); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("white-only region = %v, want white", got)
	}
	if got := img.RGBAAt(50, 20); got != (color.RGBA{100, 100, 100, 255}) {
		t.Errorf("grey-only region = %v, want grey", got)
	}
	if got := img.RGBAAt(30, 20); got != (color.RGBA{100, 100, 100, 255}) {
		t.Errorf("overlap region = %v, want 100*255/255=100 grey", got)
	}
}

// nonRGBAImage hides the concrete image type so the generic composite path is
// exercised, while still implementing draw.Image.
type nonRGBAImage struct {
	*image.NRGBA
}

func (m *nonRGBAImage) Set(x, y int, c color.Color) { m.NRGBA.Set(x, y, c) }

var _ draw.Image = (*image.RGBA)(nil)

func TestLayerNestedTransparentInner(t *testing.T) {
	// A transparent inner layer must not erase what the outer layer already
	// painted: normal compositing of the transparent group is a no-op.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{0, 0, 255, 255}, 0, 0, 20, 20)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil { // empty
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(10, 10); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("inner empty layer erased outer content: %v", got)
	}
}

func TestLayerMultiplyOnTransparentBackdrop(t *testing.T) {
	// Multiply on a fully transparent destination must be identical to normal:
	// the composite reduces to the source.
	for _, c := range []color.Color{
		color.NRGBA{255, 0, 0, 128},
		color.NRGBA{0, 255, 0, 255},
		color.NRGBA{10, 20, 30, 64},
	} {
		normal := image.NewRGBA(image.Rect(0, 0, 8, 8))
		multiplied := image.NewRGBA(image.Rect(0, 0, 8, 8))
		gcN := draw2dimg.NewGraphicContext(normal)
		fillRect(gcN, c, 0, 0, 8, 8)
		gcM := draw2dimg.NewGraphicContext(multiplied)
		if err := gcM.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
			t.Fatal(err)
		}
		fillRect(gcM, c, 0, 0, 8, 8)
		if err := gcM.EndLayer(); err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if a, b := normal.RGBAAt(x, y), multiplied.RGBAAt(x, y); a != b {
					t.Fatalf("color %v: multiply layer %v differs from normal %v", c, b, a)
				}
			}
		}
	}
}

func TestLayerDoesNotDoubleApply(t *testing.T) {
	// The layer must be composited once: a fully opaque red over green with
	// multiply yields black in the covered area only, not twice-blended values.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+1], img.Pix[i+3] = 200, 255
	}
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, 5, 5, 10, 10)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(7, 7); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("covered pixel = %v, want black", got)
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("uncovered pixel = %v, want unchanged green", got)
	}
}

func TestBlendModeNegativeOpacityLayer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendMultiply, -3); err != nil {
		t.Fatalf("negative opacity should clamp, got %v", err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, 0, 0, 8, 8)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(4, 4); got != (color.RGBA{}) {
		t.Errorf("zero-opacity layer changed destination: %v", got)
	}
}

func TestLayerResetAfterEnd(t *testing.T) {
	// After EndLayer the context must draw into the original destination
	// again, with the original painter.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{0, 0, 255, 255}, 0, 0, 10, 10)
	if got := img.RGBAAt(5, 5); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("draw after layer did not reach the destination: %v", got)
	}
}

func TestLayerWithClearRect(t *testing.T) {
	// A ClearRect inside a layer paints the layer surface, and the group is
	// blended with the backdrop exactly like any other layer content.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+1], img.Pix[i+3] = 200, 255 // opaque green backdrop
	}
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	gc.SetFillColor(color.NRGBA{255, 0, 0, 255})
	gc.ClearRect(0, 0, 10, 20)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(5, 5); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("cleared-then-multiplied pixel = %v, want black", got)
	}
	if got := img.RGBAAt(15, 5); got != (color.RGBA{0, 200, 0, 255}) {
		t.Errorf("untouched pixel = %v, want unchanged green", got)
	}
}

func TestLayerAfterClearRectOrder(t *testing.T) {
	// ClearRect drawn before the layer is already in the backdrop and is not
	// re-read from the layer surface.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := draw2dimg.NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{0, 0, 255, 255})
	gc.ClearRect(0, 0, 20, 20)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 128}, 0, 0, 10, 10)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(5, 5); !near(got.R, 128, 1) || !near(got.B, 128, 1) || got.A != 255 {
		t.Errorf("layer over ClearRect = %v, want 50%% red over blue", got)
	}
	if got := img.RGBAAt(15, 15); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ClearRect area = %v, want blue", got)
	}
}

func TestBlendInsideLayerMarksLayerDirty(t *testing.T) {
	// A per-operation blend performed inside a layer must still be included
	// when the layer is composited.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, color.NRGBA{255, 0, 0, 255}, 0, 0, 20, 20)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(10, 10); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("blend inside layer = %v, want red", got)
	}
}

func TestLayerEmptyCanvas(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 5, 5, 5)) // empty canvas
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); !errors.Is(err, draw2d.ErrEmptyCanvas) {
		t.Errorf("BeginLayer on empty canvas = %v, want ErrEmptyCanvas", err)
	}
	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("rejected layer left one open: %v", err)
	}
}

func TestLayerRejectsEmpty(t *testing.T) {
	// A layer whose destination is a zero rectangle is rejected without
	// allocating anything.
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	gc := draw2dimg.NewGraphicContext(img)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); !errors.Is(err, draw2d.ErrEmptyCanvas) {
		t.Errorf("BeginLayer on 0x0 = %v, want ErrEmptyCanvas", err)
	}
}

// straightPremethod returns the premultiplied form of a straight color, the
// representation the rasterizer produces for fully covered pixels.
func straightPremul(c color.Color) [4]uint8 {
	r, g, b, a := c.RGBA()
	return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func TestBlendPerPixelMatchesReference(t *testing.T) {
	// Draw an opaque green rectangle followed by a translucent red rectangle
	// and check EVERY interior pixel against the blending formula.
	const w, h = 64, 48
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	gc := draw2dimg.NewGraphicContext(img)

	green := color.NRGBA{0, 204, 0, 255}
	red := color.NRGBA{255, 40, 0, 128}

	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, green, 0, 0, w, h)
	fillRect(gc, red, 16, 8, 52, 40)

	start := [4]uint8{} // transparent black
	afterGreen := draw2dbase.BlendPremultiplied(start, straightPremul(green), draw2d.BlendMultiply, 1)
	afterRed := draw2dbase.BlendPremultiplied(afterGreen, straightPremul(red), draw2d.BlendMultiply, 1)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			want := afterGreen
			if x >= 16 && x < 52 && y >= 8 && y < 40 {
				want = afterRed
			}
			if got := img.RGBAAt(x, y); got != (color.RGBA{want[0], want[1], want[2], want[3]}) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestBlendReferenceScreenScene(t *testing.T) {
	const w, h = 40, 30
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	gc := draw2dimg.NewGraphicContext(img)

	base := color.NRGBA{20, 60, 200, 255}
	glow := color.NRGBA{255, 200, 0, 96}
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	fillRect(gc, base, 0, 0, w, h)
	fillRect(gc, glow, 5, 5, 35, 25)

	start := [4]uint8{}
	afterBase := draw2dbase.BlendPremultiplied(start, straightPremul(base), draw2d.BlendScreen, 1)
	afterGlow := draw2dbase.BlendPremultiplied(afterBase, straightPremul(glow), draw2d.BlendScreen, 1)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			want := afterBase
			if x >= 5 && x < 35 && y >= 5 && y < 25 {
				want = afterGlow
			}
			got := img.RGBAAt(x, y)
			if got != (color.RGBA{want[0], want[1], want[2], want[3]}) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestExplicitNormalMatchesDefault(t *testing.T) {
	// Setting BlendNormal explicitly must produce byte-identical output to the
	// untouched default context.
	draw := func(explicit bool) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 80, 60))
		gc := draw2dimg.NewGraphicContext(img)
		if explicit {
			if err := gc.SetBlendMode(draw2d.BlendNormal); err != nil {
				t.Fatal(err)
			}
		}
		gc.SetFillColor(color.NRGBA{10, 120, 220, 255})
		draw2dkit.Ellipse(gc, 40, 30, 30, 20)
		gc.Fill()
		gc.SetFillColor(color.NRGBA{255, 0, 0, 90})
		draw2dkit.Rectangle(gc, 10, 10, 60, 50)
		gc.Fill()
		gc.SetLineWidth(3)
		gc.SetStrokeColor(color.NRGBA{0, 0, 0, 180})
		draw2dkit.Circle(gc, 30, 25, 18)
		gc.Stroke()
		return img
	}
	a, b := draw(false), draw(true)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Errorf("explicit normal differs from default output")
	}
}
