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

// fillRectWithGradient fills a rectangle covering the whole canvas using the
// given gradient. The fill color is deliberately distinct from the gradient
// endpoints so a fallback to the solid color is detectable.
func fillRectWithGradient(img *image.RGBA, g *draw2d.LinearGradient) {
	gc := NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	gc.SetFillLinearGradient(g)
	b := img.Bounds()
	gc.MoveTo(float64(b.Min.X), float64(b.Min.Y))
	gc.LineTo(float64(b.Max.X), float64(b.Min.Y))
	gc.LineTo(float64(b.Max.X), float64(b.Max.Y))
	gc.LineTo(float64(b.Min.X), float64(b.Max.Y))
	gc.Close()
	gc.Fill()
}

func TestFillLinearGradientHorizontal(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 201, 20))
	grad := draw2d.NewLinearGradient(0, 0, 200, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 128, B: 0, A: 255}},
	})
	fillRectWithGradient(img, grad)
	// Pixel centers are at x+0.5, so the sampled parameter is (x+0.5)/200.
	// The expectation comes from an independent reference sampler.
	for _, x := range []int{0, 49, 99, 100, 150, 200} {
		want := toPremultiplied(referenceGradientColor(t, grad, (float64(x)+0.5)/200))
		if got := img.RGBAAt(x, 10); !colorClose(got, want, 2) {
			t.Errorf("x=%d (t=%.4f): got %v, want %v", x, (float64(x)+0.5)/200, got, want)
		}
	}
}

// referenceGradientColor samples the gradient with a straightforward
// premultiplied interpolation, independent from the production implementation.
func referenceGradientColor(t *testing.T, g *draw2d.LinearGradient, at float64) color.NRGBA {
	t.Helper()
	stops := append([]draw2d.GradientStop(nil), g.Stops...)
	for i := 0; i < len(stops); i++ {
		for j := i + 1; j < len(stops); j++ {
			if stops[j].Offset < stops[i].Offset {
				stops[i], stops[j] = stops[j], stops[i]
			}
		}
	}
	if at <= stops[0].Offset {
		return color.NRGBAModel.Convert(stops[0].Color).(color.NRGBA)
	}
	if at >= stops[len(stops)-1].Offset {
		return color.NRGBAModel.Convert(stops[len(stops)-1].Color).(color.NRGBA)
	}
	for i := 1; i < len(stops); i++ {
		if at <= stops[i].Offset {
			a := color.NRGBAModel.Convert(stops[i-1].Color).(color.NRGBA)
			b := color.NRGBAModel.Convert(stops[i].Color).(color.NRGBA)
			u := (at - stops[i-1].Offset) / (stops[i].Offset - stops[i-1].Offset)
			aa := float64(a.A) / 255
			ba := float64(b.A) / 255
			pa := aa + (ba-aa)*u
			if pa == 0 {
				return color.NRGBA{}
			}
			mix := func(ca, cb uint8) uint8 {
				pp := float64(ca)*aa + (float64(cb)*ba-float64(ca)*aa)*u
				return uint8(math.Round(pp / pa))
			}
			return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: uint8(math.Round(pa * 255))}
		}
	}
	return color.NRGBA{}
}

// toPremultiplied converts a straight NRGBA into the premultiplied RGBA stored
// in an *image.RGBA.
func toPremultiplied(c color.NRGBA) color.RGBA {
	return color.RGBAModel.Convert(c).(color.RGBA)
}

func colorClose(a, b color.RGBA, tol int) bool {
	diff := func(x, y uint8) bool {
		d := int(x) - int(y)
		return d <= tol && d >= -tol
	}
	return diff(a.R, b.R) && diff(a.G, b.G) && diff(a.B, b.B) && diff(a.A, b.A)
}

