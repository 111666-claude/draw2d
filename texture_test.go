// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// checkerImage builds a 2x2 texture with distinct, fully opaque corners. With
// bilinear filtering the centres of the four texels are pure corner colors,
// which makes phase and wrap errors easy to assert on.
func checkerImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	return img
}

// offsetCheckerImage is checkerImage translated so that its non-zero Bounds.Min
// exercises the phase handling.
func offsetCheckerImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(10, 20, 12, 22))
	img.SetRGBA(10, 20, color.RGBA{R: 255, A: 255})
	img.SetRGBA(11, 20, color.RGBA{G: 255, A: 255})
	img.SetRGBA(10, 21, color.RGBA{B: 255, A: 255})
	img.SetRGBA(11, 21, color.RGBA{R: 255, G: 255, A: 255})
	return img
}

func TestTextureCommons(t *testing.T) {
	img := checkerImage()
	if !NewImageTexture(img, Matrix{}, TextureRepeatBoth).Usable() {
		t.Fatal("plain checker should be usable")
	}

	for _, tc := range []struct {
		name string
		tex  *ImageTexture
	}{
		{"nil texture", nil},
		{"nil image", NewImageTexture(nil, Matrix{}, TextureClamp)},
		{"empty image", NewImageTexture(image.NewRGBA(image.Rect(0, 0, 0, 0)), Matrix{}, TextureClamp)},
		{"nan transform", NewImageTexture(img, Matrix{math.NaN(), 0, 0, 1, 0, 0}, TextureClamp)},
		{"inf transform", NewImageTexture(img, Matrix{1, 0, 0, math.Inf(1), 0, 0}, TextureClamp)},
		{"singular transform", NewImageTexture(img, Matrix{1, 2, 2, 4, 0, 0}, TextureClamp)},
	} {
		if tc.tex.Usable() {
			t.Errorf("%s: Usable() = true, want false", tc.name)
		}
		if s := NewTextureSampler(tc.tex); s != nil {
			t.Errorf("%s: NewTextureSampler = %v, want nil", tc.name, s)
		}
	}
}

func TestTextureZeroMatrixIsIdentity(t *testing.T) {
	tex := &ImageTexture{Image: checkerImage()}
	if got := tex.LocalTransform(); got != NewIdentityMatrix() {
		t.Errorf("zero matrix LocalTransform = %v, want identity", got)
	}
	norm := tex.Normalize()
	if norm.LocalTransform() != NewIdentityMatrix() {
		t.Errorf("Normalize should replace the zero matrix with identity")
	}
	if norm.Repeat != TextureClamp {
		t.Errorf("Normalize repeat = %v, want clamp", norm.Repeat)
	}
	// Normalize must not alias the caller's matrix storage.
	tex.Transform = Matrix{2, 0, 0, 2, 3, 4}
	if norm.Transform == tex.Transform {
		t.Error("Normalize aliased the caller transform")
	}
}

func TestTextureInvalidRepeatFallsBackToClamp(t *testing.T) {
	tex := NewImageTexture(checkerImage(), Matrix{}, TextureRepeat(99))
	if got := tex.Normalize().Repeat; got != TextureClamp {
		t.Errorf("invalid repeat normalized to %v, want clamp", got)
	}
	if TextureRepeat(99).Valid() {
		t.Error("99 should not be valid")
	}
	if TextureRepeatBoth.String() != "repeat" || TextureRepeatX.String() != "repeat-x" ||
		TextureRepeatY.String() != "repeat-y" || TextureClamp.String() != "clamp" {
		t.Error("unexpected repeat string names")
	}
}

