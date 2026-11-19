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

// gradientPainter is a raster.Painter that paints spans with a linear gradient
// instead of a solid color.
//
// The freetype rasterizer applies the current transformation matrix to the
// geometry before producing spans, so spans are in device space. The gradient,
// however, is defined in user space. gradientPainter therefore maps every span
// pixel back through the inverse of the current transformation and evaluates
// the gradient at that user space point. As a result the gradient rotates and
// scales together with the path, and the image and SVG backends share the same
// user space definition.
type gradientPainter struct {
	img  *image.RGBA
	grad *draw2d.LinearGradient
	ramp *draw2d.GradientRamp
	inv  draw2d.Matrix
	op   stddraw.Op

	// The gradient parameter is affine in device coordinates:
	// t(x, y) = t0 + dtx*x + dty*y. Precomputing these coefficients makes
	// sampling independent of canvas size and avoids repeating the inverse
	// transform and projection for every pixel.
	t0, dtx, dty float64
	degenerate   bool
}

// newGradientPainter prepares a painter for the gradient. It returns nil when
// the gradient cannot be painted (non-finite axis or no usable stop), in which
// case callers must fall back to a solid painter.
func newGradientPainter(img *image.RGBA, g *draw2d.LinearGradient, tr draw2d.Matrix, op stddraw.Op) *gradientPainter {
	if img == nil || g == nil {
		return nil
	}
	norm := g.Normalize()
	if !norm.Usable() {
		return nil
	}
	ramp := norm.Ramp()
	p := &gradientPainter{
		img:  img,
		grad: norm,
		ramp: ramp,
		op:   op,
	}
	// Invert the transformation used at fill time. A singular matrix
	// collapses the geometry to a line or a point, producing no span worth
	// painting; the identity fallback keeps the painter safe.
	d := tr.Determinant()
	if d != 0 && !math.IsNaN(d) && !math.IsInf(d, 0) {
		p.inv = tr
		p.inv.Inverse()
	} else {
		p.inv = draw2d.NewIdentityMatrix()
	}
	p.prepareAxis()
	return p
}

// prepareAxis folds the inverse transform and the gradient projection into a
// single affine map from device coordinates to the gradient parameter:
//
//	t(x, y) = t0 + dtx*x + dty*y
//
// A zero-length axis (or non-finite coefficients) sets degenerate; the painter
// then uses the last stop color, matching the SVG rule.
func (p *gradientPainter) prepareAxis() {
	g := p.grad
	dx := g.X1 - g.X0
	dy := g.Y1 - g.Y0
	length := math.Hypot(dx, dy)
	if length == 0 || math.IsNaN(length) {
		p.degenerate = true
		return
	}
	// Unit direction along the axis. Dividing by length first keeps very large
	// axis coordinates from overflowing when multiplied by the matrix.
	ux := dx / length
	uy := dy / length
	m := p.inv
	// user x' = m0*x + m2*y + m4, y' = m1*x + m3*y + m5.
	t0 := ((m[4]-g.X0)*ux + (m[5]-g.Y0)*uy) / length
	dtx := (m[0]*ux + m[1]*uy) / length
	dty := (m[2]*ux + m[3]*uy) / length
	if math.IsInf(t0, 0) || math.IsNaN(t0) ||
		math.IsInf(dtx, 0) || math.IsNaN(dtx) ||
		math.IsInf(dty, 0) || math.IsNaN(dty) {
		p.degenerate = true
		return
	}
	p.t0, p.dtx, p.dty = t0, dtx, dty
}

// SetColor satisfies the Painter interface but is ignored: the gradient painter
// derives every pixel color from its gradient.
func (p *gradientPainter) SetColor(color.Color) {}

// Paint satisfies raster.Painter. Each span is filled with the gradient color
// sampled at the user space position of its pixel centers. Spans are in device
// space and are clipped to the image bounds.
func (p *gradientPainter) Paint(ss []raster.Span, done bool) {
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
// Colors are evaluated at pixel centers (x+0.5, y+0.5), matching the pixel
// sampling used when the SVG output is rasterized.
func (p *gradientPainter) paintSpan(y, x0, x1 int, alpha uint32) {
	if alpha == 0 || x1 <= x0 {
		return
	}
	const m = 1<<16 - 1
	b := p.img.Bounds()
	base := (y-b.Min.Y)*p.img.Stride + (x0-b.Min.X)*4
	pix := p.img.Pix
	// The compositing below mirrors freetype's RGBAPainter so that gradient
	// and solid fills blend identically for the same coverage.
	ma := alpha
	// t is affine in x, so advance it per pixel instead of recomputing the
	// inverse transform and projection each time.
	t := p.t0 + p.dtx*(float64(x0)+0.5) + p.dty*(float64(y)+0.5)
	for x := x0; x < x1; x++ {
		var c color.NRGBA
		if p.degenerate {
			// Zero length axis: SVG paints the last stop color.
			c = p.ramp.LastColor()
		} else {
			c = p.ramp.ColorAt(t)
		}
		t += p.dtx
		cr, cg, cb, ca := c.RGBA()
		off := base + (x-x0)*4
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