func TestFillLinearGradientPadOutsideAxis(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 20))
	fillRectWithGradient(img, draw2d.NewLinearGradient(25, 0, 75, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	if got := img.RGBAAt(2, 10); !colorClose(got, color.RGBA{R: 255, A: 255}, 1) {
		t.Errorf("left of axis = %v, want solid first stop", got)
	}
	if got := img.RGBAAt(98, 10); !colorClose(got, color.RGBA{B: 255, A: 255}, 1) {
		t.Errorf("right of axis = %v, want solid last stop", got)
	}
}

func TestFillLinearGradientRepeatSeam(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 10))
	g := draw2d.NewLinearGradient(0, 0, 40, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	})
	g.Extend = draw2d.ExtendRepeat
	fillRectWithGradient(img, g)
	// 2.5 periods fit across 100 px: the seam is at x=40 and x=80.
	lastOfPeriod := img.RGBAAt(39, 5)
	firstOfNext := img.RGBAAt(41, 5)
	if lastOfPeriod.B <= lastOfPeriod.R {
		t.Errorf("pixel just before seam = %v, want blue dominant", lastOfPeriod)
	}
	if firstOfNext.R <= firstOfNext.B {
		t.Errorf("pixel just after seam = %v, want red dominant", firstOfNext)
	}
	if img.RGBAAt(45, 5) != img.RGBAAt(5, 5) {
		t.Errorf("repeat should reproduce the first period exactly")
	}
}

func TestFillLinearGradientReflectSeam(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 10))
	g := draw2d.NewLinearGradient(0, 0, 40, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	})
	g.Extend = draw2d.ExtendReflect
	fillRectWithGradient(img, g)
	// Reflect is continuous across the seam: the pixel either side is still
	// blue dominant.
	for _, x := range []int{38, 39, 41, 42} {
		c := img.RGBAAt(x, 5)
		if c.B < c.R {
			t.Errorf("reflect x=%d = %v, want blue dominant (continuous seam)", x, c)
		}
	}
}

