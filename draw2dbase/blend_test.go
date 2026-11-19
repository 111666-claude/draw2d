// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

// premul builds a premultiplied pixel from straight RGBA values.
func premul(r, g, b, a uint8) [4]uint8 {
	if a == 0 {
		return [4]uint8{}
	}
	scale := func(v uint8) uint8 { return uint8(uint32(v) * uint32(a) / 255) }
	return [4]uint8{scale(r), scale(g), scale(b), a}
}

func TestBlendPremultipliedNormal(t *testing.T) {
	// Opaque red over opaque green gives red.
	got := BlendPremultiplied(premul(0, 255, 0, 255), premul(255, 0, 0, 255), draw2d.BlendNormal, 1)
	if got != premul(255, 0, 0, 255) {
		t.Errorf("normal opaque red over green = %v, want red", got)
	}
	// 50% red over opaque green: r=128 (approx), g=127 (approx)
	dst := premul(0, 255, 0, 255)
	src := premul(255, 0, 0, 128)
	got = BlendPremultiplied(dst, src, draw2d.BlendNormal, 1)
	if got[0] < 120 || got[0] > 136 {
		t.Errorf("normal translucent red: R = %d, want ~128", got[0])
	}
	if got[1] < 120 || got[1] > 136 {
		t.Errorf("normal translucent red: G = %d, want ~127", got[1])
	}
	if got[3] != 255 {
		t.Errorf("normal translucent red: A = %d, want 255", got[3])
	}
}

func TestBlendPremultipliedTransparentSource(t *testing.T) {
	dst := premul(10, 20, 30, 255)
	for _, mode := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen, draw2d.BlendMode(9)} {
		if got := BlendPremultiplied(dst, [4]uint8{}, mode, 1); got != dst {
			t.Errorf("%v: transparent source changed destination: %v", mode, got)
		}
	}
}

func TestBlendPremultipliedMultiply(t *testing.T) {
	// Red * green = black (opaque).
	dst := premul(0, 255, 0, 255)
	src := premul(255, 0, 0, 255)
	got := BlendPremultiplied(dst, src, draw2d.BlendMultiply, 1)
	if got != premul(0, 0, 0, 255) {
		t.Errorf("multiply opaque red x green = %v, want black", got)
	}
	// White is the identity for multiply.
	got = BlendPremultiplied(dst, premul(255, 255, 255, 255), draw2d.BlendMultiply, 1)
	if got != dst {
		t.Errorf("multiply white = %v, want %v", got, dst)
	}
	// 50% opaque red over opaque green: csr=0.5, csg=csb=0, as=0.5, cb=(0,1,0)
	// r = csr*cbr = 0, g = (1-as)*cbg = 0.5, so (0,128,0,255)
	got = BlendPremultiplied(dst, premul(255, 0, 0, 128), draw2d.BlendMultiply, 1)
	if got[0] != 0 {
		t.Errorf("multiply translucent red: R = %d, want 0", got[0])
	}
	if got[1] < 124 || got[1] > 132 {
		t.Errorf("multiply translucent red: G = %d, want ~127", got[1])
	}
	if got[3] != 255 {
		t.Errorf("multiply translucent red: A = %d, want 255", got[3])
	}
}

func TestBlendPremultipliedScreen(t *testing.T) {
	dst := premul(0, 0, 255, 255) // opaque blue
	// Black is the identity for screen.
	got := BlendPremultiplied(dst, premul(0, 0, 0, 255), draw2d.BlendScreen, 1)
	if got != dst {
		t.Errorf("screen black = %v, want %v", got, dst)
	}
	// White saturates.
	got = BlendPremultiplied(dst, premul(255, 255, 255, 255), draw2d.BlendScreen, 1)
	if got != premul(255, 255, 255, 255) {
		t.Errorf("screen white = %v, want white", got)
	}
	// Opaque red over opaque green: r=1, g=1, b=0.
	got = BlendPremultiplied(premul(0, 255, 0, 255), premul(255, 0, 0, 255), draw2d.BlendScreen, 1)
	if got != premul(255, 255, 0, 255) {
		t.Errorf("screen red over green = %v, want yellow", got)
	}
}

func TestBlendPremultipliedTransparentBackdrop(t *testing.T) {
	// On a transparent backdrop, multiply and screen reduce to normal source
	// compositing: (1-ab)=1 and cb=0, so co = cs.
	src := premul(255, 0, 0, 128)
	for _, mode := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen} {
		got := BlendPremultiplied([4]uint8{}, src, mode, 1)
		if got != src {
			t.Errorf("%v on transparent backdrop = %v, want %v", mode, got, src)
		}
	}
	// Multiply over an opaque green backdrop darkens it to black instead of
	// painting red over it, which is the difference from normal compositing.
	green := premul(0, 255, 0, 255)
	red := premul(255, 0, 0, 255)
	multiplied := BlendPremultiplied(green, red, draw2d.BlendMultiply, 1)
	if multiplied != premul(0, 0, 0, 255) {
		t.Errorf("multiply red over green = %v, want black", multiplied)
	}
	over := BlendPremultiplied(green, red, draw2d.BlendNormal, 1)
	if over != red {
		t.Errorf("normal red over green = %v, want red", over)
	}
}

