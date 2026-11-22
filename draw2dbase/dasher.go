// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 13/12/2010 by Laurent Le Goff

package draw2dbase

import (
	"errors"
	"math"
)

// ErrInvalidDash is returned when a dash pattern contains non-finite or
// negative lengths or a non-finite offset.
var ErrInvalidDash = errors.New("draw2dbase: invalid line dash pattern")

// NormalizeDashPattern converts a dash pattern to the semantics shared by
// the pdf and svg specifications:
//   - an empty pattern means a solid line;
//   - an odd number of entries is duplicated so the pattern repeats with
//     alternating dashes and gaps (a single entry [d] becomes [d d]);
//   - zero entries are dropped, an all-zero pattern becomes solid;
//   - the offset is reduced into [0, patternLength), a negative offset
//     being shifted forward by whole pattern cycles.
//
// The returned boolean is false for a solid line. Non-finite or negative
// lengths and non-finite offsets are invalid and reported as an error.
func NormalizeDashPattern(dash []float64, dashOffset float64) (normalized []float64, offset float64, dashed bool, err error) {
	return normalizeDashPattern(dash, dashOffset)
}

// normalizeDashPattern converts a dash pattern to the semantics shared by
// the pdf and svg specifications:
//   - an empty pattern means a solid line;
//   - an odd number of entries is duplicated so the pattern repeats with
//     alternating dashes and gaps (a single entry [d] becomes [d d]);
//   - zero entries are dropped, an all-zero pattern becomes solid;
//   - the offset is reduced into [0, patternLength), a negative offset
//     being shifted forward by whole pattern cycles.
//
// The returned boolean is false for a solid line. Non-finite or negative
// lengths and non-finite offsets are invalid and reported as an error.
func normalizeDashPattern(dash []float64, dashOffset float64) (normalized []float64, offset float64, dashed bool, err error) {
	if math.IsNaN(dashOffset) || math.IsInf(dashOffset, 0) {
		return nil, 0, false, ErrInvalidDash
	}
	if len(dash) == 0 {
		return nil, 0, false, nil
	}
	entries := make([]float64, 0, len(dash)*2)
	var patternLength float64
	for _, length := range dash {
		if math.IsNaN(length) || math.IsInf(length, 0) || length < 0 {
			return nil, 0, false, ErrInvalidDash
		}
		if length == 0 {
			continue
		}
		entries = append(entries, length)
		patternLength += length
	}
	if len(entries) == 0 || patternLength == 0 {
		return nil, 0, false, nil
	}
	if len(entries)%2 == 1 {
		entries = append(entries, entries...)
		patternLength *= 2
	}
	offset = math.Mod(dashOffset, patternLength)
	if offset < 0 {
		offset += patternLength
	}
	if offset == 0 {
		// avoid a negative zero leaking into the rendered output
		offset = 0
	}
	return entries, offset, true, nil
}

// DashVertexConverter splits flattened path segments into dash and gap
// pieces according to the current line dash pattern. It implements the
// dash semantics of the pdf and svg specifications.
type DashVertexConverter struct {
	next        Flattener
	x, y        float64
	dash        []float64
	currentDash int
	// distance is the already consumed length of the active dash entry
	distance   float64
	dashOffset float64
	// drawing reports whether the active entry is a dash (even index) or
	// a gap (odd index)
	drawing bool
}

// NewDashConverter creates a dash converter around the provided flattener.
// The pattern and offset follow the same normalization rules as the pdf
// line dash operator. Invalid patterns fall back to a solid line.
func NewDashConverter(dash []float64, dashOffset float64, flattener Flattener) *DashVertexConverter {
	var dasher DashVertexConverter
	normalized, offset, dashed, err := normalizeDashPattern(dash, dashOffset)
	if err != nil || !dashed {
		// solid line: a single dash entry longer than any path
		normalized = []float64{math.Inf(1)}
		offset = 0
	}
	dasher.dash = normalized
	dasher.dashOffset = offset
	dasher.next = flattener
	return &dasher
}

func (dasher *DashVertexConverter) LineTo(x, y float64) {
	dasher.lineTo(x, y)
}

func (dasher *DashVertexConverter) MoveTo(x, y float64) {
	dasher.next.MoveTo(x, y)
	dasher.x, dasher.y = x, y
	// Every subpath restarts the dash pattern at the configured offset.
	dasher.resetOffset()
}

// resetOffset positions the converter at the dash entry that contains the
// configured dash offset.
func (dasher *DashVertexConverter) resetOffset() {
	phase := dasher.dashOffset
	dasher.currentDash = 0
	for phase >= dasher.dash[dasher.currentDash] {
		phase -= dasher.dash[dasher.currentDash]
		dasher.currentDash = (dasher.currentDash + 1) % len(dasher.dash)
	}
	dasher.distance = phase
	dasher.drawing = dasher.currentDash%2 == 0
}

func (dasher *DashVertexConverter) LineJoin() {
	if dasher.drawing {
		dasher.next.LineJoin()
	}
}

func (dasher *DashVertexConverter) Close() {
	if dasher.drawing {
		dasher.next.Close()
	}
}

func (dasher *DashVertexConverter) End() {
	dasher.next.End()
}

func (dasher *DashVertexConverter) lineTo(x, y float64) {
	d := distance(dasher.x, dasher.y, x, y)
	for d > 0 {
		rest := dasher.dash[dasher.currentDash] - dasher.distance
		if rest <= 0 {
			// the active entry is fully consumed, switch to the next one
			dasher.advance()
			continue
		}
		if d <= rest {
			// the whole segment stays inside the active entry; an
			// exact boundary only consumes the entry without emitting
			// a zero sized gap or dash
			if dasher.drawing {
				dasher.next.LineTo(x, y)
			}
			dasher.distance += d
			if dasher.distance >= dasher.dash[dasher.currentDash] {
				dasher.advance()
			}
			d = 0
			break
		}
		// the entry boundary lies strictly inside this segment
		k := rest / d
		bx := dasher.x + k*(x-dasher.x)
		by := dasher.y + k*(y-dasher.y)
		if dasher.drawing {
			dasher.next.LineTo(bx, by)
		} else {
			// restart the subpath at the boundary
			dasher.next.End()
			dasher.next.MoveTo(bx, by)
		}
		d -= rest
		dasher.x, dasher.y = bx, by
		dasher.advance()
	}
	dasher.x, dasher.y = x, y
}

// advance moves to the next dash entry and updates the drawing state.
func (dasher *DashVertexConverter) advance() {
	dasher.currentDash = (dasher.currentDash + 1) % len(dasher.dash)
	dasher.distance = 0
	dasher.drawing = dasher.currentDash%2 == 0
}

func distance(x1, y1, x2, y2 float64) float64 {
	return vectorDistance(x2-x1, y2-y1)
}
