// Copyright 2018 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d"
	"golang.org/x/image/draw"
)

// AlphaMode selects how color and alpha are treated while resampling an
// image.
//
// The default, StraightAlphaMode, keeps the historical draw2d behavior
// unchanged: color channels are interpolated independently of alpha. The
// premultiplied modes eliminate the dark fringe ("black halo") that appears
// when downscaling a texture whose fully transparent pixels carry hidden
// RGB, by weighting every source sample by its alpha before filtering.
type AlphaMode int

const (
	// StraightAlphaMode resamples non-premultiplied ("straight") RGBA
	// channels directly. It is the zero value and preserves the original
	// draw2d output bit for bit.
	StraightAlphaMode AlphaMode = iota

	// PremulAlphaMode converts every source pixel to premultiplied alpha,
	// performs the resampling in a well defined (explicit) color space and
	// un-premultiplies the result before writing it. A fully transparent
	// source pixel contributes nothing, so hidden RGB can never bleed into an
	// edge.
	PremulAlphaMode

	// LinearPremulAlphaMode behaves like PremulAlphaMode but additionally
	// linearizes the sRGB-encoded channels (decode with the sRGB transfer
	// function) before filtering and re-encodes them afterwards. Filtering
	// then happens in linear light, which gives physically meaningful
	// downscaling of semi-transparent gradients.
	LinearPremulAlphaMode
)

// ColorSpace describes the transfer encoding of the color data passed to the
// resampler. SRGBColorSpace is the standard gamma-encoded space of 8-bit
// images. LinearColorSpace marks data that is already linear, so no transfer
// conversion is applied for LinearPremulAlphaMode.
type ColorSpace int

const (
	// SRGBColorSpace treats channel values as sRGB-encoded (the common case
	// for PNG/JPEG images).
	SRGBColorSpace ColorSpace = iota

	// LinearColorSpace treats channel values as already linear.
	LinearColorSpace
)

// ResampleOptions configures the optional premultiplied-alpha resampling
// path. The zero value is valid and means "no change from the legacy
// behavior".
type ResampleOptions struct {
	// AlphaMode selects straight versus premultiplied resampling. The zero
	// value (StraightAlphaMode) keeps the historical output.
	AlphaMode AlphaMode

	// ColorSpace selects the transfer encoding. It only affects
	// LinearPremulAlphaMode; the other two modes operate on the raw channel
	// values regardless of this field.
	ColorSpace ColorSpace

	// MaxTaps caps the number of source samples inspected per axis for one
	// destination pixel. It guarantees bounded temporary storage even for
	// extreme downscaling. If <= 0, a sane default (64) is used. The cap
	// never changes correctness of in-bounds pixels; it only limits the
	// support of the (already normalized) reconstruction kernel.
	MaxTaps int
}

// DefaultMaxTaps is the per-axis sample cap used when ResampleOptions does
// not specify one.
const DefaultMaxTaps = 64

// resampleKernel describes a symmetric reconstruction kernel. radius is the
// support radius in source pixels; the kernel is assumed zero at or beyond it.
type resampleKernel struct {
	radius float64
	at     func(t float64) float64
}

func kernelFor(filter ImageFilter) resampleKernel {
	switch filter {
	case LinearFilter:
		// Nearest neighbor is handled specially (radius 0, single tap).
		return resampleKernel{radius: 0, at: nil}
	case BicubicFilter:
		// Catmull-Rom, identical to x/image/draw.CatmullRom.
		return resampleKernel{
			radius: 2,
			at: func(t float64) float64 {
				if t < 1 {
					return (1.5*t-2.5)*t*t + 1
				}
				return ((-0.5*t+2.5)*t-4)*t + 2
			},
		}
	default:
		// BilinearFilter: the tent kernel, identical to x/image/draw.BiLinear.
		return resampleKernel{
			radius: 1,
			at:     func(t float64) float64 { return 1 - t },
		}
	}
}

