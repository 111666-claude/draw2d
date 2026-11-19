// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"

	"github.com/golang/freetype/raster"
)

// layer is one open BeginLayer/EndLayer scope.
type layer struct {
	mode    draw2d.BlendMode
	opacity float64
	// img is a transparent premultiplied surface holding what the layer drew.
	// It shares the destination bounds so both can be composited pixel for
	// pixel; dirty limits how much of it is read back.
	img *image.RGBA
	// dirty is the bounding rectangle of spans painted into img. An empty
	// image composites nothing, so an empty layer is a no-op.
	dirty image.Rectangle
	// savedImg and savedPainter are the destination and painter that were
	// active when the layer was opened; they are restored by EndLayer.
	savedImg     draw.Image
	savedPainter Painter
	// parent is the enclosing layer, or nil when the destination is the
	// backdrop. It lets a closed inner layer mark the enclosing surface dirty.
	parent *layer
}

// SetBlendMode sets the blend mode used by subsequent drawing operations.
//
// The normal mode keeps the historical fast path (drawing directly into the
// destination). Multiply and screen are applied per drawing operation: the
// rasterized spans are combined pixel by pixel, in premultiplied alpha, with
// the current destination. Unlike a plain "over", this works on a transparent
// destination and avoids washing colors towards grey.
//
// For a group of operations that must blend as a whole (for example an
// anti-aliased sprite with soft edges), use BeginLayer instead.
//
// An unknown mode is rejected with draw2d.ErrInvalidBlendMode and leaves the
// current mode unchanged.
func (gc *GraphicContext) SetBlendMode(mode draw2d.BlendMode) error {
	if !mode.IsValid() {
		return draw2d.ErrInvalidBlendMode
	}
	return gc.StackGraphicContext.SetBlendMode(mode)
}

// BeginLayer starts an isolated layer rendered into an offscreen RGBA image
// the size of the destination, then composited with mode and opacity in
// EndLayer.
//
// The layer behaves like Save: drawing state at the call is restored when the
// layer ends. While the layer is open, operations draw into the offscreen
// buffer, which starts fully transparent, so a partially covered layer
// composites only the pixels it actually painted. Inside the layer drawing
// uses normal compositing: elements combine with each other, and the whole
// group is blended with the backdrop when the layer ends.
//
// An unknown mode, a NaN or infinite opacity, nesting deeper than
// draw2d.MaxLayerDepth, an empty canvas and a canvas larger than
// MaxLayerPixels pixels are rejected (draw2d.ErrInvalidBlendMode,
// draw2d.ErrInvalidOpacity, draw2d.ErrLayerDepthExceeded, draw2d.ErrEmptyCanvas
// and draw2d.ErrLayerTooLarge respectively) without changing the context.
func (gc *GraphicContext) BeginLayer(mode draw2d.BlendMode, opacity float64) error {
	if !mode.IsValid() {
		return draw2d.ErrInvalidBlendMode
	}
	opacity, err := draw2d.NormalizeOpacity(opacity)
	if err != nil {
		return err
	}
	if len(gc.layers) >= draw2d.MaxLayerDepth {
		return draw2d.ErrLayerDepthExceeded
	}
	bounds := gc.img.Bounds()
	pixels := int64(bounds.Dx()) * int64(bounds.Dy())
	if pixels <= 0 {
		return draw2d.ErrEmptyCanvas
	}
	maxPixels := gc.MaxLayerPixels
	if maxPixels <= 0 {
		maxPixels = draw2d.DefaultMaxLayerPixels
	}
	if pixels > int64(maxPixels) {
		return draw2d.ErrLayerTooLarge
	}

	gc.Save()
	gc.Current.BlendMode = draw2d.BlendNormal

	// The layer surface shares the destination bounds so the two can be
	// composited pixel for pixel; only the painted part is ever read.
	img := image.NewRGBA(bounds)
	var parent *layer
	if n := len(gc.layers); n > 0 {
		parent = gc.layers[n-1]
	}
	newLayer := &layer{
		mode:         mode,
		opacity:      opacity,
		img:          img,
		savedImg:     gc.img,
		savedPainter: gc.painter,
		parent:       parent,
	}
	gc.layers = append(gc.layers, newLayer)
	gc.img = img
	gc.painter = &layerPainter{layer: newLayer, img: img}
	gc.setRasterizerBounds(bounds)
	return nil
}

// EndLayer closes the innermost layer, composites it onto the backdrop using
// the mode and opacity given to BeginLayer, and restores the drawing state
// saved by that BeginLayer call.
//
// An empty layer (nothing painted) leaves the destination untouched. EndLayer
// returns draw2d.ErrLayerNotOpen when no layer is open; it never panics.
func (gc *GraphicContext) EndLayer() error {
	if len(gc.layers) == 0 {
		return draw2d.ErrLayerNotOpen
	}
	layer := gc.layers[len(gc.layers)-1]
	gc.layers = gc.layers[:len(gc.layers)-1]

	// Restore the destination before compositing so the layer is not applied
	// to itself, then restore the drawing state saved by BeginLayer.
	gc.img = layer.savedImg
	gc.painter = layer.savedPainter
	gc.setRasterizerBounds(layer.savedImg.Bounds())
	gc.Restore()

	// Composite clips to the intersection of both bounds, so painting outside
	// the canvas is discarded like any other clipped drawing operation.
	if !layer.dirty.Empty() {
		draw2dbase.CompositeRect(layer.savedImg, layer.img, layer.dirty, layer.mode, layer.opacity)
		if layer.parent != nil {
			// The closed layer painted into the enclosing surface; mark that
			// area so the enclosing layer composites it too.
			layer.parent.dirty = layer.parent.dirty.Union(layer.dirty)
		}
	}
	return nil
}

