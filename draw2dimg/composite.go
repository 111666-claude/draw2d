// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d"

	"golang.org/x/image/draw"
)

// maxCompositePixels bounds the area (in pixels) of the destination and of
// the temporary layer allocated while compositing, so that absurd or
// overflowing sizes are rejected instead of triggering huge allocations.
const maxCompositePixels = 1 << 28 // 268M pixels (~1GB as RGBA)

var (
	// ErrEmptyImage is returned when the source image is nil or has empty bounds.
	ErrEmptyImage = errors.New("draw2dimg: source image is empty")
	// ErrImageTooLarge is returned when the destination or the composited
	// region exceeds maxCompositePixels.
	ErrImageTooLarge = errors.New("draw2dimg: image too large to composite")
	// ErrNonFiniteTransform is returned when the transformation matrix
	// contains NaN or infinite values.
	ErrNonFiniteTransform = errors.New("draw2dimg: transform matrix contains non-finite values")
)

// SupportedCompositeOps lists the compositing operations this backend
// renders natively: all of them.
func (gc *GraphicContext) SupportedCompositeOps() []draw2d.CompositeOp {
	return []draw2d.CompositeOp{
		draw2d.CompositeOpOver,
		draw2d.CompositeOpSrc,
		draw2d.CompositeOpIn,
		draw2d.CompositeOpOut,
		draw2d.CompositeOpAtop,
		draw2d.CompositeOpXor,
	}
}

// DrawImageWithOp draws img into the current canvas using the given
// compositing operation and the current transformation matrix. Unlike
// DrawImage it reports invalid operations, empty sources, non-finite
// transforms and oversized targets as errors instead of ignoring them.
func (gc *GraphicContext) DrawImageWithOp(img image.Image, op draw2d.CompositeOp) error {
	return DrawImageComposite(img, gc.img, gc.Current.Tr, op, gc.Filter)
}

// DrawImageComposite draws src into dest using an affine transformation
// matrix, a Porter-Duff compositing operation and a filter. All operators
// work on premultiplied alpha and are only applied inside the transformed
// bounds of src; destination pixels outside are left unchanged. The source
// image is only read, never modified.
func DrawImageComposite(src image.Image, dest draw.Image, tr draw2d.Matrix, op draw2d.CompositeOp, filter ImageFilter) error {
	if !op.Valid() {
		return fmt.Errorf("%w: %d", draw2d.ErrUnsupportedCompositeOp, int(op))
	}
	if src == nil || src.Bounds().Empty() {
		return ErrEmptyImage
	}
	for _, v := range tr {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrNonFiniteTransform
		}
	}
	db := dest.Bounds()
	if int64(db.Dx())*int64(db.Dy()) > maxCompositePixels {
		return ErrImageTooLarge
	}
	switch op {
	case draw2d.CompositeOpOver:
		DrawImage(src, dest, tr, draw.Over, filter)
		return nil
	case draw2d.CompositeOpSrc:
		DrawImage(src, dest, tr, draw.Src, filter)
		return nil
	}
	// In, Out, Atop and Xor are not provided by golang.org/x/image/draw:
	// render the transformed source into a temporary layer, then composite
	// it pixel by pixel onto the destination.
	region := transformedBounds(src.Bounds(), tr).Intersect(db)
	if region.Empty() {
		return nil
	}
	if int64(region.Dx())*int64(region.Dy()) > maxCompositePixels {
		return ErrImageTooLarge
	}
	tmp := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	layerTr := tr
	layerTr[4] -= float64(region.Min.X)
	layerTr[5] -= float64(region.Min.Y)
	DrawImage(src, tmp, layerTr, draw.Src, filter)
	compositeRegion(tmp, dest, region, op)
	return nil
}

// transformedBounds returns the integer bounding box of r transformed by tr.
func transformedBounds(r image.Rectangle, tr draw2d.Matrix) image.Rectangle {
	x0, y0, x1, y1 := tr.TransformRectangle(
		float64(r.Min.X), float64(r.Min.Y),
		float64(r.Max.X), float64(r.Max.Y))
	return image.Rect(
		int(math.Floor(x0)), int(math.Floor(y0)),
		int(math.Ceil(x1)), int(math.Ceil(y1)))
}

// compositeRegion composites the premultiplied source layer onto dest inside
// region (in dest coordinates; the layer origin is region.Min) using op.
func compositeRegion(src *image.RGBA, dest draw.Image, region image.Rectangle, op draw2d.CompositeOp) {
	const m = 0xffff
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			i := src.PixOffset(x-region.Min.X, y-region.Min.Y)
			sr := uint64(src.Pix[i+0]) * 0x101
			sg := uint64(src.Pix[i+1]) * 0x101
			sb := uint64(src.Pix[i+2]) * 0x101
			sa := uint64(src.Pix[i+3]) * 0x101
			dr, dg, db, da := dest.At(x, y).RGBA()
			da64 := uint64(da)
			var fs, fd uint64
			switch op {
			case draw2d.CompositeOpIn:
				fs, fd = da64, 0
			case draw2d.CompositeOpOut:
				fs, fd = m-da64, 0
			case draw2d.CompositeOpAtop:
				fs, fd = da64, m-uint64(sa)
			case draw2d.CompositeOpXor:
				fs, fd = m-da64, m-uint64(sa)
			}
			r := (sr*fs + uint64(dr)*fd + m/2) / m
			g := (sg*fs + uint64(dg)*fd + m/2) / m
			b := (sb*fs + uint64(db)*fd + m/2) / m
			a := (sa*fs + da64*fd + m/2) / m
			dest.Set(x, y, color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)})
		}
	}
}