// DrawImageWithOptions draws src into dest through the affine matrix tr using
// the requested filter and Porter-Duff op. When opts.AlphaMode is a
// premultiplied mode, filtering runs on premultiplied-alpha values (optionally
// in linear light) and the output is converted back to the destination's
// format. A nil opts (or StraightAlphaMode) delegates to the legacy
// DrawImage path so existing output is unchanged.
func DrawImageWithOptions(src image.Image, dest draw.Image, tr draw2d.Matrix, op draw.Op, filter ImageFilter, opts *ResampleOptions) {
	if opts == nil || opts.AlphaMode == StraightAlphaMode {
		DrawImage(src, dest, tr, op, filter)
		return
	}
	resample(src, dest, tr, op, filter, *opts)
}

// DrawImagePremultiplied is a convenience wrapper around DrawImageWithOptions
// that enables premultiplied-alpha resampling with default settings.
func DrawImagePremultiplied(src image.Image, dest draw.Image, tr draw2d.Matrix, op draw.Op, filter ImageFilter) {
	DrawImageWithOptions(src, dest, tr, op, filter, &ResampleOptions{AlphaMode: PremulAlphaMode})
}

// sample holds one source contribution in normalized [0,1] premultiplied or
// straight float RGBA, plus its kernel weight.
type resampleSample struct {
	r, g, b, a float64
	weight     float64
}

// resample implements the bounded, allocation-safe resampler. It never
// allocates a buffer proportional to the source image size: per destination
// pixel it uses a scratch slice of at most (MaxTaps*MaxTaps) entries.
func resample(src image.Image, dest draw.Image, tr draw2d.Matrix, op draw.Op, filter ImageFilter, opts ResampleOptions) {
	if src == nil || dest == nil {
		return
	}
	sb := src.Bounds()
	db := dest.Bounds()
	if sb.Empty() || db.Empty() {
		return
	}

	// Reject non-finite matrices up front: an inverse of such a matrix would
	// produce NaN coordinates and undefined pixels.
	for _, v := range tr {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return
		}
	}
	det := tr.Determinant()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return
	}

	maxTaps := opts.MaxTaps
	if maxTaps <= 0 {
		maxTaps = DefaultMaxTaps
	}
	kernel := kernelFor(filter)
	nearest := filter == LinearFilter

	// Forward (src->dst) and inverse (dst->src) affine maps. draw2d stores
	// matrices column-major as [sx, shy, shx, sy, tx, ty].
	s2d := [6]float64{tr[0], tr[1], tr[2], tr[3], tr[4], tr[5]}
	invDet := 1 / det
	d2s := [6]float64{
		tr[3] * invDet,
		-tr[1] * invDet,
		-tr[2] * invDet,
		tr[0] * invDet,
		(tr[2]*tr[5] - tr[3]*tr[4]) * invDet,
		(tr[1]*tr[4] - tr[0]*tr[5]) * invDet,
	}

	// Affected destination rectangle: transform the four source corners and
	// round outward, then clip to the destination bounds.
	affected := transformBounds(&s2d, sb).Intersect(db)
	if affected.Empty() {
		return
	}

	linearize := opts.AlphaMode == LinearPremulAlphaMode && opts.ColorSpace == SRGBColorSpace
	premul := opts.AlphaMode != StraightAlphaMode

	// Bounded scratch buffer, reused for every destination pixel.
	capTaps := maxTaps
	if nearest {
		capTaps = 1
	}
	scratch := make([]resampleSample, 0, capTaps*capTaps)

	for dy := affected.Min.Y; dy < affected.Max.Y; dy++ {
		// Destination pixel center in destination coordinates.
		dcy := float64(dy) + 0.5
		for dx := affected.Min.X; dx < affected.Max.X; dx++ {
			dcx := float64(dx) + 0.5
			scx := d2s[0]*dcx + d2s[2]*dcy + d2s[4]
			scy := d2s[1]*dcx + d2s[3]*dcy + d2s[5]
			if math.IsNaN(scx) || math.IsNaN(scy) || math.IsInf(scx, 0) || math.IsInf(scy, 0) {
				continue
			}

			scratch = scratch[:0]
			if nearest {
				ix := int(math.Floor(scx))
				iy := int(math.Floor(scy))
				if image.Pt(ix, iy).In(sb) {
					s := loadSource(src, ix, iy, premul, linearize)
					s.weight = 1
					scratch = append(scratch, s)
				}
			} else {
				scratch = gather(src, sb, scx, scy, kernel, maxTaps, premul, linearize, scratch)
			}

			if len(scratch) == 0 {
				// Entire kernel fell outside the source: leave the
				// destination untouched, matching x/image/draw semantics.
				continue
			}

			fr, fg, fb, fa := reduce(scratch, premul)
			if premul {
				fr, fg, fb, fa = unPremultiply(fr, fg, fb, fa, linearize)
			}
			composite(dest, dx, dy, fr, fg, fb, fa, op)
		}
	}
}

