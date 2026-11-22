// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 13/12/2010 by Laurent Le Goff

package draw2dimg

import (
	"math"

	"github.com/golang/freetype/raster"
	"golang.org/x/image/math/fixed"
)

// maxRasterCoord bounds the coordinates handed to the freetype rasterizer.
// The rasterizer walks one iteration per scanline between the lowest and
// highest y of a segment, so coordinates far outside any realistic canvas
// (or NaN/±Inf, which convert to implementation-defined integers) would
// make it loop for an unbounded amount of time. Clamping does not change
// the rendered output for any canvas smaller than maxRasterCoord pixels,
// because the rasterizer clips cells to its own bounds anyway.
const maxRasterCoord = 1 << 15

// clampRasterCoord converts v to 26.6 fixed point, clamping to
// ±maxRasterCoord pixels. It returns false for NaN, which has no
// representation and must be skipped by the caller.
func clampRasterCoord(v float64) (fixed.Int26_6, bool) {
	if math.IsNaN(v) {
		return 0, false
	}
	if v > maxRasterCoord {
		v = maxRasterCoord
	} else if v < -maxRasterCoord {
		v = -maxRasterCoord
	}
	return fixed.Int26_6(v * 64), true
}

type FtLineBuilder struct {
	Adder raster.Adder
}

func (liner FtLineBuilder) MoveTo(x, y float64) {
	fx, okx := clampRasterCoord(x)
	fy, oky := clampRasterCoord(y)
	if !okx || !oky {
		return
	}
	liner.Adder.Start(fixed.Point26_6{X: fx, Y: fy})
}

func (liner FtLineBuilder) LineTo(x, y float64) {
	fx, okx := clampRasterCoord(x)
	fy, oky := clampRasterCoord(y)
	if !okx || !oky {
		return
	}
	liner.Adder.Add1(fixed.Point26_6{X: fx, Y: fy})
}

func (liner FtLineBuilder) LineJoin() {
}

func (liner FtLineBuilder) Close() {
}

func (liner FtLineBuilder) End() {
}
