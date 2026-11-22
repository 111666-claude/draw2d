// Copyright 2026 The draw2d Authors. All rights reserved.

package boolop

import (
	"math"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
)

// flattenPath converts a draw2d path into a set of closed rings of straight
// segments. Curves and arcs are flattened with the same routines and
// tolerance the rendering backends use, so the boolean result matches what
// would have been drawn. Open subpaths are implicitly closed, following fill
// semantics. Degenerate subpaths (fewer than 3 distinct points) are dropped.
func flattenPath(p *draw2d.Path, o options) ([][]point, error) {
	f := &ringFlattener{}
	if p != nil {
		draw2dbase.Flatten(p, f, o.scale)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.rings, nil
}

// ringFlattener implements draw2dbase.Flattener, collecting closed rings.
type ringFlattener struct {
	rings [][]point
	cur   []point
	err   error
}

func (f *ringFlattener) add(x, y float64) {
	if f.err != nil {
		return
	}
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		f.err = ErrNonFinite
		return
	}
	if math.Abs(x) > MaxCoordinate || math.Abs(y) > MaxCoordinate {
		f.err = ErrCoordinateRange
		return
	}
	p := point{x, y}
	if n := len(f.cur); n > 0 && f.cur[n-1] == p {
		return
	}
	f.cur = append(f.cur, p)
}

func (f *ringFlattener) MoveTo(x, y float64) { f.flush(); f.add(x, y) }
func (f *ringFlattener) LineTo(x, y float64) { f.add(x, y) }
func (f *ringFlattener) LineJoin()           {}
func (f *ringFlattener) Close()              { f.flush() }
func (f *ringFlattener) End()                { f.flush() }

// flush terminates the current subpath as a closed ring (fill semantics
// implicitly close open subpaths).
func (f *ringFlattener) flush() {
	c := f.cur
	f.cur = nil
	for len(c) > 1 && c[len(c)-1] == c[0] {
		c = c[:len(c)-1]
	}
	if len(c) < 3 {
		return
	}
	ring := make([]point, len(c))
	copy(ring, c)
	f.rings = append(f.rings, ring)
}
