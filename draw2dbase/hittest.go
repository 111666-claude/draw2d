// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 2026

package draw2dbase

import "github.com/llgcode/draw2d"

// PathContains reports whether the device space point (x, y) is inside the
// current path. The current transformation matrix is applied to the path and
// the current fill rule selects between even-odd and non-zero winding, so
// the result matches what a Fill of the current path would paint. The
// current path is not modified.
//
// It is a convenience wrapper around draw2d.Path.ContainsWithMatrix driven
// by the graphic context state; see that method for the handling of empty
// paths, boundary points, non finite points and singular matrices.
func (gc *StackGraphicContext) PathContains(x, y float64) bool {
	return gc.Current.Path.ContainsWithMatrix(x, y, gc.Current.Tr, gc.Current.FillRule)
}

// PathContainsWithRule works like PathContains but lets the caller pick the
// fill rule instead of using the graphic context's current one.
func (gc *StackGraphicContext) PathContainsWithRule(x, y float64, rule draw2d.FillRule) bool {
	return gc.Current.Path.ContainsWithMatrix(x, y, gc.Current.Tr, rule)
}
