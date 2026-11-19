// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 20/11/2026 by draw2d contributors

package draw2d

import (
	"errors"
	"fmt"
	"math"
)

// BlendMode defines how the color produced by a drawing operation, or by an
// enclosed layer, is combined with the color already present in the
// destination (the backdrop).
//
// Blend modes follow the W3C Compositing and Blending specification. They are
// applied to straight (non-premultiplied) sRGB colors; alpha compositing is
// always performed with premultiplied alpha. BlendModeNormal is the default
// and preserves the historical draw2d output.
type BlendMode int

const (
	// BlendNormal places the source over the backdrop:
	// result = source over destination. This is the default.
	BlendNormal BlendMode = iota
	// BlendMultiply multiplies the source and backdrop colors:
	// result = source * backdrop. White leaves the backdrop unchanged,
	// black produces black.
	BlendMultiply
	// BlendScreen inverts, multiplies and inverts again:
	// result = source + backdrop - source * backdrop. Black leaves the
	// backdrop unchanged, white produces white.
	BlendScreen
)

// String returns the CSS name of the blend mode ("normal", "multiply",
// "screen"). Unknown modes are rendered as "BlendMode(n)".
func (m BlendMode) String() string {
	switch m {
	case BlendNormal:
		return "normal"
	case BlendMultiply:
		return "multiply"
	case BlendScreen:
		return "screen"
	default:
		return fmt.Sprintf("BlendMode(%d)", int(m))
	}
}

// IsValid reports whether m is a blend mode known to draw2d.
func (m BlendMode) IsValid() bool {
	switch m {
	case BlendNormal, BlendMultiply, BlendScreen:
		return true
	}
	return false
}

// Errors returned by the blend and layer API.
var (
	// ErrInvalidBlendMode is returned when an unknown BlendMode is supplied.
	// The graphic context is left untouched in that case.
	ErrInvalidBlendMode = errors.New("draw2d: invalid blend mode")
	// ErrInvalidOpacity is returned when a layer opacity is NaN or infinite.
	ErrInvalidOpacity = errors.New("draw2d: layer opacity must be finite")
	// ErrLayerTooLarge is returned when the offscreen buffer required by a
	// layer would exceed the backend's maximum layer size.
	ErrLayerTooLarge = errors.New("draw2d: layer exceeds the maximum offscreen size")
	// ErrLayerNotOpen is returned by EndLayer when no layer is open, and by
	// EndLayer calls that do not match a BeginLayer call.
	ErrLayerNotOpen = errors.New("draw2d: no layer is open")
	// ErrLayerDepthExceeded is returned when BeginLayer would nest layers
	// deeper than MaxLayerDepth. The limit keeps both the image and SVG
	// backends within a representable group nesting.
	ErrLayerDepthExceeded = errors.New("draw2d: maximum layer nesting depth exceeded")
	// ErrEmptyCanvas is returned by BeginLayer when the destination has an
	// empty area, so no offscreen buffer can be allocated.
	ErrEmptyCanvas = errors.New("draw2d: cannot open a layer on an empty canvas")
	// ErrBlendUnsupported is returned by backends that cannot render blend
	// modes (for example the PDF and OpenGL backends).
	ErrBlendUnsupported = errors.New("draw2d: blend modes are not supported by this backend")
)

const (
	// MaxLayerDepth is the maximum number of nested BeginLayer calls accepted
	// by a backend. It applies to the image backend's offscreen buffers and to
	// the SVG backend's nested groups.
	MaxLayerDepth = 64
	// DefaultMaxLayerPixels is the default maximum number of pixels of the
	// offscreen buffer required by one layer on the image backend. With 4
	// bytes per pixel it corresponds to 256 MiB. A backend rejects a layer
	// whose canvas is larger with ErrLayerTooLarge; the limit can be changed
	// per context with the MaxLayerPixels field.
	DefaultMaxLayerPixels = 64 << 20
)

// NormalizeOpacity validates a layer opacity. Non-finite values (NaN, +Inf,
// -Inf) are rejected with ErrInvalidOpacity. Finite values are clamped to the
// inclusive range [0, 1] and returned.
func NormalizeOpacity(opacity float64) (float64, error) {
	if math.IsNaN(opacity) || math.IsInf(opacity, 0) {
		return 0, ErrInvalidOpacity
	}
	if opacity < 0 {
		return 0, nil
	}
	if opacity > 1 {
		return 1, nil
	}
	return opacity, nil
}