// gather collects the in-bounds source samples weighted by the separable
// kernel around source point (cx, cy). Out-of-bounds taps are skipped, which
// is equivalent to sampling transparent black. The returned weights are not
// normalized here; reduce normalizes them.
func gather(src image.Image, sb image.Rectangle, cx, cy float64, kernel resampleKernel, maxTaps int, premul, linearize bool, out []resampleSample) []resampleSample {
	x0 := int(math.Floor(cx - kernel.radius))
	x1 := int(math.Ceil(cx + kernel.radius))
	y0 := int(math.Floor(cy - kernel.radius))
	y1 := int(math.Ceil(cy + kernel.radius))

	// Clamp the support so the tap count stays bounded on extreme minification.
	if x1-x0 > maxTaps {
		mid := int(math.Floor(cx))
		x0 = mid - maxTaps/2
		x1 = x0 + maxTaps
	}
	if y1-y0 > maxTaps {
		mid := int(math.Floor(cy))
		y0 = mid - maxTaps/2
		y1 = y0 + maxTaps
	}

	for iy := y0; iy < y1; iy++ {
		if iy < sb.Min.Y || iy >= sb.Max.Y {
			continue
		}
		wy := kernel.at(math.Abs(cy - float64(iy)))
		if wy == 0 {
			continue
		}
		for ix := x0; ix < x1; ix++ {
			if ix < sb.Min.X || ix >= sb.Max.X {
				continue
			}
			wx := kernel.at(math.Abs(cx - float64(ix)))
			if wx == 0 {
				continue
			}
			s := loadSource(src, ix, iy, premul, linearize)
			s.weight = wx * wy
			out = append(out, s)
		}
	}
	return out
}

// reduce combines the gathered, weighted samples. In premultiplied mode the
// kernel weights are normalized by their sum so that edge pixels (where some
// taps are clipped) keep correct alpha and color instead of fading to black.
//
// Bicubic kernels have negative lobes: the weighted average can momentarily
// leave the valid premultiplied range. The result is clamped to legal
// premultiplied values (0 <= r,g,b <= a <= 1) before un-premultiplication,
// which is what keeps the negative lobes from re-introducing a dark fringe.
func reduce(samples []resampleSample, premul bool) (r, g, b, a float64) {
	var wsum float64
	for _, s := range samples {
		w := s.weight
		wsum += w
		r += s.r * w
		g += s.g * w
		b += s.b * w
		a += s.a * w
	}
	if wsum > 0 {
		inv := 1 / wsum
		r *= inv
		g *= inv
		b *= inv
		a *= inv
	}
	if premul {
		a = clamp01(a)
		r = clamp01(r)
		g = clamp01(g)
		b = clamp01(b)
		// Premultiplied channels cannot exceed alpha.
		if r > a {
			r = a
		}
		if g > a {
			g = a
		}
		if b > a {
			b = a
		}
	}
	return
}