func TestFillLinearGradientRotation(t *testing.T) {
	// A vertical user space gradient on a path rotated 90 degrees must read as
	// horizontal in device space. This proves the painter maps device pixels
	// back to user space instead of sampling the device x coordinate.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	gc := NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	// Rotate about the canvas center. The rotated user space point of device
	// pixel (px, py) is obtained with the same matrix via InverseTransform, so
	// the check is anchored to the documented user space definition.
	tr := draw2d.NewTranslationMatrix(50, 50)
	tr.Compose(draw2d.NewRotationMatrix(math.Pi / 2))
	tr.Compose(draw2d.NewTranslationMatrix(-50, -50))
	gc.SetMatrixTransform(tr)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 0, 100, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	// Fill a large square in user space so it covers the canvas after rotation.
	gc.MoveTo(-50, -50)
	gc.LineTo(150, -50)
	gc.LineTo(150, 150)
	gc.LineTo(-50, 150)
	gc.Close()
	gc.Fill()

	for _, x := range []int{1, 25, 50, 75, 98} {
		ux, uy := tr.InverseTransformPoint(float64(x)+0.5, 50.5)
		at := uy / 100
		want := toPremultiplied(referenceGradientColor(t, &draw2d.LinearGradient{
			Stops: []draw2d.GradientStop{
				{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
				{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
			},
		}, at))
		if got := img.RGBAAt(x, 50); !colorClose(got, want, 2) {
			t.Errorf("x=%d user=(%.2f,%.2f) t=%.4f: got %v, want %v", x, ux, uy, at, got, want)
		}
	}
}

func TestFillLinearGradientScale(t *testing.T) {
	// A user space axis of length 10 on a path scaled 3x must span 30 device
	// pixels; a naive device-space axis of 10 would end at a third of that.
	img := image.NewRGBA(image.Rect(0, 0, 60, 10))
	gc := NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	gc.Scale(3, 1)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	gc.MoveTo(0, 0)
	gc.LineTo(20, 0)
	gc.LineTo(20, 10)
	gc.LineTo(0, 10)
	gc.Close()
	gc.Fill()
	// Device x=15 -> user x=5 -> t=0.5, so it must be roughly half blended.
	mid := img.RGBAAt(15, 5)
	if mid.R < 100 || mid.B < 100 || mid.R > 155 || mid.B > 155 {
		t.Errorf("scaled gradient mid = %v, want ~50/50 blend", mid)
	}
	// Device x=45 -> user x=15 -> t=1.5 -> clamped last stop.
	if got := img.RGBAAt(45, 5); !colorClose(got, color.RGBA{B: 255, A: 255}, 2) {
		t.Errorf("scaled gradient past axis = %v, want last stop", got)
	}
}

func TestFillLinearGradientSemiTransparentStop(t *testing.T) {
	// A fully transparent middle stop with opaque red and blue ends: the
	// result must be premultiplied (dark red/blue at half coverage), not a
	// washed out blend with black fringe.
	img := image.NewRGBA(image.Rect(0, 0, 100, 10))
	fillRectWithGradient(img, draw2d.NewLinearGradient(0, 0, 100, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{A: 0}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	mid := img.RGBAAt(50, 5)
	if mid.A > 8 {
		t.Errorf("mid alpha = %d, want ~0", mid.A)
	}
	if mid.R > 8 || mid.G > 8 || mid.B > 8 {
		t.Errorf("mid pixel = %v, want transparent", mid)
	}
	quarter := img.RGBAAt(25, 5)
	// At t=0.25, alpha is 128 and premultiplied red is 128.
	if quarter.A < 120 || quarter.A > 136 {
		t.Errorf("quarter alpha = %d, want ~128", quarter.A)
	}
	if quarter.R < 120 || quarter.G != 0 || quarter.B != 0 {
		t.Errorf("quarter pixel = %v, want premultiplied red", quarter)
	}
}

func TestFillLinearGradientZeroLengthAxis(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	fillRectWithGradient(img, draw2d.NewLinearGradient(5, 5, 5, 5, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	if got := img.RGBAAt(5, 5); !colorClose(got, color.RGBA{B: 255, A: 255}, 1) {
		t.Errorf("zero length axis = %v, want solid last stop", got)
	}
}

func TestFillLinearGradientFallsBackToSolidColor(t *testing.T) {
	solid := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	cases := map[string]*draw2d.LinearGradient{
		"no stops":        draw2d.NewLinearGradient(0, 0, 10, 0, nil),
		"non-finite axis": {X0: math.NaN(), X1: 10, Stops: []draw2d.GradientStop{{Offset: 0, Color: color.White}, {Offset: 1, Color: color.Black}}},
		"all bad offsets": draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{{Offset: math.Inf(1), Color: color.White}}),
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 10, 10))
			fillRectWithGradient(img, g)
			if got := img.RGBAAt(5, 5); got != solid {
				t.Errorf("got %v, want solid fill color %v", got, solid)
			}
		})
	}
}

func TestFillLinearGradientDoesNotAffectSolidFill(t *testing.T) {
	// Installing a gradient and then clearing it must restore the solid color
	// behavior, leaving legacy output untouched.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := NewGraphicContext(img)
	solid := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	gc.SetFillColor(solid)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	gc.SetFillColor(solid)
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.LineTo(10, 10)
	gc.LineTo(0, 10)
	gc.Close()
	gc.Fill()
	if got := img.RGBAAt(5, 5); got != color.RGBA(solid) {
		t.Errorf("got %v, want solid %v", got, solid)
	}
}

func TestFillLinearGradientSaveRestore(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := NewGraphicContext(img)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	gc.Save()
	gc.SetFillColor(color.White)
	if gc.Current.FillGradient != nil {
		t.Error("SetFillColor should clear the gradient")
	}
	gc.Restore()
	if gc.Current.FillGradient == nil {
		t.Error("Restore should bring the gradient back")
	}
}

func TestFillLinearGradientHugeCanvas(t *testing.T) {
	// A gradient axis and canvas far beyond the usual range must still be
	// sampled at the correct user space positions: every pixel lands close to
	// the midpoint of the axis, so the result is a mid blend rather than the
	// solid fallback color, with no overflow or panic.
	const size = 4096
	img := image.NewRGBA(image.Rect(0, 0, size, 16))
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	grad := draw2d.NewLinearGradient(-1e12, -1e12, 1e12, 1e12, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	})
	fillRectWithGradient(img, grad)
	for _, x := range []int{0, size / 2, size - 1} {
		got := img.RGBAAt(x, 8)
		if got == fallback {
			t.Fatalf("x=%d fell back to the solid color", x)
		}
		if got.R < 120 || got.R > 135 || got.B < 120 || got.B > 135 {
			t.Errorf("x=%d = %v, want a mid blend near {127,0,128}", x, got)
		}
	}
}

