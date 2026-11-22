// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/golang/freetype/raster"
	"github.com/llgcode/draw2d"
)

// RadialGradientPainter is a freetype raster.Painter that fills spans
// with a draw2d.RadialGradient. The gradient is evaluated in user
// space: each device pixel is mapped back through Inv, the inverse of
// the transformation matrix in place when the filled path was
// rasterized, so the gradient follows the path through any affine
// transform.
type RadialGradientPainter struct {
	// Image is the destination image. *image.RGBA takes a fast path;
	// any other draw.Image is painted through its Set method.
	Image    draw.Image
	Gradient *draw2d.RadialGradient
	// Inv maps device coordinates back to the gradient's user space.
	Inv draw2d.Matrix
}

// compile-time check that RadialGradientPainter satisfies Painter
var _ Painter = (*RadialGradientPainter)(nil)

// SetColor is part of the Painter interface; the gradient supplies the
// colors, so it is a no-op.
func (p *RadialGradientPainter) SetColor(c color.Color) {}

// Paint satisfies the raster.Painter interface, compositing the
// gradient over the destination with the same source-over formula as
// freetype's RGBAPainter.
func (p *RadialGradientPainter) Paint(ss []raster.Span, done bool) {
	if rgba, ok := p.Image.(*image.RGBA); ok {
		p.paintRGBA(rgba, ss)
		return
	}
	p.paintGeneric(ss)
}

// colorAt returns the premultiplied 16-bit gradient color at the device
// pixel whose top-left corner is (x, y). The pixel is sampled at its
// center.
func (p *RadialGradientPainter) colorAt(x, y int) (r, g, b, a uint32) {
	ux, uy := p.Inv.TransformPoint(float64(x)+0.5, float64(y)+0.5)
	t := p.Gradient.Parameter(ux, uy)
	return p.Gradient.ColorAt16(t)
}

func (p *RadialGradientPainter) paintRGBA(img *image.RGBA, ss []raster.Span) {
	b := img.Bounds()
	const m = 1<<16 - 1
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
		}
		if s.X0 < b.Min.X {
			s.X0 = b.Min.X
		}
		if s.X1 > b.Max.X {
			s.X1 = b.Max.X
		}
		if s.X0 >= s.X1 {
			continue
		}
		ma := s.Alpha
		i0 := (s.Y-img.Rect.Min.Y)*img.Stride + (s.X0-img.Rect.Min.X)*4
		for x := s.X0; x < s.X1; x++ {
			cr, cg, cb, ca := p.colorAt(x, s.Y)
			if ca == 0 {
				i0 += 4
				continue
			}
			a := (m - (ca * ma / m)) * 0x101
			img.Pix[i0+0] = uint8((uint32(img.Pix[i0+0])*a + cr*ma) / m >> 8)
			img.Pix[i0+1] = uint8((uint32(img.Pix[i0+1])*a + cg*ma) / m >> 8)
			img.Pix[i0+2] = uint8((uint32(img.Pix[i0+2])*a + cb*ma) / m >> 8)
			img.Pix[i0+3] = uint8((uint32(img.Pix[i0+3])*a + ca*ma) / m >> 8)
			i0 += 4
		}
	}
}

func (p *RadialGradientPainter) paintGeneric(ss []raster.Span) {
	b := p.Image.Bounds()
	const m = 1<<16 - 1
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
		}
		if s.X0 < b.Min.X {
			s.X0 = b.Min.X
		}
		if s.X1 > b.Max.X {
			s.X1 = b.Max.X
		}
		if s.X0 >= s.X1 {
			continue
		}
		ma := s.Alpha
		for x := s.X0; x < s.X1; x++ {
			cr, cg, cb, ca := p.colorAt(x, s.Y)
			if ca == 0 {
				continue
			}
			dr, dg, db, da := p.Image.At(x, s.Y).RGBA()
			a := (m - (ca * ma / m))
			p.Image.Set(x, s.Y, color.RGBA64{
				uint16((dr*a + cr*ma) / m),
				uint16((dg*a + cg*ma) / m),
				uint16((db*a + cb*ma) / m),
				uint16((da*a + ca*ma) / m),
			})
		}
	}
}
