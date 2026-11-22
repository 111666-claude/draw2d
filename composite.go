// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import "errors"

// CompositeOp is a Porter-Duff compositing operator used when drawing a
// source (e.g. an image) onto a destination canvas. All operators work on
// premultiplied alpha values: with S the source, D the destination and
// Sa/Da their alphas, the result of each channel is:
//
//	CompositeOpOver: S + D*(1-Sa)
//	CompositeOpSrc:  S
//	CompositeOpIn:   S*Da
//	CompositeOpOut:  S*(1-Da)
//	CompositeOpAtop: S*Da + D*(1-Sa)
//	CompositeOpXor:  S*(1-Da) + D*(1-Sa)
//
// The operator is only applied inside the bounds of the (transformed)
// source; destination pixels outside those bounds are left unchanged.
//
// Backend support:
//
//	draw2dimg: all operators are supported natively.
//	draw2dsvg: only CompositeOpOver is supported; other operators degrade
//	           to CompositeOpOver with a logged warning (DrawImage), or are
//	           reported as an error (DrawImageWithOp).
//	draw2dpdf: only CompositeOpOver is supported (gofpdf exposes PDF blend
//	           modes but no Porter-Duff operators); same fallback as svg.
//	draw2dgl:  DrawImage is not implemented.
type CompositeOp int

const (
	// CompositeOpOver draws the source over the destination (the default,
	// matching the historical behavior of DrawImage).
	CompositeOpOver CompositeOp = iota
	// CompositeOpSrc replaces the destination with the source.
	CompositeOpSrc
	// CompositeOpIn keeps the source only where it overlaps the destination.
	CompositeOpIn
	// CompositeOpOut keeps the source only where it does not overlap the destination.
	CompositeOpOut
	// CompositeOpAtop draws the source where it overlaps the destination,
	// keeping the destination elsewhere.
	CompositeOpAtop
	// CompositeOpXor keeps source and destination only where they do not overlap.
	CompositeOpXor
)

// ErrUnsupportedCompositeOp is returned (possibly wrapped) when a
// compositing operation is unknown or not supported by a backend.
var ErrUnsupportedCompositeOp = errors.New("draw2d: unsupported composite operation")

// Valid reports whether op is a known compositing operation.
func (op CompositeOp) Valid() bool {
	return op >= CompositeOpOver && op <= CompositeOpXor
}

func (op CompositeOp) String() string {
	switch op {
	case CompositeOpOver:
		return "over"
	case CompositeOpSrc:
		return "src"
	case CompositeOpIn:
		return "in"
	case CompositeOpOut:
		return "out"
	case CompositeOpAtop:
		return "atop"
	case CompositeOpXor:
		return "xor"
	}
	return "unknown"
}