func TestFillLinearGradientFillStroke(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	gc := NewGraphicContext(img)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 20, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	gc.SetStrokeColor(color.NRGBA{G: 255, A: 255})
	gc.SetLineWidth(2)
	gc.MoveTo(2, 2)
	gc.LineTo(18, 2)
	gc.LineTo(18, 18)
	gc.LineTo(2, 18)
	gc.Close()
	gc.FillStroke()
	if got := img.RGBAAt(10, 10); got.R == 0 && got.B == 0 {
		t.Errorf("interior = %v, gradient fill was not applied", got)
	}
	// Stroke alpha 255 green should win on the edge.
	if got := img.RGBAAt(2, 2); got.G == 0 {
		t.Errorf("stroke pixel = %v, want green stroke", got)
	}
}

func TestFillLinearGradientDiagonalThreeStops(t *testing.T) {
	// A diagonal axis across a rectangle: every sampled pixel must match an
	// independent reference evaluation of the three-stop ramp, including the
	// semi-transparent middle stop.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	grad := draw2d.NewLinearGradient(0, 0, 64, 64, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 255, G: 255, B: 255, A: 96}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 100, B: 0, A: 255}},
	})
	fillRectWithGradient(img, grad)

	for y := 1; y < 64; y += 9 {
		for x := 1; x < 64; x += 9 {
			tp := (float64(x) + 0.5 + float64(y) + 0.5) / 128
			want := toPremultiplied(referenceGradientColor(t, grad, tp))
			if got := img.RGBAAt(x, y); !colorClose(got, want, 2) {
				t.Errorf("pixel (%d,%d) t=%.3f: got %v, want %v", x, y, tp, got, want)
			}
		}
	}
}

func TestFillLinearGradientRepeatSeamPixelExact(t *testing.T) {
	// Verify the repeat seam to the pixel: period of 16 px, seams at 16 and
	// 32. Each period must be an exact copy of the first one.
	img := image.NewRGBA(image.Rect(0, 0, 48, 4))
	g := draw2d.NewLinearGradient(0, 0, 16, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 255, G: 255, B: 0, A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
	})
	g.Extend = draw2d.ExtendRepeat
	fillRectWithGradient(img, g)
	for x := 0; x < 16; x++ {
		c0 := img.RGBAAt(x, 2)
		if c1 := img.RGBAAt(x+16, 2); c1 != c0 {
			t.Errorf("x=%d: period 2 = %v, period 1 = %v", x, c1, c0)
		}
		if c2 := img.RGBAAt(x+32, 2); c2 != c0 {
			t.Errorf("x=%d: period 3 = %v, period 1 = %v", x, c2, c0)
		}
	}
	// At x=15 (t=15.5/16=0.969) blue must dominate; x=16 (t=0.031) red must
	// dominate, showing the hard seam jump rather than a smooth wrap.
	if c := img.RGBAAt(15, 2); c.B <= c.R {
		t.Errorf("x=15 = %v, want blue dominant before the seam", c)
	}
	if c := img.RGBAAt(16, 2); c.R <= c.B {
		t.Errorf("x=16 = %v, want red dominant after the seam", c)
	}
}

func TestFillLinearGradientOpaqueMiddleStopNoFringe(t *testing.T) {
	// A gradient whose middle stop is fully transparent must not produce a
	// dark fringe where the alpha is intermediate: premultiplied blending
	// keeps the hue of the nearest opaque stop.
	img := image.NewRGBA(image.Rect(0, 0, 101, 4))
	fillRectWithGradient(img, draw2d.NewLinearGradient(0, 0, 100, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 0, G: 0, B: 0, A: 0}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
	}))
	for x := 0; x <= 100; x++ {
		c := img.RGBAAt(x, 2)
		if c.A == 0 {
			continue
		}
		// With a transparent black middle stop, the straight hue at any
		// non-transparent pixel is purely red or blue, never grey.
		if c.R != 0 && c.B != 0 {
			t.Errorf("x=%d = %v, dark fringe from transparent stop", x, c)
		}
	}
}

