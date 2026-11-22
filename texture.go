// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"image"
	"image/color"
	stddraw "image/draw"
	"math"
)

// TextureRepeat selects how an ImageTexture is sampled outside the rectangle
// covered by the source image.
//
// The values map directly to the CSS background-repeat keywords: the repeated
// axes wrap around, while an axis that is not repeated is transparent outside
// the source image.
type TextureRepeat int

const (
	// TextureClamp places the image once and returns transparency outside it.
	// This is the "no-repeat" / clamp policy and the zero value.
	TextureClamp TextureRepeat = iota
	// TextureRepeatX repeats the image horizontally and leaves the vertical
	// direction transparent outside the image.
	TextureRepeatX
	// TextureRepeatY repeats the image vertically and leaves the horizontal
	// direction transparent outside the image.
	TextureRepeatY
	// TextureRepeatBoth repeats the image along both axes, producing a
	// seamless tile.
	TextureRepeatBoth
)

// String returns the CSS/SVG style name of the repeat policy.
func (r TextureRepeat) String() string {
	switch r {
	case TextureRepeatX:
		return "repeat-x"
	case TextureRepeatY:
		return "repeat-y"
	case TextureRepeatBoth:
		return "repeat"
	default:
		return "clamp"
	}
}

// Valid reports whether r is one of the defined repeat policies. Invalid
// values are normalized to TextureClamp.
func (r TextureRepeat) Valid() bool {
	switch r {
	case TextureClamp, TextureRepeatX, TextureRepeatY, TextureRepeatBoth:
		return true
	}
	return false
}

// TextureSampling selects how texels are filtered when the texture is sampled
// between pixel centres.
type TextureSampling int

const (
	// TextureSamplingBilinear interpolates the four neighbouring texels in
	// premultiplied alpha. This is the default and produces smooth scaling.
	TextureSamplingBilinear TextureSampling = iota
	// TextureSamplingNearest picks the closest texel. It keeps pixel art crisp
	// and maps to SVG's image-rendering="pixelated".
	TextureSamplingNearest
)

// String returns a descriptive name for the sampling mode.
func (s TextureSampling) String() string {
	switch s {
	case TextureSamplingNearest:
		return "nearest"
	default:
		return "bilinear"
	}
}

// Valid reports whether s is one of the defined sampling modes. Invalid values
// are normalized to TextureSamplingBilinear so that a mode a backend cannot
// represent always degrades to the default filter rather than failing.
func (s TextureSampling) Valid() bool {
	return s == TextureSamplingBilinear || s == TextureSamplingNearest
}

// ImageTexture describes an image used as a fill brush.
//
// Transform maps image pixel coordinates to user space using the same
// convention as Matrix.TransformPoint and the SVG matrix() function:
// image (u, v) becomes (u*a + v*c + e, u*b + v*d + f) for Matrix{a, b, c, d, e,
// f}. Image pixel coordinates are the coordinates of the image's own Bounds; a
// non-zero Bounds.Min therefore shifts the texture phase, and the repeated
// period is the image width or height. The zero Matrix is treated as the
// identity transform, so the common case of a texture whose pixels map
// one-to-one to user units needs no explicit matrix.
//
// The brush is installed with GraphicContext.SetFillImageTexture and is then
// used by Fill and FillStroke. Instances are treated as immutable snapshots by
// the backends: SetFillImageTexture copies and normalizes the description, so
// mutating the struct afterwards does not affect an already installed brush.
//
// Sampling rules:
//   - texels are filtered with the configured Sampling mode (bilinear by
//     default) entirely in premultiplied alpha, so semi transparent edges do
//     not produce dark fringes;
//   - repeated axes wrap continuously, so bilinear samples across a tile
//     boundary interpolate the first and last texels and leave no seam;
//   - non-repeated axes are transparent outside the image.
//
// A texture is unusable, and the solid fill color is used instead, when the
// image is nil, has an empty Bounds, the transform is non-finite or singular,
// or the transform has no inverse. See Usable. An unknown repeat policy is
// normalized to TextureClamp and an unknown sampling mode to bilinear.
type ImageTexture struct {
	// Image is the source image. It is read lazily by the backends.
	Image image.Image
	// Transform maps image pixel coordinates (the coordinate system of
	// Image.Bounds()) to user space. The zero Matrix means identity.
	Transform Matrix
	// Repeat selects the wrapping policy. The zero value is TextureClamp.
	Repeat TextureRepeat
	// Sampling selects the texel filter. The zero value is
	// TextureSamplingBilinear. An unknown mode is normalized to bilinear.
	Sampling TextureSampling
}

