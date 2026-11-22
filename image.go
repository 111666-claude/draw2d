// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"errors"
	"image"
	"math"
)

// Filter defines the interpolation algorithm used when an image is scaled
// while being drawn by DrawImageRect.
type Filter int

const (
	// FilterNearest selects the nearest source pixel. It is fast and
	// preserves hard edges, which makes it a good fit for sprite sheets
	// and pixel art.
	FilterNearest Filter = iota
	// FilterLinear uses bilinear interpolation.
	FilterLinear
	// FilterBicubic uses Catmull-Rom bicubic interpolation.
	FilterBicubic
)

// MaxImageScale is the largest scale factor (device pixels per source
// pixel, on either axis) accepted when drawing an image with DrawImageRect.
// Larger scales are rejected with ErrImageScaleTooLarge.
const MaxImageScale = 1 << 14

// Errors returned by the DrawImageRect implementations and by
// ImageRectMatrix.
var (
	// ErrNilImage is returned when a nil source image is passed.
	ErrNilImage = errors.New("draw2d: nil source image")
	// ErrEmptySourceRect is returned when the source rectangle is empty.
	ErrEmptySourceRect = errors.New("draw2d: empty source rectangle")
	// ErrSourceRectOutside is returned when the source rectangle does not
	// intersect the bounds of the source image.
	ErrSourceRectOutside = errors.New("draw2d: source rectangle does not intersect image bounds")
	// ErrNegativeDestRect is returned when the destination rectangle has a
	// negative width or height.
	ErrNegativeDestRect = errors.New("draw2d: negative destination rectangle size")
	// ErrNonFiniteMatrix is returned when the transformation matrix
	// contains NaN or infinite values.
	ErrNonFiniteMatrix = errors.New("draw2d: non-finite transformation matrix")
	// ErrImageScaleTooLarge is returned when the scale factor from source
	// pixels to device pixels exceeds MaxImageScale.
	ErrImageScaleTooLarge = errors.New("draw2d: image scale exceeds MaxImageScale")
)

// ImageRectMatrix validates the geometry of a DrawImageRect call and
// computes the affine matrix that maps source image pixels to device
// coordinates: srcRect (in source pixel coordinates) is mapped onto dstRect
// (in user coordinates) and the result is composed with tr, the current
// user-to-device transformation.
//
// It returns the composed matrix together with srcRect clipped to
// srcBounds. A source rectangle that extends beyond srcBounds is not an
// error: only the intersecting part is drawn.
//
// A returned empty rectangle with a nil error means the draw is a no-op.
// This happens when dstRect has a zero width or height, or when tr is
// singular and collapses the destination to a line or a point.
func ImageRectMatrix(srcBounds, srcRect, dstRect image.Rectangle, tr Matrix) (Matrix, image.Rectangle, error) {
	var m Matrix
	if srcRect.Empty() {
		return m, image.Rectangle{}, ErrEmptySourceRect
	}
	sr := srcRect.Intersect(srcBounds)
	if sr.Empty() {
		return m, sr, ErrSourceRectOutside
	}
	dw, dh := dstRect.Dx(), dstRect.Dy()
	if dw < 0 || dh < 0 {
		return m, sr, ErrNegativeDestRect
	}
	if dw == 0 || dh == 0 {
		// Zero-sized destination: nothing to draw.
		return m, image.Rectangle{}, nil
	}
	for _, v := range tr {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return m, sr, ErrNonFiniteMatrix
		}
	}
	sx := float64(dw) / float64(srcRect.Dx())
	sy := float64(dh) / float64(srcRect.Dy())
	// t maps source pixel coordinates to user coordinates so that srcRect
	// lands exactly on dstRect.
	t := Matrix{
		sx, 0, 0, sy,
		float64(dstRect.Min.X) - float64(srcRect.Min.X)*sx,
		float64(dstRect.Min.Y) - float64(srcRect.Min.Y)*sy,
	}
	m = tr.Copy()
	m.Compose(t) // m = tr ∘ t: source pixels -> user space -> device space
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Matrix{}, sr, ErrNonFiniteMatrix
		}
	}
	if math.Hypot(m[0], m[1]) > MaxImageScale || math.Hypot(m[2], m[3]) > MaxImageScale {
		return Matrix{}, sr, ErrImageScaleTooLarge
	}
	if m.Determinant() == 0 {
		// Degenerate transformation: the image collapses to a line or a
		// point, nothing is drawn.
		return m, image.Rectangle{}, nil
	}
	return m, sr, nil
}