func TestFillLinearGradientExplicitPath(t *testing.T) {
	// The brush must paint paths passed explicitly to Fill, not only the
	// current path.
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	gc := NewGraphicContext(img)
	gc.SetFillColor(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 20, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	var p draw2d.Path
	p.MoveTo(0, 0)
	p.LineTo(20, 0)
	p.LineTo(20, 10)
	p.LineTo(0, 10)
	p.Close()
	gc.Fill(&p)
	left := img.RGBAAt(1, 5)
	right := img.RGBAAt(18, 5)
	if left == (color.RGBA{R: 1, G: 2, B: 3, A: 255}) || right == (color.RGBA{R: 1, G: 2, B: 3, A: 255}) {
		t.Errorf("explicit path fill fell back to solid color")
	}
	if left.R <= left.B || right.B <= right.R {
		t.Errorf("explicit path gradient orientation wrong: left %v right %v", left, right)
	}
}

func TestSetFillLinearGradientNilRestoresSolid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gc := NewGraphicContext(img)
	solid := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.White},
		{Offset: 1, Color: color.Black},
	}))
	gc.SetFillLinearGradient(nil)
	gc.SetFillColor(solid)
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.LineTo(10, 10)
	gc.LineTo(0, 10)
	gc.Close()
	gc.Fill()
	if got := img.RGBAAt(5, 5); got != color.RGBA(solid) {
		t.Errorf("got %v, want solid %v", got, solid)
	}
}

func TestFillLinearGradientCustomPainterFallback(t *testing.T) {
	// A user supplied painter must keep control: installing a gradient must
	// not bypass it.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	p := &recordingPainter{inner: raster.NewRGBAPainter(img)}
	gc := NewGraphicContextWithPainter(img, p)
	gc.SetFillColor(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.White},
		{Offset: 1, Color: color.Black},
	}))
	gc.MoveTo(0, 0)
	gc.LineTo(10, 0)
	gc.LineTo(10, 10)
	gc.LineTo(0, 10)
	gc.Close()
	gc.Fill()
	if !p.sawSpans {
		t.Fatal("custom painter was bypassed by the gradient fill")
	}
	if got := img.RGBAAt(5, 5); got != (color.RGBA{R: 1, G: 2, B: 3, A: 255}) {
		t.Errorf("custom painter should have used the solid fill color, got %v", got)
	}
}

type recordingPainter struct {
	inner    raster.Painter
	sawSpans bool
}

func (p *recordingPainter) Paint(ss []raster.Span, done bool) {
	if len(ss) > 0 {
		p.sawSpans = true
	}
	p.inner.Paint(ss, done)
}

func (p *recordingPainter) SetColor(c color.Color) {
	p.inner.(*raster.RGBAPainter).SetColor(c)
}

func TestFillLinearGradientDiagonalRepeatSeam(t *testing.T) {
	// A diagonal axis with a short period and repeat: sampling one period
	// further along the axis must reproduce the same color, and the seam must
	// be a hard jump rather than a smooth wrap.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	// Axis (0,0)->(32,32) has period 32*sqrt2 in parameter space; one period
	// further is the device offset (32, 32).
	g := draw2d.NewLinearGradient(0, 0, 32, 32, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{G: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
	})
	g.Extend = draw2d.ExtendRepeat
	fillRectWithGradient(img, g)
	for y := 2; y < 30; y += 7 {
		for x := 2; x < 30; x += 7 {
			c0 := img.RGBAAt(x, y)
			c1 := img.RGBAAt(x+32, y+32)
			if c0 != c1 {
				t.Errorf("(%d,%d)=%v and one period later (%d,%d)=%v differ", x, y, c0, x+32, y+32, c1)
			}
		}
	}
}
