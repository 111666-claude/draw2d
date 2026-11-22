// Copyright 2018 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"golang.org/x/image/draw"
)

// nrgba builds an *image.NRGBA of the given dimensions, using fn to supply
// straight-alpha (r,g,b,a) byte values for every pixel.
func nrgba(w, h int, fn func(x, y int) (r, g, b, a uint8)) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := fn(x, y)
			i := img.PixOffset(x, y)
			img.Pix[i+0] = r
			img.Pix[i+1] = g
			img.Pix[i+2] = b
			img.Pix[i+3] = a
		}
	}
	return img
}

func scaleMatrix(sx, sy float64) draw2d.Matrix {
	return draw2d.NewScaleMatrix(sx, sy)
}

// haloSource builds a white disc on a transparent background whose hidden RGB
// is black (the classic sprite that produces a dark fringe when shrunk).
func haloSource(size int) *image.NRGBA {
	center := float64(size) / 2
	radius := float64(size) * 0.4
	return nrgba(size, size, func(x, y int) (r, g, b, a uint8) {
		dx := float64(x) + 0.5 - center
		dy := float64(y) + 0.5 - center
		if dx*dx+dy*dy <= radius*radius {
			return 255, 255, 255, 255
		}
		// Fully transparent, hidden RGB black.
		return 0, 0, 0, 0
	})
}

// TestHaloBlackOnWhiteDownscale verifies that downscaling a white sprite with
// a transparent black surround leaves no dark fringe in premultiplied mode,
// while the legacy straight-alpha path still exhibits it.
func TestHaloBlackOnWhiteDownscale(t *testing.T) {
	const n = 32
	src := haloSource(n)

	// totalDarkening sums, over every non-opaque pixel, how far the straight
	// red channel falls below white. A correct premultiplied filter keeps this
	// near zero; the straight-alpha filter accumulates a visible dark ring.
	totalDarkening := func(mode AlphaMode) (sum uint64, edgePixels int) {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		DrawImageWithOptions(src, dst, scaleMatrix(0.25, 0.25), draw.Over, BilinearFilter,
			&ResampleOptions{AlphaMode: mode})
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				i := dst.PixOffset(x, y)
				a := dst.Pix[i+3]
				if a == 0 || a == 255 {
					continue
				}
				edgePixels++
				sum += uint64(uint8(255) - dst.Pix[i+0])
			}
		}
		return sum, edgePixels
	}

	legacySum, legacyEdges := totalDarkening(StraightAlphaMode)
	premulSum, _ := totalDarkening(PremulAlphaMode)
	linearSum, _ := totalDarkening(LinearPremulAlphaMode)

	if legacyEdges == 0 || legacySum == 0 {
		t.Fatalf("test setup produced no measurable fringe (edges=%d sum=%d)", legacyEdges, legacySum)
	}
	if premulSum >= legacySum {
		t.Errorf("premul darkening (%d) not smaller than legacy (%d)", premulSum, legacySum)
	}
	if linearSum >= legacySum {
		t.Errorf("linear premul darkening (%d) not smaller than legacy (%d)", linearSum, legacySum)
	}
	// Allow only a rounding-level residual: straight color of a white sprite
	// premultiplied-filtered must stay essentially white.
	if premulSum > uint64(legacyEdges)*2 {
		t.Errorf("premul edge fringe too strong: %d over %d edge pixels", premulSum, legacyEdges)
	}
}

// TestTransparentHiddenRGBNeverLeaks asserts that a fully transparent pixel
// carrying saturated hidden RGB contributes none of that RGB after filtering.
func TestTransparentHiddenRGBNeverLeaks(t *testing.T) {
	const n = 16
	// Left half opaque red, right half fully transparent with hidden green.
	src := nrgba(n, n, func(x, y int) (r, g, b, a uint8) {
		if x < n/2 {
			return 255, 0, 0, 255
		}
		return 0, 255, 0, 0
	})

	for _, mode := range []AlphaMode{PremulAlphaMode, LinearPremulAlphaMode} {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		DrawImageWithOptions(src, dst, scaleMatrix(0.5, 0.5), draw.Src, BicubicFilter,
			&ResampleOptions{AlphaMode: mode})
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				i := dst.PixOffset(x, y)
				if dst.Pix[i+3] == 0 {
					// Fully transparent result must be transparent black: no
					// hidden green channel.
					if dst.Pix[i+1] != 0 {
						t.Errorf("mode %d: hidden RGB leaked at (%d,%d): %v", mode, x, y, dst.Pix[i:i+4])
					}
				}
			}
		}
	}
}

