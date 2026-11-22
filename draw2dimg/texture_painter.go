// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	stddraw "image/draw"
	"math"

	"github.com/golang/freetype/raster"
	"github.com/llgcode/draw2d"
)

// texturePainter is a raster.Painter that fills spans with an image texture
// instead of a solid color.
//
// The freetype rasterizer applies the current transformation matrix to the
// geometry before producing spans, so spans are in device space. The texture,
// however, is described in image pixel space. texturePainter therefore folds
// the texture's local transform and the current transformation matrix into a
// single affine map from device coordinates to image pixel coordinates. As a
// result the texture moves, scales and rotates together with the path and the
// image and SVG backends share the same user space definition.
type texturePainter struct {
	img   *image.RGBA
	samp  *draw2d.TextureSampler
	local draw2d.Matrix
	op    stddraw.Op

	// Image pixel coordinates are affine in device coordinates:
	//   u(x, y) = u0 + dux*x + duy*y
	//   v(x, y) = v0 + dvx*x + dvy*y
	// Precomputing the coefficients keeps sampling independent of the canvas
	// size and avoids repeating the matrix work for every pixel.
	u0, dux, duy float64
	v0, dvx, dvy float64
}

// newTexturePainter prepares a painter for the texture. It returns nil when
// the texture cannot be painted (unusable description, or a singular
// device/image mapping), in which case callers must fall back to a solid
// painter.
func newTexturePainter(img *image.RGBA, t *draw2d.ImageTexture, tr draw2d.Matrix, op stddraw.Op) *texturePainter {
	if img == nil || t == nil {
		return nil
	}
	samp := draw2d.NewTextureSampler(t)
	if samp == nil {
		return nil
	}
	p := &texturePainter{
		img:   img,
		samp:  samp,
		local: t.LocalTransform(),
		op:    op,
	}
	if !p.prepare(tr) {
		return nil
	}
	return p
}

// prepare builds the device -> image pixel affine map from the texture's local
// transform and the current transformation matrix. It reports false when the
// composition is non-finite or singular.
func (p *texturePainter) prepare(tr draw2d.Matrix) bool {
	l := p.local
	// Combined image -> device transform, composed as tr(l(u, v)).
	a00 := l[0]*tr[0] + l[1]*tr[2]
	a01 := l[2]*tr[0] + l[3]*tr[2]
	a10 := l[0]*tr[1] + l[1]*tr[3]
	a11 := l[2]*tr[1] + l[3]*tr[3]
	b0 := l[4]*tr[0] + l[5]*tr[2] + tr[4]
	b1 := l[4]*tr[1] + l[5]*tr[3] + tr[5]
	for _, v := range []float64{a00, a01, a10, a11, b0, b1} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	det := a00*a11 - a01*a10
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return false
	}
	// Invert and absorb the translation: device (dx, dy) -> image (u, v).
	i00, i01 := a11/det, -a01/det
	i10, i11 := -a10/det, a00/det
	p.dux, p.duy = i00, i01
	p.dvx, p.dvy = i10, i11
	p.u0 = -(i00*b0 + i01*b1)
	p.v0 = -(i10*b0 + i11*b1)
	for _, v := range []float64{p.u0, p.dux, p.duy, p.v0, p.dvx, p.dvy} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// SetColor satisfies the Painter interface but is ignored: the texture painter
// derives every pixel color from its sampler.
func (p *texturePainter) SetColor(color.Color) {}

// Paint satisfies raster.Painter. Spans are in device space and are clipped to
// the image bounds. Each output pixel samples the texture at its centre.
func (p *texturePainter) Paint(ss []raster.Span, done bool) {
	b := p.img.Bounds()
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			break
		}
		x0, x1 := s.X0, s.X1
		if x0 < b.Min.X {
			x0 = b.Min.X
		}
		if x1 > b.Max.X {
			x1 = b.Max.X
		}
		if x0 >= x1 {
			continue
		}
		p.paintSpan(s.Y, x0, x1, s.Alpha)
	}
}

// paintSpan paints the horizontal span [x0, x1) at row y with coverage alpha.
// Samples are taken at device pixel centres (x+0.5, y+0.5).
func (p *texturePainter) paintSpan(y, x0, x1 int, alpha uint32) {
	if alpha == 0 || x1 <= x0 {
		return
	}
	const m = 1<<16 - 1
	b := p.img.Bounds()
	base := (y-b.Min.Y)*p.img.Stride + (x0-b.Min.X)*4
	pix := p.img.Pix
	ma := alpha
	fy := float64(y) + 0.5
	// u and v are affine in x, so advance them per pixel instead of
	// re-evaluating the inverse transform every time.
	u := p.u0 + p.dux*(float64(x0)+0.5) + p.duy*fy
	v := p.v0 + p.dvx*(float64(x0)+0.5) + p.dvy*fy
	for x := x0; x < x1; x++ {
		cr8, cg8, cb8, ca8 := p.samp.SamplePremultiplied(u, v)
		u += p.dux
		v += p.dvx
		// Scale premultiplied 8-bit channels to the 16-bit space used by the
		// compositing formula below.
		cr := uint32(cr8) * 0x101
		cg := uint32(cg8) * 0x101
		cb := uint32(cb8) * 0x101
		ca := uint32(ca8) * 0x101
		off := base + (x-x0)*4
		// This mirrors freetype's RGBAPainter so that texture and solid fills
		// blend identically for the same coverage.
		if p.op == stddraw.Over {
			dr := uint32(pix[off+0])
			dg := uint32(pix[off+1])
			db := uint32(pix[off+2])
			da := uint32(pix[off+3])
			a := (m - (ca * ma / m)) * 0x101
			pix[off+0] = uint8((dr*a + cr*ma) / m >> 8)
			pix[off+1] = uint8((dg*a + cg*ma) / m >> 8)
			pix[off+2] = uint8((db*a + cb*ma) / m >> 8)
			pix[off+3] = uint8((da*a + ca*ma) / m >> 8)
		} else {
			pix[off+0] = uint8(cr * ma / m >> 8)
			pix[off+1] = uint8(cg * ma / m >> 8)
			pix[off+2] = uint8(cb * ma / m >> 8)
			pix[off+3] = uint8(ca * ma / m >> 8)
		}
	}
}