func TestTextureSampleTileCenters(t *testing.T) {
	// Pixel centres of the 2x2 tile are at half-integer coordinates.
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatBoth))
	if s == nil {
		t.Fatal("nil sampler")
	}
	for _, tc := range []struct {
		x, y float64
		want color.NRGBA
	}{
		{0.5, 0.5, color.NRGBA{R: 255, A: 255}},
		{1.5, 0.5, color.NRGBA{G: 255, A: 255}},
		{0.5, 1.5, color.NRGBA{B: 255, A: 255}},
		{1.5, 1.5, color.NRGBA{R: 255, G: 255, A: 255}},
	} {
		if got := s.Sample(tc.x, tc.y); got != tc.want {
			t.Errorf("Sample(%.1f, %.1f) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestTextureSampleNonZeroBoundsPhase(t *testing.T) {
	s := NewTextureSampler(NewImageTexture(offsetCheckerImage(), Matrix{}, TextureRepeatBoth))
	if s == nil {
		t.Fatal("nil sampler")
	}
	// The same texel colours must appear at the shifted pixel centres.
	for _, tc := range []struct {
		x, y float64
		want color.NRGBA
	}{
		{10.5, 20.5, color.NRGBA{R: 255, A: 255}},
		{11.5, 20.5, color.NRGBA{G: 255, A: 255}},
		{10.5, 21.5, color.NRGBA{B: 255, A: 255}},
		{11.5, 21.5, color.NRGBA{R: 255, G: 255, A: 255}},
	} {
		if got := s.Sample(tc.x, tc.y); got != tc.want {
			t.Errorf("Sample(%.1f, %.1f) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	// One tile later the pattern repeats, so the colour at Bounds.Min is the
	// same as one tile width/height away.
	if got, want := s.Sample(12.5, 22.5), s.Sample(10.5, 20.5); got != want {
		t.Errorf("wrap sample = %v, want %v", got, want)
	}
}

func TestTextureSampleSeamIsContinuous(t *testing.T) {
	// Two different corner colours meeting at the tile seam must interpolate
	// across the seam instead of jumping to transparent. On the repeated X
	// axis, x = 2.0 sits between the rightmost (G) and the wrapped (R) texel.
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatX))
	mid := s.Sample(2.0, 0.5)
	if mid.A == 0 {
		t.Fatalf("seam sample is transparent: %v", mid)
	}
	if mid.R == 0 || mid.G == 0 {
		t.Errorf("seam sample %v does not blend both sides of the seam", mid)
	}
	// It must be the average of the two neighbouring centres.
	left := s.Sample(1.5, 0.5)
	right := s.Sample(2.5, 0.5) // wrapped: heals to the R texel
	for _, pair := range [][2]uint8{{mid.R, avg(left.R, right.R)}, {mid.G, avg(left.G, right.G)}} {
		if d := int(pair[0]) - int(pair[1]); d > 2 || d < -2 {
			t.Errorf("seam sample %v is not the blend of %v and %v", mid, left, right)
		}
	}
}

func avg(a, b uint8) uint8 { return uint8((int(a) + int(b) + 1) / 2) }

func TestTextureSampleClampOutsideIsTransparent(t *testing.T) {
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureClamp))
	for _, tc := range []struct{ x, y float64 }{
		{-1, 0.5}, {2.5, 0.5}, {0.5, -1}, {0.5, 2.5}, {100, 100}, {-100, 0.5},
	} {
		if got := s.Sample(tc.x, tc.y); got.A != 0 {
			t.Errorf("Sample(%.1f, %.1f) = %v, want transparent", tc.x, tc.y, got)
		}
	}
	// Just inside the edge remains opaque.
	if got := s.Sample(0.4, 0.5); got.A == 0 {
		t.Errorf("sample just inside edge is transparent")
	}
}

func TestTextureSampleRepeatAxesLimited(t *testing.T) {
	// RepeatX wraps horizontally but stays transparent above and below.
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatX))
	if got := s.Sample(100.5, 0.5); got.A == 0 {
		t.Errorf("repeat-x should wrap along x: %v", got)
	}
	if got := s.Sample(0.5, 100.5); got.A != 0 {
		t.Errorf("repeat-x should not wrap along y: %v", got)
	}

	// RepeatY is the mirror image.
	s = NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatY))
	if got := s.Sample(0.5, 100.5); got.A == 0 {
		t.Errorf("repeat-y should wrap along y: %v", got)
	}
	if got := s.Sample(100.5, 0.5); got.A != 0 {
		t.Errorf("repeat-y should not wrap along x: %v", got)
	}
}

func TestTextureSampleNegativeCoordinates(t *testing.T) {
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatBoth))
	// -0.5 maps to image pixel x = -1.0, which is the texel at 1.5 wrapped.
	if got, want := s.Sample(-0.5, 0.5), s.Sample(1.5, 0.5); got != want {
		t.Errorf("Sample(-0.5, 0.5) = %v, want %v", got, want)
	}
	// Far negative coordinates still wrap instead of overflowing.
	if got := s.Sample(-1e12+0.5, 0.5); got.A == 0 {
		t.Errorf("large negative coordinate should wrap, got %v", got)
	}
}

func TestTextureSampleNonFiniteCoordinate(t *testing.T) {
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatBoth))
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := s.Sample(v, 0.5); got.A != 0 {
			t.Errorf("Sample(%v, 0.5) = %v, want transparent", v, got)
		}
		if got := s.Sample(0.5, v); got.A != 0 {
			t.Errorf("Sample(0.5, %v) = %v, want transparent", v, got)
		}
	}
}

func TestTextureSamplePremultipliedAlpha(t *testing.T) {
	// A half transparent red edge next to a transparent texel must filter to a
	// half transparent red, not a dark fringe.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	img.SetNRGBA(1, 0, color.NRGBA{})
	s := NewTextureSampler(NewImageTexture(img, Matrix{}, TextureRepeatBoth))
	if s == nil {
		t.Fatal("nil sampler")
	}
	got := s.Sample(1.0, 0.5) // exactly between the two texel centres
	if got.A == 0 {
		t.Fatalf("seam sample is transparent")
	}
	if got.R != 255 {
		t.Errorf("premultiplied filtering changed hue: %v", got)
	}
	if got.A > 70 || got.A < 56 {
		t.Errorf("alpha = %d, want about half of 128", got.A)
	}
}