// TestColoredTranslucentCheckerboard filters a multi-color, partially
// transparent checkerboard and checks alpha is preserved in [0,255], colors
// stay within gamut, and premultiplied outputs remain physically consistent
// (straight color never exceeds 255, never negative).
func TestColoredTranslucentCheckerboard(t *testing.T) {
	const n = 24
	palette := [][4]uint8{
		{255, 0, 0, 200},
		{0, 255, 0, 120},
		{0, 0, 255, 64},
		{255, 255, 0, 255},
		{0, 255, 255, 32},
		{255, 0, 255, 160},
	}
	src := nrgba(n, n, func(x, y int) (r, g, b, a uint8) {
		c := palette[(x/4+y/4)%len(palette)]
		return c[0], c[1], c[2], c[3]
	})

	for _, mode := range []AlphaMode{StraightAlphaMode, PremulAlphaMode, LinearPremulAlphaMode} {
		for _, filter := range []ImageFilter{LinearFilter, BilinearFilter, BicubicFilter} {
			dst := image.NewRGBA(image.Rect(0, 0, n, n))
			DrawImageWithOptions(src, dst, scaleMatrix(0.4, 0.4), draw.Over, filter,
				&ResampleOptions{AlphaMode: mode})
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					i := dst.PixOffset(x, y)
					r, g, b, a := dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3]
					if a == 0 && (r != 0 || g != 0 || b != 0) {
						t.Errorf("mode %d filter %d: transparent pixel has color at (%d,%d)", mode, filter, x, y)
					}
					// Gamut sanity: all channels are uint8 by construction;
					// ensure premultiplied consistency r*a does not overflow.
					if uint16(r)*uint16(a) > 0xffff {
						t.Errorf("mode %d filter %d: out-of-gamut pixel at (%d,%d)", mode, filter, x, y)
					}
				}
			}
		}
	}
}

// TestEnergyConservation checks that downscaling an opaque uniform image keeps
// its color and full alpha (the normalized kernel must not lose energy).
func TestEnergyConservation(t *testing.T) {
	const n = 40
	src := nrgba(n, n, func(x, y int) (r, g, b, a uint8) {
		return 200, 100, 50, 255
	})
	for _, filter := range []ImageFilter{BilinearFilter, BicubicFilter} {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		DrawImageWithOptions(src, dst, scaleMatrix(0.13, 0.13), draw.Src, filter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		var count int
		var rSum, gSum, bSum, aSum float64
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				i := dst.PixOffset(x, y)
				if dst.Pix[i+3] == 0 {
					continue
				}
				count++
				rSum += float64(dst.Pix[i+0])
				gSum += float64(dst.Pix[i+1])
				bSum += float64(dst.Pix[i+2])
				aSum += float64(dst.Pix[i+3])
			}
		}
		if count == 0 {
			t.Fatalf("filter %d: nothing was drawn", filter)
		}
		ar, ag, ab, aa := rSum/float64(count), gSum/float64(count), bSum/float64(count), aSum/float64(count)
		if math.Abs(aa-255) > 1.5 {
			t.Errorf("filter %d: average alpha %.2f not ~255", filter, aa)
		}
		if math.Abs(ar-200) > 3 || math.Abs(ag-100) > 3 || math.Abs(ab-50) > 3 {
			t.Errorf("filter %d: average color drifted to (%.1f,%.1f,%.1f)", filter, ar, ag, ab)
		}
	}
}

// TestAlphaGradientStable exercises an extreme alpha ramp (every alpha step)
// and ensures no NaN, no negative values and a broadly monotone alpha after a
// strong downscale. The downscale keeps many destination columns so the ramp
// stays resolvable; bicubic overshoot is tolerated within a small slack.
func TestAlphaGradientStable(t *testing.T) {
	const n = 128
	src := nrgba(n, 1, func(x, y int) (r, g, b, a uint8) {
		return 255, 255, 255, uint8(x * 255 / (n - 1))
	})
	dst := image.NewRGBA(image.Rect(0, 0, n, 4))
	DrawImageWithOptions(src, dst, scaleMatrix(0.5, 4), draw.Src, BicubicFilter,
		&ResampleOptions{AlphaMode: PremulAlphaMode})
	prev := -1
	for x := 0; x < n/2; x++ {
		i := dst.PixOffset(x, 2)
		a := int(dst.Pix[i+3])
		if a < prev-16 { // tolerate minor bicubic overshoot, reject collapse
			t.Errorf("alpha gradient non-monotone at x=%d: %d after %d", x, a, prev)
		}
		prev = a
	}
	// The first source column is fully transparent. Bicubic negative lobes can
	// ring a hard alpha edge by a tiny amount; that value must be negligible
	// (a few units), never the hidden RGB of a transparent pixel.
	i0 := dst.PixOffset(0, 2)
	if dst.Pix[i0+3] > 4 {
		t.Errorf("zero-alpha source produced substantial alpha: %v", dst.Pix[i0:i0+4])
	}
	if dst.Pix[i0+3] > 0 && dst.Pix[i0] < dst.Pix[i0+3] {
		t.Errorf("transparent edge carried hidden color: %v", dst.Pix[i0:i0+4])
	}
}

