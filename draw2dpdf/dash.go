// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf

import "github.com/llgcode/draw2d/draw2dbase"

// maxDashEntries bounds the length of the dash array that is emitted to
// the PDF content stream. It keeps pathological input (for example a slice
// with millions of entries) from bloating the document or stalling a
// conforming reader.
const maxDashEntries = 10000

// ErrInvalidDash is returned by normalizeDash when a dash pattern cannot
// be converted into a PDF compatible one.
var ErrInvalidDash = draw2dbase.ErrInvalidDash

// normalizeDash adapts the shared dash normalization of draw2dbase to the
// limits of the pdf backend. It rejects patterns that would produce an
// oversized dash array in addition to the invalid values (non-finite or
// negative lengths, non-finite offset) already rejected by the shared
// implementation.
func normalizeDash(dash []float64, offset float64) (normalized []float64, normalizedOffset float64, dashed bool, err error) {
	normalized, normalizedOffset, dashed, err = draw2dbase.NormalizeDashPattern(dash, offset)
	if err != nil {
		return nil, 0, false, err
	}
	if dashed && len(normalized) > maxDashEntries {
		return nil, 0, false, ErrInvalidDash
	}
	return normalized, normalizedOffset, dashed, nil
}
