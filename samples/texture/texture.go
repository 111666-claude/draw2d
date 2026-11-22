// Copyright 2026 The draw2d Authors. All rights reserved.

// Package texture demonstrates filling paths with a repeating image brush.
package texture

import (
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dkit"
	"github.com/llgcode/draw2d/samples"
)

// Tile builds the small checkerboard tile used by the sample. It is generated
// in memory so the sample has no resource file dependency.
func Tile() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	dark := color.RGBA{R: 0x30, G: 0x30, B: 0x38, A: 0xff}
	light := color.RGBA{R: 0xd8, G: 0xd8, B: 0xe0, A: 0xff}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if (x/8+y/8)%2 == 0 {
				img.SetRGBA(x, y, dark)
			} else {
				img.SetRGBA(x, y, light)
			}
		}
	}
	// A translucent margin makes the premultiplied filtering visible on the
	// tile border.
	for i := 0; i < 16; i++ {
		img.SetRGBA(i, 0, color.RGBA{R: 0xff, G: 0x33, B: 0x33, A: 0x80})
		img.SetRGBA(i, 15, color.RGBA{R: 0xff, G: 0x33, B: 0x33, A: 0x80})
	}
	return img
}

// Main draws the texture sample and returns the output filename.
// This should only be used during testing.
func Main(gc draw2d.GraphicContext, ext string) (string, error) {
	Draw(gc, 297, 210)
	return samples.Output("texture", ext), nil
}

// Draw fills a path with a repeated image brush. The path is rotated so the
// texture transform, not a pre-rotated copy of the image, provides the local
// rotation.
func Draw(gc draw2d.GraphicContext, w, h float64) {
	gc.SetFillImageTexture(draw2d.NewImageTexture(Tile(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.SetFillRule(draw2d.FillRuleWinding)

	// A rotated quadrilateral filled with the tiled texture.
	cx, cy := w/2, h/2
	const half = 70
	angle := math.Pi / 7
	// Corners of the local square, placed with the current matrix rotated by
	// angle. The texture follows the path's user space, so it rotates with it.
	gc.Save()
	gc.Translate(cx, cy)
	gc.Rotate(angle)
	draw2dkit.Rectangle(gc, -half, -half, half, half)
	gc.Fill()
	gc.Restore()

	// A second, unrotated path shows the axis-aligned tile phase under a local
	// translation and the repeat-x policy: the texture repeats horizontally and
	// is transparent above and below the tile.
	gc.SetFillImageTexture(draw2d.NewImageTexture(Tile(), draw2d.NewTranslationMatrix(20, 20), draw2d.TextureRepeatX))
	draw2dkit.Rectangle(gc, 10, 10, w-10, 10+2*16)
	gc.Fill()
}
