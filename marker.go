// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"errors"
	"image/color"
	"math"
)

// MarkerPosition identifies the vertices of a path a marker is attached to.
type MarkerPosition int

const (
	// MarkerStart attaches the marker to the first vertex of every subpath.
	MarkerStart MarkerPosition = 1 << iota
	// MarkerMid attaches the marker to every interior vertex of every subpath.
	MarkerMid
	// MarkerEnd attaches the marker to the last vertex of every subpath.
	MarkerEnd
)

// MaxMarkerSize is the largest allowed marker viewport or viewBox extent, in
// user units. Larger markers are rejected with ErrMarkerSize so a runaway
// value cannot produce gigantic output documents or rasters.
const MaxMarkerSize = 1 << 16

var (
	// ErrMarkerEmpty is returned when a marker has no path or an empty path.
	ErrMarkerEmpty = errors.New("draw2d: marker has no drawable path")
	// ErrMarkerSize is returned when a marker size, viewBox or reference
	// point is not finite, not positive, or exceeds MaxMarkerSize.
	ErrMarkerSize = errors.New("draw2d: marker size, viewBox and refX/refY must be finite, sizes must be in (0, MaxMarkerSize]")
	// ErrMarkerPosition is returned when SetMarker is called with a position
	// that is not exactly one of MarkerStart, MarkerMid or MarkerEnd.
	ErrMarkerPosition = errors.New("draw2d: marker position must be one of MarkerStart, MarkerMid or MarkerEnd")
	// ErrMarkerCircular is reported when a marker's path is (one of) the
	// path(s) it decorates. Backends omit such markers (or return this error
	// from the raster fallback) instead of recursing.
	ErrMarkerCircular = errors.New("draw2d: marker path cannot be the path it decorates")
)

// Marker describes a reusable symbol (arrow head, start/end dot, ...) that is
// instantiated at the vertices of a stroked or filled path, like the SVG
// marker-start/marker-mid/marker-end properties.
//
// Path holds the marker artwork in its own coordinate system. ViewBox
// (minX, minY, width, height) maps that artwork into the marker viewport of
// Width x Height user units; the zero ViewBox means the artwork is used
// unscaled. RefX/RefY is the point of the artwork (in Path coordinates) that
// is placed exactly on the vertex. If ScaleWithStroke is true the viewport is
// additionally scaled by the stroke width of the referencing path (SVG
// markerUnits="strokeWidth"), otherwise it uses plain user units
// (markerUnits="userSpaceOnUse").
//
// If OrientAuto is true the marker is rotated to the tangent of the path at
// each vertex, otherwise OrientAngle (degrees, clockwise, y-down) is used.
//
// Fill and Stroke optionally override the colors of the marker artwork. A nil
// Fill makes the marker inherit the stroke color of the referencing context,
// which is what arrow heads on stroked routes usually want.
type Marker struct {
	Path            *Path
	ViewBox         [4]float64
	RefX, RefY      float64
	Width, Height   float64
	OrientAuto      bool
	OrientAngle     float64
	ScaleWithStroke bool
	Fill, Stroke    color.Color
	// StrokeWidth is used to stroke the marker artwork when Stroke is set.
	// Values <= 0 mean 1 user unit.
	StrokeWidth float64
}

// Validate reports whether the marker can be rendered. It returns
// ErrMarkerEmpty for a missing or empty path and ErrMarkerSize for
// non-finite, non-positive or oversized dimensions.
func (m *Marker) Validate() error {
	if m == nil || m.Path == nil || m.Path.IsEmpty() {
		return ErrMarkerEmpty
	}
	if !isFinite(m.RefX) || !isFinite(m.RefY) || !isFinite(m.OrientAngle) {
		return ErrMarkerSize
	}
	if !validExtent(m.Width) || !validExtent(m.Height) {
		return ErrMarkerSize
	}
	if m.ViewBox != ([4]float64{}) {
		vb := m.ViewBox
		if !isFinite(vb[0]) || !isFinite(vb[1]) || !validExtent(vb[2]) || !validExtent(vb[3]) {
			return ErrMarkerSize
		}
	}
	return nil
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func validExtent(f float64) bool {
	return isFinite(f) && f > 0 && f <= MaxMarkerSize
}

// MarkerGraphicContext is an optional interface implemented by graphic
// contexts that support path markers. The svg backend renders them as native
// SVG marker elements, the image backend rasterizes them, and backends
// without marker support (pdf, gl) accept and ignore them.
type MarkerGraphicContext interface {
	GraphicContext
	// SetMarker attaches m to the given vertex position of subsequently
	// drawn paths. A nil marker removes the marker at that position.
	// It returns a validation error if the marker is not renderable.
	SetMarker(pos MarkerPosition, m *Marker) error
	// ClearMarkers removes all markers from the context.
	ClearMarkers()
}