// NewImageTexture creates an image texture brush from img. localTransform maps
// image pixel coordinates to user space; pass NewIdentityMatrix (or the zero
// Matrix) for a one-to-one mapping. repeat selects the wrapping policy. The
// texel filter defaults to TextureSamplingBilinear.
func NewImageTexture(img image.Image, localTransform Matrix, repeat TextureRepeat) *ImageTexture {
	return NewImageTextureWithSampling(img, localTransform, repeat, TextureSamplingBilinear)
}

// NewImageTextureWithSampling is NewImageTexture with an explicit texel filter.
func NewImageTextureWithSampling(img image.Image, localTransform Matrix, repeat TextureRepeat, sampling TextureSampling) *ImageTexture {
	return &ImageTexture{
		Image:     img,
		Transform: normalizeTextureTransform(localTransform),
		Repeat:    repeat,
		Sampling:  sampling,
	}
}

// normalizeTextureTransform replaces the zero matrix with the identity. Any
// other matrix is copied unchanged so the brush never aliases caller state.
func normalizeTextureTransform(m Matrix) Matrix {
	if m == (Matrix{}) {
		return NewIdentityMatrix()
	}
	return m.Copy()
}

// LocalTransform returns the effective image-to-user transform. It is the
// identity when the stored transform is the zero matrix.
func (t *ImageTexture) LocalTransform() Matrix {
	if t == nil {
		return NewIdentityMatrix()
	}
	return normalizeTextureTransform(t.Transform)
}

// Normalize returns a normalized copy of t: the zero transform becomes the
// identity, an unknown repeat policy becomes TextureClamp and an unknown
// sampling mode becomes TextureSamplingBilinear. The image is not copied.
// Normalize returns nil when t is nil.
func (t *ImageTexture) Normalize() *ImageTexture {
	if t == nil {
		return nil
	}
	out := &ImageTexture{
		Image:     t.Image,
		Transform: normalizeTextureTransform(t.Transform),
		Repeat:    t.Repeat,
		Sampling:  t.Sampling,
	}
	if !out.Repeat.Valid() {
		out.Repeat = TextureClamp
	}
	if !out.Sampling.Valid() {
		out.Sampling = TextureSamplingBilinear
	}
	return out
}

// Usable reports whether the texture can be painted. It returns false for a
// nil texture or image, an empty image Bounds, a non-finite or singular
// transform. Backends fall back to the solid fill color when it returns false.
func (t *ImageTexture) Usable() bool {
	if t == nil || t.Image == nil {
		return false
	}
	b := t.Image.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return false
	}
	m := t.LocalTransform()
	for _, v := range m {
		if !isFiniteFloat(v) {
			return false
		}
	}
	d := m.Determinant()
	if !isFiniteFloat(d) || d == 0 {
		return false
	}
	return true
}

func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// TextureSampler is a prepared, immutable sampler for an ImageTexture. It
// converts the source image to premultiplied RGBA once and filters it with the
// texture's sampling mode.
//
// Coordinates passed to the sampling methods are absolute image pixel
// coordinates: the value (Bounds.Min.X, Bounds.Min.Y) addresses the top-left
// corner of the first pixel and (Bounds.Max.X, Bounds.Max.Y) the bottom-right
// corner of the last pixel. Pixel centres are therefore at half-integer
// coordinates.
//
// A TextureSampler is safe for concurrent use.
type TextureSampler struct {
	img      *image.RGBA
	repeat   TextureRepeat
	sampling TextureSampling
}