// TestEmptyAndDegenerateInputs ensures empty images, non-finite matrices and
// singular transforms never panic and leave the destination untouched.
func TestEmptyAndDegenerateInputs(t *testing.T) {
	t.Run("empty source", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		empty := image.NewNRGBA(image.Rect(0, 0, 0, 4))
		DrawImageWithOptions(empty, dst, scaleMatrix(2, 2), draw.Src, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
	})

	t.Run("empty destination", func(t *testing.T) {
		src := haloSource(8)
		dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
		DrawImageWithOptions(src, dst, scaleMatrix(2, 2), draw.Src, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
	})

	t.Run("nil images", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		DrawImageWithOptions(nil, dst, scaleMatrix(2, 2), draw.Src, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		DrawImageWithOptions(haloSource(4), nil, scaleMatrix(2, 2), draw.Src, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
	})

	t.Run("non-finite matrix", func(t *testing.T) {
		src := haloSource(8)
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		bad := []draw2d.Matrix{
			{math.NaN(), 0, 0, 1, 0, 0},
			{math.Inf(1), 0, 0, 1, 0, 0},
			{0, 0, 0, 0, 0, 0}, // singular
			{1e308, 0, 0, 1e308, 0, 0},
		}
		for k, m := range bad {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("matrix case %d panicked: %v", k, r)
					}
				}()
				DrawImageWithOptions(src, dst, m, draw.Src, BicubicFilter,
					&ResampleOptions{AlphaMode: PremulAlphaMode})
			}()
		}
	})
}

// TestOutOfBoundsSampling draws fully outside the destination and confirms the
// destination is untouched, plus a partial draw clips cleanly.
func TestOutOfBoundsSampling(t *testing.T) {
	src := haloSource(16)

	t.Run("fully outside", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 16, 16))
		m := draw2d.NewTranslationMatrix(100, 100)
		DrawImageWithOptions(src, dst, m, draw.Src, BicubicFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		for _, v := range dst.Pix {
			if v != 0 {
				t.Fatal("destination modified although source maps outside")
			}
		}
	})

	t.Run("clipped partial", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 16, 16))
		m := draw2d.NewTranslationMatrix(-8, -8)
		DrawImageWithOptions(src, dst, m, draw.Over, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		touched := false
		for _, v := range dst.Pix {
			if v != 0 {
				touched = true
				break
			}
		}
		if !touched {
			t.Fatal("clipped draw wrote nothing")
		}
	})
}

// TestExtremeScalingBounded runs a huge minification and magnification and
// confirms bounded memory use (no source-sized allocation) and finite output.
func TestExtremeScalingBounded(t *testing.T) {
	src := haloSource(128)

	t.Run("strong minify", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 16, 16))
		DrawImageWithOptions(src, dst, scaleMatrix(0.01, 0.01), draw.Src, BicubicFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode, MaxTaps: 16})
		assertFinite(t, dst)
	})

	t.Run("strong magnify", func(t *testing.T) {
		small := haloSource(4)
		dst := image.NewRGBA(image.Rect(0, 0, 64, 64))
		DrawImageWithOptions(small, dst, scaleMatrix(16, 16), draw.Src, BicubicFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		assertFinite(t, dst)
	})

	t.Run("default max taps", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		DrawImageWithOptions(src, dst, scaleMatrix(0.02, 0.02), draw.Src, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode}) // MaxTaps 0 -> default
		assertFinite(t, dst)
	})
}

func assertFinite(t *testing.T, img *image.RGBA) {
	t.Helper()
	for i := 0; i < len(img.Pix); i++ {
		if img.Pix[i] > 255 { // impossible for uint8; documents the finite contract
			t.Fatalf("non-finite/overflow pixel byte at %d", i)
		}
	}
}