// setRasterizerBounds updates both rasterizers to the given target bounds.
func (gc *GraphicContext) setRasterizerBounds(bounds image.Rectangle) {
	gc.fillRasterizer.SetBounds(bounds.Dx(), bounds.Dy())
	gc.strokeRasterizer.SetBounds(bounds.Dx(), bounds.Dy())
}

// blendPainter paints freetype spans directly onto a destination, combining
// each pixel with the backdrop using a blend mode. The color is supplied as a
// straight (non-premultiplied) color and each span's coverage becomes the
// source alpha, so anti-aliased edges blend correctly pixel by pixel.
type blendPainter struct {
	dst  draw.Image
	mode draw2d.BlendMode
	cr   uint32
	cg   uint32
	cb   uint32
	ca   uint32
	// dirty accumulates the painted area so a blend performed inside a layer
	// is composited by EndLayer.
	dirty image.Rectangle
}

// SetColor sets the color used for subsequent spans.
func (p *blendPainter) SetColor(c color.Color) {
	p.cr, p.cg, p.cb, p.ca = c.RGBA()
}

// Paint satisfies raster.Painter.
func (p *blendPainter) Paint(ss []raster.Span, done bool) {
	const m = 1<<16 - 1
	if rgba, ok := p.dst.(*image.RGBA); ok {
		b := rgba.Bounds()
		for _, s := range ss {
			if s.Y < b.Min.Y {
				continue
			}
			if s.Y >= b.Max.Y {
				return
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
			i0 := rgba.PixOffset(x0, s.Y)
			i1 := rgba.PixOffset(x1, s.Y)
			p.dirty = p.dirty.Union(image.Rect(x0, s.Y, x1, s.Y+1))
			ma := s.Alpha
			for i := i0; i < i1; i += 4 {
				sa := p.ca * ma / m
				if sa == 0 {
					continue
				}
				src := [4]uint8{
					uint8(p.cr * ma / m >> 8),
					uint8(p.cg * ma / m >> 8),
					uint8(p.cb * ma / m >> 8),
					uint8(sa >> 8),
				}
				dst := [4]uint8{rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3]}
				out := draw2dbase.BlendPremultiplied(dst, src, p.mode, 1)
				rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] = out[0], out[1], out[2], out[3]
			}
		}
		return
	}
	for _, s := range ss {
		ma := s.Alpha
		sa := p.ca * ma / m
		if sa == 0 {
			continue
		}
		p.dirty = p.dirty.Union(image.Rect(s.X0, s.Y, s.X1, s.Y+1))
		src := color.RGBA{
			R: uint8(p.cr * ma / m >> 8),
			G: uint8(p.cg * ma / m >> 8),
			B: uint8(p.cb * ma / m >> 8),
			A: uint8(sa >> 8),
		}
		for x := s.X0; x < s.X1; x++ {
			if !image.Pt(x, s.Y).In(p.dst.Bounds()) {
				continue
			}
			out := draw2dbase.BlendColor(p.dst.At(x, s.Y), src, p.mode, 1)
			p.dst.Set(x, s.Y, out)
		}
	}
}

// layerPainter paints raster spans onto the layer surface and records the
// bounding rectangle of everything it touched.
type layerPainter struct {
	layer *layer
	img   *image.RGBA
	cr    uint32
	cg    uint32
	cb    uint32
	ca    uint32
}

// SetColor sets the color used for subsequent spans.
func (p *layerPainter) SetColor(c color.Color) {
	p.cr, p.cg, p.cb, p.ca = c.RGBA()
}

// Paint satisfies raster.Painter.
func (p *layerPainter) Paint(ss []raster.Span, done bool) {
	const m = 1<<16 - 1
	b := p.img.Bounds()
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
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
		p.layer.dirty = p.layer.dirty.Union(image.Rect(x0, s.Y, x1, s.Y+1))
		i0 := p.img.PixOffset(x0, s.Y)
		i1 := p.img.PixOffset(x1, s.Y)
		ma := s.Alpha
		cr, cg, cb, ca := p.cr, p.cg, p.cb, p.ca
		for i := i0; i < i1; i += 4 {
			dr := uint32(p.img.Pix[i+0])
			dg := uint32(p.img.Pix[i+1])
			db := uint32(p.img.Pix[i+2])
			da := uint32(p.img.Pix[i+3])
			a := (m - (ca * ma / m)) * 0x101
			p.img.Pix[i+0] = uint8((dr*a + cr*ma) / m >> 8)
			p.img.Pix[i+1] = uint8((dg*a + cg*ma) / m >> 8)
			p.img.Pix[i+2] = uint8((db*a + cb*ma) / m >> 8)
			p.img.Pix[i+3] = uint8((da*a + ca*ma) / m >> 8)
		}
	}
}

// markDirty records that the current target now has painted pixels in r.
func (gc *GraphicContext) markDirty(r image.Rectangle) {
	if n := len(gc.layers); n > 0 {
		gc.layers[n-1].dirty = gc.layers[n-1].dirty.Union(r.Intersect(gc.img.Bounds()))
	}
}