func TestTextureSamplePremultipliedOutput(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	s := NewTextureSampler(NewImageTexture(img, Matrix{}, TextureClamp))
	r, g, b, a := s.SamplePremultiplied(0.5, 0.5)
	if a != 128 {
		t.Fatalf("alpha = %d, want 128", a)
	}
	// Premultiplied channels must be <= alpha and roughly proportional.
	for _, c := range []uint8{r, g, b} {
		if c > a {
			t.Errorf("premultiplied channel %d exceeds alpha %d", c, a)
		}
	}
	if r < 95 || r > 105 || g < 45 || g > 55 || b < 20 || b > 30 {
		t.Errorf("unexpected premultiplied sample (%d,%d,%d,%d)", r, g, b, a)
	}
}

func TestTextureSamplerBoundsAndRepeat(t *testing.T) {
	var nilSampler *TextureSampler
	if nilSampler.Bounds() != (image.Rectangle{}) {
		t.Error("nil sampler bounds")
	}
	if nilSampler.Repeat() != TextureClamp {
		t.Error("nil sampler repeat")
	}
	s := NewTextureSampler(NewImageTexture(offsetCheckerImage(), Matrix{}, TextureRepeatY))
	if got := s.Bounds(); got != image.Rect(10, 20, 12, 22) {
		t.Errorf("bounds = %v", got)
	}
	if s.Repeat() != TextureRepeatY {
		t.Errorf("repeat = %v", s.Repeat())
	}
}

func TestTextureNearestSampling(t *testing.T) {
	// At a fractional position bilinear blends the two neighbouring texels,
	// while nearest returns exactly one of them.
	img := checkerImage()
	nearest := NewTextureSampler(NewImageTextureWithSampling(img, Matrix{}, TextureRepeatBoth, TextureSamplingNearest))
	bilinear := NewTextureSampler(NewImageTexture(img, Matrix{}, TextureRepeatBoth))
	if nearest == nil {
		t.Fatal("nil nearest sampler")
	}
	if got := nearest.Sampling(); got != TextureSamplingNearest {
		t.Errorf("sampling = %v", got)
	}
	// The point (1.0, 0.5) is equidistant from the R (x=0.5) and G (x=1.5)
	// texel centres; bilinear mixes them, nearest picks a single texel.
	mixed := bilinear.Sample(1.0, 0.5)
	single := nearest.Sample(1.0, 0.5)
	if mixed.R == 0 || mixed.G == 0 {
		t.Fatalf("bilinear should mix both texels, got %v", mixed)
	}
	if !(single == (color.NRGBA{R: 255, A: 255}) || single == (color.NRGBA{G: 255, A: 255})) {
		t.Errorf("nearest sample = %v, want one of the pure texels", single)
	}
	// Pixel centres must still match for both filters.
	for _, tc := range []struct {
		x, y float64
		want color.NRGBA
	}{
		{0.5, 0.5, color.NRGBA{R: 255, A: 255}},
		{1.5, 1.5, color.NRGBA{R: 255, G: 255, A: 255}},
	} {
		if got := nearest.Sample(tc.x, tc.y); got != tc.want {
			t.Errorf("nearest Sample(%v,%v) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestTextureSamplingInvalidFallsBackToBilinear(t *testing.T) {
	tex := NewImageTextureWithSampling(checkerImage(), Matrix{}, TextureRepeatBoth, TextureSampling(99))
	if got := tex.Normalize().Sampling; got != TextureSamplingBilinear {
		t.Errorf("invalid sampling normalized to %v, want bilinear", got)
	}
	if TextureSampling(99).Valid() {
		t.Error("99 should not be a valid sampling mode")
	}
	if TextureSamplingNearest.String() != "nearest" || TextureSamplingBilinear.String() != "bilinear" {
		t.Error("unexpected sampling string names")
	}
}

func TestTextureNearestClampOutsideTransparent(t *testing.T) {
	s := NewTextureSampler(NewImageTextureWithSampling(checkerImage(), Matrix{}, TextureClamp, TextureSamplingNearest))
	if got := s.Sample(5.5, 0.5); got.A != 0 {
		t.Errorf("nearest clamp outside = %v, want transparent", got)
	}
}

func TestTextureHugeRepeatRange(t *testing.T) {
	// A tiny texture sampled over a huge repeated range must stay cheap and
	// finite: the wrap happens in float space, not by enumerating tiles.
	s := NewTextureSampler(NewImageTexture(checkerImage(), Matrix{}, TextureRepeatBoth))
	const far = 1e9
	if got, want := s.Sample(far+0.5, far+0.5), s.Sample(0.5, 0.5); got != want {
		t.Errorf("far sample = %v, want %v", got, want)
	}
}