// NewTextureSampler prepares a sampler for t. It returns nil when t is not
// usable; backends must then fall back to the solid fill color.
func NewTextureSampler(t *ImageTexture) *TextureSampler {
	n := t.Normalize()
	if !n.Usable() {
		return nil
	}
	b := n.Image.Bounds()
	// Normalize the source into premultiplied RGBA. Working in premultiplied
	// space is what keeps transparent or semi transparent edges from bleeding
	// dark pixels into the filtered result.
	dst := image.NewRGBA(b)
	stddraw.Draw(dst, b, n.Image, b.Min, stddraw.Src)
	return &TextureSampler{img: dst, repeat: n.Repeat, sampling: n.Sampling}
}

// Bounds returns the image bounds the sampler reads from.
func (s *TextureSampler) Bounds() image.Rectangle {
	if s == nil || s.img == nil {
		return image.Rectangle{}
	}
	return s.img.Bounds()
}

// Repeat returns the wrapping policy of the sampler.
func (s *TextureSampler) Repeat() TextureRepeat {
	if s == nil {
		return TextureClamp
	}
	return s.repeat
}

// Sampling returns the texel filter of the sampler.
func (s *TextureSampler) Sampling() TextureSampling {
	if s == nil {
		return TextureSamplingBilinear
	}
	return s.sampling
}

func (s *TextureSampler) repeatsX() bool {
	return s.repeat == TextureRepeatX || s.repeat == TextureRepeatBoth
}

func (s *TextureSampler) repeatsY() bool {
	return s.repeat == TextureRepeatY || s.repeat == TextureRepeatBoth
}

// texel returns the premultiplied RGBA value of the image pixel at (ix, iy),
// or all zeroes when the pixel is outside the bounds. Repeated axes are
// wrapped by axis before this is called.
func (s *TextureSampler) texel(ix, iy int) (r, g, b, a float64) {
	bnd := s.img.Bounds()
	if ix < bnd.Min.X || ix >= bnd.Max.X || iy < bnd.Min.Y || iy >= bnd.Max.Y {
		return 0, 0, 0, 0
	}
	i := (iy-bnd.Min.Y)*s.img.Stride + (ix-bnd.Min.X)*4
	p := s.img.Pix
	return float64(p[i]), float64(p[i+1]), float64(p[i+2]), float64(p[i+3])
}

// axis converts a continuous pixel coordinate v into the two neighbouring
// texel indices and the interpolation weight. ok0/ok1 report whether each
// texel is inside the image; a false value contributes transparent.
func (s *TextureSampler) axis(v float64, repeat bool, min, max int) (i0, i1 int, f float64, ok0, ok1 bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, 0, 0, false, false
	}
	// Move from pixel edges to pixel centres.
	x := v - 0.5
	size := max - min
	if repeat && size > 0 {
		x = float64(min) + wrapTextureCoord(x-float64(min), float64(size))
		if x >= float64(max) {
			x = float64(min)
		}
		i0 = int(math.Floor(x))
		f = x - float64(i0)
		if i0 < min {
			i0 = min
		}
		i1 = i0 + 1
		if i1 >= max {
			i1 = min
		}
		return i0, i1, f, true, true
	}
	// Only the two texels around x can contribute. Outside that band the
	// sample is fully transparent; checking before the int conversion also
	// keeps huge coordinates from overflowing the conversion.
	if x < float64(min)-1 || x >= float64(max) {
		return 0, 0, 0, false, false
	}
	i0 = int(math.Floor(x))
	f = x - float64(i0)
	i1 = i0 + 1
	ok0 = i0 >= min && i0 < max
	ok1 = i1 >= min && i1 < max
	return i0, i1, f, ok0, ok1
}