func TestBlendPremultipliedOpacity(t *testing.T) {
	dst := [4]uint8{}
	src := premul(255, 0, 0, 255)
	got := BlendPremultiplied(dst, src, draw2d.BlendNormal, 0.5)
	if got[3] < 126 || got[3] > 130 {
		t.Errorf("opacity 0.5: A = %d, want ~128", got[3])
	}
	if got[0] < 126 || got[0] > 130 {
		t.Errorf("opacity 0.5: R = %d, want ~128", got[0])
	}
	// Zero opacity never changes the destination.
	if got := BlendPremultiplied(premul(1, 2, 3, 255), src, draw2d.BlendMultiply, 0); got != premul(1, 2, 3, 255) {
		t.Errorf("opacity 0 changed destination: %v", got)
	}
}

func TestBlendPremultipliedInvariant(t *testing.T) {
	// Every produced channel must be <= alpha, for all modes, alphas and
	// opacities. Exhaustive over a coarse grid.
	for _, mode := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen} {
		for da := 0; da <= 255; da += 17 {
			for sa := 0; sa <= 255; sa += 17 {
				for c := 0; c <= 255; c += 51 {
					dst := premul(uint8(c), uint8(255-c), 128, uint8(da))
					src := premul(200, uint8(c), 240, uint8(sa))
					for _, op := range []float64{0, 0.25, 0.5, 1} {
						out := BlendPremultiplied(dst, src, mode, op)
						if out[0] > out[3] || out[1] > out[3] || out[2] > out[3] {
							t.Fatalf("mode %v, da=%d sa=%d c=%d op=%v: non-premultiplied output %v", mode, da, sa, c, op, out)
						}
					}
				}
			}
		}
	}
}

func TestBlendPremultipliedUnknownModeFallsBackToNormal(t *testing.T) {
	dst := premul(0, 255, 0, 255)
	src := premul(255, 0, 0, 128)
	unknown := BlendPremultiplied(dst, src, draw2d.BlendMode(77), 1)
	normal := BlendPremultiplied(dst, src, draw2d.BlendNormal, 1)
	if unknown != normal {
		t.Errorf("unknown mode = %v, want normal %v", unknown, normal)
	}
}

func TestCompositeRGBA(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 3))
	// Backdrop: opaque green.
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = 0, 255, 0, 255
	}
	src := image.NewRGBA(image.Rect(1, 1, 3, 3))
	// Opaque red in the source rectangle.
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+3] = 255, 255
	}
	Composite(dst, src, draw2d.BlendMultiply, 1)
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("pixel outside source changed: %v", got)
	}
	if got := dst.RGBAAt(2, 2); got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("multiplied pixel = %v, want black", got)
	}
}

// srcIntoDst builds a source image whose bounds share the destination origin,
// as the backends do, so that pixel (x, y) of src lands on pixel (x, y) of dst.
func redTile(bounds image.Rectangle) *image.RGBA {
	src := image.NewRGBA(bounds)
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+3] = 255, 255
	}
	return src
}

func TestCompositeBoundsMismatch(t *testing.T) {
	// The source may be smaller than the destination; only the overlap
	// changes and matching coordinates line up because bounds are aligned.
	dst := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i+1], dst.Pix[i+3] = 255, 255
	}
	src := redTile(image.Rect(0, 0, 5, 5))
	Composite(dst, src, draw2d.BlendNormal, 1)
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("dst(0,0) = %v, want red", got)
	}
	if got := dst.RGBAAt(4, 4); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("dst(4,4) = %v, want red", got)
	}
	if got := dst.RGBAAt(5, 5); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("dst(5,5) = %v, want green (unchanged)", got)
	}
}

func TestCompositeMismatchedOriginsClip(t *testing.T) {
	// A source with a different origin is clipped to the shared coordinate
	// space, matching image/draw's dst-space semantics of the backends.
	dst := image.NewRGBA(image.Rect(10, 10, 20, 20))
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i+1], dst.Pix[i+3] = 255, 255
	}
	src := redTile(image.Rect(12, 12, 14, 14))
	Composite(dst, src, draw2d.BlendNormal, 1)
	if got := dst.RGBAAt(13, 13); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("dst(13,13) = %v, want red", got)
	}
	if got := dst.RGBAAt(10, 10); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("dst(10,10) = %v, want green (unchanged)", got)
	}
}

func TestCompositeNoOps(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+3] = 255, 255
	}
	Composite(nil, src, draw2d.BlendNormal, 1)
	Composite(dst, nil, draw2d.BlendNormal, 1)
	Composite(dst, src, draw2d.BlendNormal, 0)
	Composite(dst, src, draw2d.BlendNormal, math.NaN())
	Composite(dst, src, draw2d.BlendNormal, -1)
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{}) {
		t.Errorf("no-op composite changed destination: %v", got)
	}
	// Disjoint bounds are a no-op.
	far := image.NewRGBA(image.Rect(100, 100, 102, 102))
	Composite(dst, far, draw2d.BlendNormal, 1)
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{}) {
		t.Errorf("disjoint composite changed destination: %v", got)
	}
}

func TestCompositeGenericDestination(t *testing.T) {
	// NRGBA is not *image.RGBA, exercising the generic path.
	dst := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 255})
		}
	}
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			src.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	Composite(dst, src, draw2d.BlendMultiply, 1)
	got := dst.NRGBAAt(0, 0)
	if got.A != 255 || got.R != 0 || got.G != 0 || got.B != 0 {
		t.Errorf("generic multiply destination = %v, want opaque black", got)
	}
}