// loadSource reads one source pixel as normalized [0,1] floats. When premul is
// true the returned color is premultiplied (and optionally linearized first);
// fully transparent pixels become (0,0,0,0) so their hidden RGB cannot leak.
func loadSource(src image.Image, x, y int, premul, linearize bool) resampleSample {
	cr, cg, cb, ca := src.At(x, y).RGBA()
	// color.Color.RGBA returns alpha-premultiplied 16-bit values.
	a := float64(ca) / 0xffff
	var r, g, b float64
	if premul {
		r = float64(cr) / 0xffff
		g = float64(cg) / 0xffff
		b = float64(cb) / 0xffff
		if linearize {
			r = srgbDecode(r)
			g = srgbDecode(g)
			b = srgbDecode(b)
			a = clamp01(a)
		}
		if a == 0 {
			r, g, b = 0, 0, 0
		}
	} else {
		// Recover straight (non-premultiplied) color.
		if a != 0 {
			inv := 1 / a
			r = clamp01(float64(cr) / 0xffff * inv)
			g = clamp01(float64(cg) / 0xffff * inv)
			b = clamp01(float64(cb) / 0xffff * inv)
		}
	}
	return resampleSample{r: r, g: g, b: b, a: a}
}

// unPremultiply converts a filtered premultiplied color back to straight
// RGBA in [0,1], re-encoding to sRGB when the filtering ran in linear light.
func unPremultiply(pr, pg, pb, pa float64, linearize bool) (r, g, b, a float64) {
	a = clamp01(pa)
	if a == 0 {
		return 0, 0, 0, 0
	}
	r = clamp01(pr / a)
	g = clamp01(pg / a)
	b = clamp01(pb / a)
	if linearize {
		r = srgbEncode(r)
		g = srgbEncode(g)
		b = srgbEncode(b)
	}
	return r, g, b, a
}

// composite writes the straight [0,1] source color to the destination pixel,
// applying either Src (replace) or Over (Porter-Duff) blending.
func composite(dst draw.Image, x, y int, fr, fg, fb, fa float64, op draw.Op) {
	fa = clamp01(fa)
	if op == draw.Over {
		dr, dg, db, da := dst.At(x, y).RGBA()
		outA := fa + (float64(da)/0xffff)*(1-fa)
		if outA <= 0 {
			return
		}
		inv := 1 / outA
		fr = (fr*fa + (float64(dr)/0xffff)*(1-fa)) * inv
		fg = (fg*fa + (float64(dg)/0xffff)*(1-fa)) * inv
		fb = (fb*fa + (float64(db)/0xffff)*(1-fa)) * inv
		fa = outA
	}
	dst.Set(x, y, color.NRGBA{
		R: floatTo8(fr),
		G: floatTo8(fg),
		B: floatTo8(fb),
		A: floatTo8(fa),
	})
}

// transformBounds returns the smallest integer rectangle containing the
// transformed four corners of r.
func transformBounds(m *[6]float64, r image.Rectangle) image.Rectangle {
	corners := [4][2]float64{
		{float64(r.Min.X), float64(r.Min.Y)},
		{float64(r.Max.X), float64(r.Min.Y)},
		{float64(r.Min.X), float64(r.Max.Y)},
		{float64(r.Max.X), float64(r.Max.Y)},
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range corners {
		tx := m[0]*c[0] + m[2]*c[1] + m[4]
		ty := m[1]*c[0] + m[3]*c[1] + m[5]
		minX = math.Min(minX, tx)
		minY = math.Min(minY, ty)
		maxX = math.Max(maxX, tx)
		maxY = math.Max(maxY, ty)
	}
	return image.Rect(
		int(math.Floor(minX)),
		int(math.Floor(minY)),
		int(math.Ceil(maxX)),
		int(math.Ceil(maxY)),
	)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	if math.IsNaN(v) {
		return 0
	}
	return v
}

func floatTo8(v float64) uint8 {
	v = clamp01(v)
	return uint8(v*0xff + 0.5)
}

// srgbDecode applies the sRGB electro-optical transfer function (EOTF).
func srgbDecode(v float64) float64 {
	v = clamp01(v)
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// srgbEncode applies the inverse sRGB transfer function (OETF).
func srgbEncode(v float64) float64 {
	v = clamp01(v)
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}
