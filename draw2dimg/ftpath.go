// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 13/12/2010 by Laurent Le Goff

package draw2dimg

import (
	"github.com/golang/freetype/raster"
	"golang.org/x/image/math/fixed"
)

type FtLineBuilder struct {
	Adder raster.Adder
}

func (liner FtLineBuilder) MoveTo(x, y float64) {
	liner.Adder.Start(fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)})
}

func (liner FtLineBuilder) LineTo(x, y float64) {
	liner.Adder.Add1(fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)})
}

func (liner FtLineBuilder) LineJoin() {
}

func (liner FtLineBuilder) Close() {
}

func (liner FtLineBuilder) End() {
}

// translatingAdder wraps a raster.Adder and shifts every accumulated point by
// a constant device-space offset. Incremental rendering feeds a sub-rasterizer
// whose origin is the top-left corner of a dirty region: device coordinates
// are translated to region-local coordinates on the way in, and the
// rasterizer's Dx/Dy span offset translates painted spans back. Keeping the
// offset in a dedicated wrapper leaves the geometry pipeline (flattening,
// stroking, dashing) byte-for-byte identical between the full and incremental
// code paths.
type translatingAdder struct {
	adder  raster.Adder
	dx, dy fixed.Int26_6
}

// newTranslatingAdder returns an adder that maps device point (x, y) to
// region-local point (x-min.X, y-min.Y).
func newTranslatingAdder(adder raster.Adder, minX, minY int) *translatingAdder {
	return &translatingAdder{
		adder: adder,
		dx:    fixed.Int26_6(minX * 64),
		dy:    fixed.Int26_6(minY * 64),
	}
}

func (t *translatingAdder) Start(p fixed.Point26_6) {
	t.adder.Start(fixed.Point26_6{X: p.X - t.dx, Y: p.Y - t.dy})
}

func (t *translatingAdder) Add1(p fixed.Point26_6) {
	t.adder.Add1(fixed.Point26_6{X: p.X - t.dx, Y: p.Y - t.dy})
}

func (t *translatingAdder) Add2(b, c fixed.Point26_6) {
	t.adder.Add2(
		fixed.Point26_6{X: b.X - t.dx, Y: b.Y - t.dy},
		fixed.Point26_6{X: c.X - t.dx, Y: c.Y - t.dy},
	)
}

func (t *translatingAdder) Add3(b, c, d fixed.Point26_6) {
	t.adder.Add3(
		fixed.Point26_6{X: b.X - t.dx, Y: b.Y - t.dy},
		fixed.Point26_6{X: c.X - t.dx, Y: c.Y - t.dy},
		fixed.Point26_6{X: d.X - t.dx, Y: d.Y - t.dy},
	)
}
