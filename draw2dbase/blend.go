// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/llgcode/draw2d"
)

// BlendPremultiplied combines one source pixel with one destination pixel
// using the given blend mode and a group opacity in [0, 1].
//
// Both pixels are alpha-premultiplied 8-bit RGBA values, matching the layout
// of image.RGBA and color.RGBA. The returned pixel is alpha-premultiplied and
// always satisfies channel <= alpha. A source alpha of zero leaves the
// destination untouched for every mode, and an unknown mode falls back to
// normal compositing.
//
// The formulas are the W3C Compositing and Blending ones, expressed with
// premultiplied components (cs, cb) and alpha values (as, ab):
//
//	normal:   co = cs + cb*(1-as)
//	multiply: co = (1-ab)*cs + cs*cb + (1-as)*cb
//	screen:   co = cs + cb - cs*cb
//	ao        = as + ab*(1-as)
func BlendPremultiplied(dst, src [4]uint8, mode draw2d.BlendMode, opacity float64) [4]uint8 {
	if src[3] == 0 || opacity <= 0 {
		return dst
	}
	if opacity > 1 {
		opacity = 1
	}
	as := float64(src[3]) / 255 * opacity
	ab := float64(dst[3]) / 255
	csr := float64(src[0]) / 255 * opacity
	csg := float64(src[1]) / 255 * opacity
	csb := float64(src[2]) / 255 * opacity
	cbr := float64(dst[0]) / 255
	cbg := float64(dst[1]) / 255
	cbb := float64(dst[2]) / 255

	var or, og, ob float64
	switch mode {
	case draw2d.BlendMultiply:
		or = (1-ab)*csr + csr*cbr + (1-as)*cbr
		og = (1-ab)*csg + csg*cbg + (1-as)*cbg
		ob = (1-ab)*csb + csb*cbb + (1-as)*cbb
	case draw2d.BlendScreen:
		or = csr + cbr - csr*cbr
		og = csg + cbg - csg*cbg
		ob = csb + cbb - csb*cbb
	default: // draw2d.BlendNormal and any unknown mode
		or = csr + cbr*(1-as)
		og = csg + cbg*(1-as)
		ob = csb + cbb*(1-as)
	}

	ao := as + ab*(1-as)
	return [4]uint8{
		premultChannel(or, ao),
		premultChannel(og, ao),
		premultChannel(ob, ao),
		premultChannel(ao, 1),
	}
}

// premultChannel converts a premultiplied component in [0, 1] to 8 bits,
// clamping it to [0, alpha] so that the premultiplied invariant holds after
// rounding.
func premultChannel(v, alpha float64) uint8 {
	if v < 0 {
		v = 0
	}
	if v > alpha {
		v = alpha
	}
	if v > 1 {
		v = 1
	}
	return uint8(v*255 + 0.5)
}

// Composite composites src onto dst with the given blend mode and group
// opacity. src must be alpha-premultiplied (*image.RGBA). dst may be any
// draw.Image; the fast path is used when it is an *image.RGBA. Only the
// intersection of the two bounds is touched.
//
// A nil image, an empty intersection and a non-positive opacity are no-ops.
// Finite opacities above 1 are clamped to 1; non-finite opacities are treated
// as a no-op so that a bad caller cannot corrupt the destination.
func Composite(dst draw.Image, src *image.RGBA, mode draw2d.BlendMode, opacity float64) {
	if dst == nil || src == nil {
		return
	}
	CompositeRect(dst, src, src.Bounds(), mode, opacity)
}

// CompositeRect is Composite limited to the rectangle r, which is clipped
// against both images. The layer backend uses it to composite only the part of
// an offscreen surface that was actually painted.
func CompositeRect(dst draw.Image, src *image.RGBA, r image.Rectangle, mode draw2d.BlendMode, opacity float64) {
	if dst == nil || src == nil || opacity <= 0 || opacity != opacity {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	r = r.Intersect(dst.Bounds()).Intersect(src.Bounds())
	if r.Empty() {
		return
	}
	if rgba, ok := dst.(*image.RGBA); ok {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			si := src.PixOffset(r.Min.X, y)
			di := rgba.PixOffset(r.Min.X, y)
			for x := r.Min.X; x < r.Max.X; x++ {
				s := [4]uint8{src.Pix[si], src.Pix[si+1], src.Pix[si+2], src.Pix[si+3]}
				if s[3] != 0 {
					d := [4]uint8{rgba.Pix[di], rgba.Pix[di+1], rgba.Pix[di+2], rgba.Pix[di+3]}
					out := BlendPremultiplied(d, s, mode, opacity)
					rgba.Pix[di], rgba.Pix[di+1], rgba.Pix[di+2], rgba.Pix[di+3] = out[0], out[1], out[2], out[3]
				}
				si += 4
				di += 4
			}
		}
		return
	}
	// Generic destination: read through color.Color, which is documented to
	// return alpha-premultiplied values.
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sc := src.RGBAAt(x, y)
			if sc.A == 0 {
				continue
			}
			dr, dg, db, da := dst.At(x, y).RGBA()
			d := [4]uint8{uint8(dr >> 8), uint8(dg >> 8), uint8(db >> 8), uint8(da >> 8)}
			s := [4]uint8{sc.R, sc.G, sc.B, sc.A}
			out := BlendPremultiplied(d, s, mode, opacity)
			dst.Set(x, y, color.RGBA{R: out[0], G: out[1], B: out[2], A: out[3]})
		}
	}
}

// BlendColor is the color produced by compositing src over dst with mode and
// opacity. It is exposed for tests and for backends that need the result as a
// color value.
func BlendColor(dst, src color.Color, mode draw2d.BlendMode, opacity float64) color.RGBA {
	dr, dg, db, da := dst.RGBA()
	sr, sg, sb, sa := src.RGBA()
	d := [4]uint8{uint8(dr >> 8), uint8(dg >> 8), uint8(db >> 8), uint8(da >> 8)}
	s := [4]uint8{uint8(sr >> 8), uint8(sg >> 8), uint8(sb >> 8), uint8(sa >> 8)}
	out := BlendPremultiplied(d, s, mode, opacity)
	return color.RGBA{R: out[0], G: out[1], B: out[2], A: out[3]}
}