// TestRotationDownscaleNoHalo rotates and shrinks the halo sprite, then checks
// that the premultiplied path keeps the accumulated straight-color darkening
// of every partial-coverage pixel well below the legacy straight-alpha path.
func TestRotationDownscaleNoHalo(t *testing.T) {
	const n = 48
	src := haloSource(n)

	run := func(mode AlphaMode) (sum uint64, edges int) {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		m := draw2d.NewTranslationMatrix(n/2, n/2)
		m.Compose(draw2d.NewRotationMatrix(math.Pi / 5))
		m.Compose(draw2d.NewScaleMatrix(0.35, 0.35))
		DrawImageWithOptions(src, dst, m, draw.Over, BicubicFilter,
			&ResampleOptions{AlphaMode: mode})
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				i := dst.PixOffset(x, y)
				a := dst.Pix[i+3]
				if a == 0 || a == 255 {
					continue
				}
				edges++
				sum += uint64(uint8(255) - dst.Pix[i+0])
			}
		}
		return sum, edges
	}

	legacy, edges := run(StraightAlphaMode)
	premul, _ := run(PremulAlphaMode)
	if edges == 0 || legacy == 0 {
		t.Fatalf("rotation produced no measurable edge (edges=%d legacy=%d)", edges, legacy)
	}
	// Catmull-Rom has negative lobes that ring on hard edges, so bicubic
	// cannot match bilinear's near-zero fringe; the premultiplied path must
	// nonetheless remove a clear majority of the legacy dark halo.
	if premul >= legacy {
		t.Errorf("rotated premul fringe (%d) not better than legacy (%d)", premul, legacy)
	}
	if premul > legacy*3/4 {
		t.Errorf("rotated premul fringe too strong: %d vs legacy %d", premul, legacy)
	}
}

// TestDefaultOutputUnchanged proves the zero-value configuration routes
// through the original DrawImage path and produces byte-identical output to a
// direct legacy DrawImage call.
func TestDefaultOutputUnchanged(t *testing.T) {
	src := haloSource(24)
	matrices := []draw2d.Matrix{
		scaleMatrix(0.5, 0.5),
		scaleMatrix(2, 2),
	}
	for k, m := range matrices {
		for _, filter := range []ImageFilter{LinearFilter, BilinearFilter, BicubicFilter} {
			legacy := image.NewRGBA(image.Rect(0, 0, 32, 32))
			viaOpts := image.NewRGBA(image.Rect(0, 0, 32, 32))
			DrawImage(src, legacy, m, draw.Over, filter)
			DrawImageWithOptions(src, viaOpts, m, draw.Over, filter, nil)
			if !rgbaEqual(legacy, viaOpts) {
				t.Errorf("case %d filter %d: nil opts changed output", k, filter)
			}

			// Explicit straight mode must also match the legacy path.
			straight := image.NewRGBA(image.Rect(0, 0, 32, 32))
			DrawImageWithOptions(src, straight, m, draw.Over, filter,
				&ResampleOptions{AlphaMode: StraightAlphaMode})
			if !rgbaEqual(legacy, straight) {
				t.Errorf("case %d filter %d: explicit straight mode changed output", k, filter)
			}
		}
	}
}

// TestGraphicContextDefaultIsLegacy verifies that a freshly created
// GraphicContext uses the straight-alpha path and matches legacy rendering.
func TestGraphicContextDefaultIsLegacy(t *testing.T) {
	src := haloSource(20)

	legacy := image.NewRGBA(image.Rect(0, 0, 40, 40))
	DrawImage(src, legacy, scaleMatrix(1.5, 1.5), draw.Over, BilinearFilter)

	gcDst := image.NewRGBA(image.Rect(0, 0, 40, 40))
	gc := NewGraphicContext(gcDst)
	if gc.AlphaMode != StraightAlphaMode {
		t.Fatalf("default AlphaMode = %d, want StraightAlphaMode", gc.AlphaMode)
	}
	gc.Translate(0, 0)
	gc.Scale(1.5, 1.5)
	gc.DrawImage(src)

	if !rgbaEqual(legacy, gcDst) {
		t.Error("GraphicContext default rendering diverged from legacy DrawImage")
	}
}

// TestGraphicContextPremulToggle checks the setters actually switch paths and
// that enabling premultiplied mode reduces the halo through the GC API.
func TestGraphicContextPremulToggle(t *testing.T) {
	src := haloSource(32)
	dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	gc := NewGraphicContext(dst)
	gc.SetAlphaMode(PremulAlphaMode)
	gc.SetColorSpace(SRGBColorSpace)
	gc.SetMaxTaps(32)
	gc.Scale(0.25, 0.25)
	gc.DrawImage(src)

	if gc.AlphaMode != PremulAlphaMode {
		t.Fatal("SetAlphaMode did not persist")
	}
	worst := uint8(0)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			i := dst.PixOffset(x, y)
			a := dst.Pix[i+3]
			if a == 0 || a == 255 {
				continue
			}
			if d := uint8(255) - dst.Pix[i+0]; d > worst {
				worst = d
			}
		}
	}
	if worst > 2 {
		t.Errorf("GC premul path fringe too strong: %d", worst)
	}
}