// wrapTextureCoord reduces v into [0, size) using a mathematically positive
// modulo. NaN, infinities and non-positive sizes map to 0.
func wrapTextureCoord(v, size float64) float64 {
	if size <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	w := math.Mod(v, size)
	if w < 0 {
		w += size
	}
	if w >= size {
		w = 0
	}
	return w
}

// samplePremultiplied returns the filtered, premultiplied RGBA value at the
// absolute image pixel coordinate (x, y). Channels are in [0, 255]. A sample
// outside the image, or with a non-finite coordinate, returns transparent
// black. Bilinear sampling interpolates the four neighbouring texels in
// premultiplied space; nearest sampling picks the single closest texel.
func (s *TextureSampler) samplePremultiplied(x, y float64) (r, g, b, a float64) {
	if s == nil || s.img == nil {
		return 0, 0, 0, 0
	}
	bnd := s.img.Bounds()
	x0, x1, fx, ox0, ox1 := s.axis(x, s.repeatsX(), bnd.Min.X, bnd.Max.X)
	y0, y1, fy, oy0, oy1 := s.axis(y, s.repeatsY(), bnd.Min.Y, bnd.Max.Y)
	if !ox0 && !ox1 {
		return 0, 0, 0, 0
	}
	if !oy0 && !oy1 {
		return 0, 0, 0, 0
	}
	if s.sampling == TextureSamplingNearest {
		// Pick the texel whose centre is closest. On the repeated axes the
		// sampler already wraps, so the choice stays seamless at the seam for
		// an integer-shifted tile.
		ix, ok := x0, ox0
		if fx >= 0.5 {
			ix, ok = x1, ox1
		}
		iy, oky := y0, oy0
		if fy >= 0.5 {
			iy, oky = y1, oy1
		}
		if !ok || !oky {
			return 0, 0, 0, 0
		}
		tr, tg, tb, ta := s.texel(ix, iy)
		return tr, tg, tb, ta
	}
	// Bilinear weights. Interpolation happens in premultiplied space.
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	add := func(ix, iy int, ok bool, w float64) {
		if !ok || w == 0 {
			return
		}
		tr, tg, tb, ta := s.texel(ix, iy)
		r += tr * w
		g += tg * w
		b += tb * w
		a += ta * w
	}
	add(x0, y0, ox0 && oy0, w00)
	add(x1, y0, ox1 && oy0, w10)
	add(x0, y1, ox0 && oy1, w01)
	add(x1, y1, ox1 && oy1, w11)
	return r, g, b, a
}

func clampTextureChannel(v float64) uint8 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}

// SamplePremultiplied returns the filtered color at the absolute image pixel
// coordinate (x, y) as premultiplied 8-bit RGBA. It is the form expected by
// raster compositing code such as freetype's raster.RGBAPainter.
func (s *TextureSampler) SamplePremultiplied(x, y float64) (r, g, b, a uint8) {
	rf, gf, bf, af := s.samplePremultiplied(x, y)
	return clampTextureChannel(rf), clampTextureChannel(gf), clampTextureChannel(bf), clampTextureChannel(af)
}

// Sample returns the filtered color at the absolute image pixel coordinate
// (x, y) as a straight-alpha color.NRGBA. Semi transparent edges retain their
// hue because filtering is done in premultiplied space.
func (s *TextureSampler) Sample(x, y float64) color.NRGBA {
	rf, gf, bf, af := s.samplePremultiplied(x, y)
	if af <= 0 {
		return color.NRGBA{}
	}
	unpremultiply := func(c float64) uint8 {
		return clampTextureChannel(c * 255 / af)
	}
	return color.NRGBA{
		R: unpremultiply(rf),
		G: unpremultiply(gf),
		B: unpremultiply(bf),
		A: clampTextureChannel(af),
	}
}