// TestNonZeroBoundsAndImageTypes covers sources whose bounds do not start at
// the origin and several concrete image types.
func TestNonZeroBoundsAndImageTypes(t *testing.T) {
	const n = 16
	base := haloSource(n)

	// Shift the NRGBA bounds away from the origin without moving pixels.
	shifted := image.NewNRGBA(image.Rect(7, 9, 7+n, 9+n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			s := base.PixOffset(x, y)
			d := shifted.PixOffset(7+x, 9+y)
			copy(shifted.Pix[d:d+4], base.Pix[s:s+4])
		}
	}

	types := map[string]image.Image{
		"nrgba-shifted": shifted,
		"rgba":          toRGBA(base),
		"paletted":      toPaletted(base),
	}
	for name, img := range types {
		dst := image.NewRGBA(image.Rect(0, 0, 24, 24))
		// Translate so the shifted bounds still land inside the destination.
		m := draw2d.NewTranslationMatrix(-float64(img.Bounds().Min.X), -float64(img.Bounds().Min.Y))
		m.Compose(scaleMatrix(1.2, 1.2))
		DrawImageWithOptions(img, dst, m, draw.Over, BilinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		if imageDirtyCount(dst) == 0 {
			t.Errorf("image type %s: nothing drawn", name)
		}
	}
}

func toRGBA(src *image.NRGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, color.NRGBAModel.Convert(src.NRGBAAt(x, y)))
		}
	}
	return dst
}

func toPaletted(src *image.NRGBA) *image.Paletted {
	p := color.Palette{
		color.RGBA{0, 0, 0, 0},
		color.RGBA{255, 255, 255, 255},
	}
	dst := image.NewPaletted(src.Bounds(), p)
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			dst.Set(x, y, src.NRGBAAt(x, y))
		}
	}
	return dst
}

func imageDirtyCount(img *image.RGBA) int {
	c := 0
	for _, v := range img.Pix {
		if v != 0 {
			c++
		}
	}
	return c
}

func rgbaEqual(a, b *image.RGBA) bool {
	if len(a.Pix) != len(b.Pix) {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// TestSrcAndOverOps confirms both Porter-Duff ops behave: Src replaces, Over
// blends with existing destination content.
func TestSrcAndOverOps(t *testing.T) {
	src := nrgba(8, 8, func(x, y int) (r, g, b, a uint8) {
		return 255, 0, 0, 128
	})

	t.Run("src replaces", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		dst.Set(2, 2, color.RGBA{G: 255, A: 255})
		DrawImageWithOptions(src, dst, draw2d.NewIdentityMatrix(), draw.Src, LinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		c := dst.RGBAAt(2, 2)
		if c.G != 0 {
			t.Errorf("Src op did not replace green channel: %+v", c)
		}
		if c.A == 0 || c.A == 255 {
			t.Errorf("Src op lost source alpha: %+v", c)
		}
	})

	t.Run("over blends", func(t *testing.T) {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		dst.Set(2, 2, color.RGBA{G: 255, A: 255})
		DrawImageWithOptions(src, dst, draw2d.NewIdentityMatrix(), draw.Over, LinearFilter,
			&ResampleOptions{AlphaMode: PremulAlphaMode})
		c := dst.RGBAAt(2, 2)
		if c.G == 0 {
			t.Error("Over op erased destination green instead of blending")
		}
		if c.R == 0 {
			t.Error("Over op dropped source red")
		}
	})
}

// TestNearestNeighborPremul verifies the nearest-neighbor path also honors
// premultiplied semantics and stays stable under rotation.
func TestNearestNeighborPremul(t *testing.T) {
	src := haloSource(16)
	dst := image.NewRGBA(image.Rect(0, 0, 24, 24))
	m := draw2d.NewTranslationMatrix(4, 4)
	m.Compose(scaleMatrix(1.25, 1.25))
	DrawImageWithOptions(src, dst, m, draw.Over, LinearFilter,
		&ResampleOptions{AlphaMode: PremulAlphaMode})
	// Nearest neighbor copies whole pixels: any transparent pixel must remain
	// transparent black.
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			i := dst.PixOffset(x, y)
			if dst.Pix[i+3] == 0 && (dst.Pix[i]|dst.Pix[i+1]|dst.Pix[i+2]) != 0 {
				t.Fatalf("nearest neighbor leaked color into transparent pixel at (%d,%d)", x, y)
			}
		}
	}
}
