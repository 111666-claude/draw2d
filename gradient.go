// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"image/color"
	"math"
	"sort"
)

// ExtendMode defines how a gradient behaves for positions outside the
// [0, 1] interval covered by its stops.
type ExtendMode int

const (
	// ExtendPad extends the first and last stop colors beyond the ends of the
	// gradient axis. This is the default and matches SVG's "pad" spreadMethod.
	ExtendPad ExtendMode = iota
	// ExtendRepeat repeats the stop sequence, so that t and t+1 sample the
	// same color. The color jumps back to the first stop at every seam.
	ExtendRepeat
	// ExtendReflect mirrors the stop sequence, alternating between forward and
	// backward runs.
	ExtendReflect
)

// String returns the SVG spreadMethod name for the extend mode.
func (e ExtendMode) String() string {
	switch e {
	case ExtendRepeat:
		return "repeat"
	case ExtendReflect:
		return "reflect"
	default:
		return "pad"
	}
}

// GradientStop associates a color with a position along a gradient axis.
//
// Offset is normalized: 0 is the start of the axis and 1 its end. Offsets
// outside that range are clamped and non-finite offsets are ignored. Stops do
// not have to be supplied in order; they are sorted by offset when the
// gradient is prepared. Two stops may share an offset, which produces a hard
// color transition at that position.
type GradientStop struct {
	Offset float64
	Color  color.Color
}

// LinearGradient describes a linear gradient brush in user space.
//
// The gradient axis goes from (X0, Y0) to (X1, Y1). Coordinates are expressed
// in the user coordinate system and are affected by the current transformation
// matrix in the same way as path coordinates, exactly like an SVG gradient
// with gradientUnits="userSpaceOnUse". The brush can be installed with
// GraphicContext.SetFillLinearGradient and is then used by Fill and FillStroke.
//
// A LinearGradient is treated as an immutable snapshot by the backends:
// SetFillLinearGradient copies and normalizes it, so later changes to the
// struct do not affect an already installed brush.
//
// Degenerate cases are handled explicitly:
//   - a zero length axis paints the last stop color (the SVG rule for a
//     linear gradient whose start and end points coincide);
//   - a gradient with no usable stop (nil, or every offset non-finite) is
//     ignored and the solid fill color is used;
//   - non-finite axis coordinates make the brush unusable and the solid fill
//     color is used;
//   - a fully transparent stop paints transparency, it does not fall back to
//     an opaque color.
type LinearGradient struct {
	X0, Y0, X1, Y1 float64
	Stops          []GradientStop
	Extend         ExtendMode
}

// NewLinearGradient creates a linear gradient from (x0, y0) to (x1, y1) with
// the given stops and pad extend mode.
func NewLinearGradient(x0, y0, x1, y1 float64, stops []GradientStop) *LinearGradient {
	return &LinearGradient{X0: x0, Y0: y0, X1: x1, Y1: y1, Stops: stops}
}

// Normalize returns a copy of g with stops clamped to [0, 1], non-finite
// offsets dropped, nil colors replaced by transparent, and stops stably sorted
// by offset. The axis and extend mode are copied unchanged. Normalize returns
// nil when g is nil.
func (g *LinearGradient) Normalize() *LinearGradient {
	if g == nil {
		return nil
	}
	out := &LinearGradient{
		X0:     g.X0,
		Y0:     g.Y0,
		X1:     g.X1,
		Y1:     g.Y1,
		Extend: g.Extend,
	}
	out.Stops = normalizeStops(g.Stops)
	return out
}

// normalizeStops applies the documented clamping, filtering and ordering to a
// stop slice. It always returns a new slice.
func normalizeStops(stops []GradientStop) []GradientStop {
	normalized := make([]GradientStop, 0, len(stops))
	for _, s := range stops {
		if math.IsNaN(s.Offset) || math.IsInf(s.Offset, 0) {
			continue
		}
		offset := s.Offset
		if offset < 0 {
			offset = 0
		} else if offset > 1 {
			offset = 1
		}
		c := s.Color
		if c == nil {
			c = color.NRGBA{}
		}
		normalized = append(normalized, GradientStop{Offset: offset, Color: c})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Offset < normalized[j].Offset
	})
	return normalized
}

// IsFinite reports whether the gradient axis coordinates are all finite.
// It does not inspect the stops; see Ramp for that.
func (g *LinearGradient) IsFinite() bool {
	if g == nil {
		return false
	}
	return isFiniteFloat(g.X0) && isFiniteFloat(g.Y0) &&
		isFiniteFloat(g.X1) && isFiniteFloat(g.Y1)
}

func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// Ramp returns a prepared, normalized sampling ramp for the gradient, or nil
// when the gradient has no usable stop.
func (g *LinearGradient) Ramp() *GradientRamp {
	if g == nil {
		return nil
	}
	return NewGradientRamp(g.Stops, g.Extend)
}

// Usable reports whether the gradient can be painted: its axis is finite and it
// has at least one stop with a finite offset. When it returns false, backends
// fall back to the solid fill color.
func (g *LinearGradient) Usable() bool {
	if !g.IsFinite() {
		return false
	}
	for _, s := range g.Stops {
		if isFiniteFloat(s.Offset) {
			return true
		}
	}
	return false
}

// ColorAt returns the gradient color at parameter t. Extend mode handling is
// applied. It returns transparent when the gradient has no usable stop.
//
// Interpolation is performed in premultiplied alpha, matching SVG gradients
// and browsers: a transparent stop does not darken the colors it is blended
// with.
func (g *LinearGradient) ColorAt(t float64) color.NRGBA {
	ramp := g.Ramp()
	if ramp == nil {
		return color.NRGBA{}
	}
	return ramp.ColorAt(t)
}

// PositionAt returns the normalized position of the user space point (x, y)
// projected on the gradient axis. It returns NaN for a zero length axis.
//
// The projection is computed with a unit direction vector so it stays finite
// for very large (but finite) coordinates instead of overflowing dx*dx+dy*dy.
func (g *LinearGradient) PositionAt(x, y float64) float64 {
	if g == nil {
		return math.NaN()
	}
	dx := g.X1 - g.X0
	dy := g.Y1 - g.Y0
	length := math.Hypot(dx, dy)
	if length == 0 {
		return math.NaN()
	}
	if math.IsInf(length, 0) || math.IsNaN(length) {
		// Subtraction of two huge finite coordinates overflowed; fall back to
		// the direct formula, which is still finite or NaN, never a panic.
		return ((x-g.X0)*dx + (y-g.Y0)*dy) / (dx*dx + dy*dy)
	}
	ux := dx / length
	uy := dy / length
	return ((x-g.X0)*ux + (y-g.Y0)*uy) / length
}

// ColorAtPoint returns the color of the gradient at the user space point
// (x, y), applying the extend mode. ok is false when the gradient cannot be
// painted (non-finite axis or no usable stop); callers should then fall back
// to the solid fill color.
func (g *LinearGradient) ColorAtPoint(x, y float64) (c color.NRGBA, ok bool) {
	if !g.IsFinite() {
		return color.NRGBA{}, false
	}
	ramp := g.Ramp()
	if ramp == nil {
		return color.NRGBA{}, false
	}
	t := g.PositionAt(x, y)
	if math.IsNaN(t) {
		// Zero length axis: SVG paints the last stop color.
		return ramp.LastColor(), true
	}
	return ramp.ColorAt(t), true
}

// GradientRamp is a normalized, immutable stop sequence used to sample a
// gradient. It is safe for concurrent use.
//
// Colors are stored as color.NRGBA. Sampling interpolates premultiplied alpha
// so that transparent stops do not introduce black fringes.
type GradientRamp struct {
	offsets []float64
	colors  []color.NRGBA
	extend  ExtendMode
}

// NewGradientRamp normalizes stops and returns a ramp, or nil if no stop has a
// finite offset.
func NewGradientRamp(stops []GradientStop, extend ExtendMode) *GradientRamp {
	normalized := normalizeStops(stops)
	if len(normalized) == 0 {
		return nil
	}
	r := &GradientRamp{
		offsets: make([]float64, len(normalized)),
		colors:  make([]color.NRGBA, len(normalized)),
		extend:  extend,
	}
	for i, s := range normalized {
		r.offsets[i] = s.Offset
		r.colors[i] = color.NRGBAModel.Convert(s.Color).(color.NRGBA)
	}
	return r
}

// Len returns the number of stops in the ramp.
func (r *GradientRamp) Len() int {
	if r == nil {
		return 0
	}
	return len(r.offsets)
}

// Offset returns the normalized offset of stop i. Out of range indexes return 0.
func (r *GradientRamp) Offset(i int) float64 {
	if r == nil || i < 0 || i >= len(r.offsets) {
		return 0
	}
	return r.offsets[i]
}

// Color returns the color of stop i. Out of range indexes return transparent.
func (r *GradientRamp) Color(i int) color.NRGBA {
	if r == nil || i < 0 || i >= len(r.colors) {
		return color.NRGBA{}
	}
	return r.colors[i]
}

// LastColor returns the color of the final stop, or transparent for an empty
// ramp.
func (r *GradientRamp) LastColor() color.NRGBA {
	if r == nil || len(r.colors) == 0 {
		return color.NRGBA{}
	}
	return r.colors[len(r.colors)-1]
}

// FirstColor returns the color of the first stop, or transparent for an empty
// ramp.
func (r *GradientRamp) FirstColor() color.NRGBA {
	if r == nil || len(r.colors) == 0 {
		return color.NRGBA{}
	}
	return r.colors[0]
}

// Extend returns the extend mode used by the ramp.
func (r *GradientRamp) Extend() ExtendMode {
	if r == nil {
		return ExtendPad
	}
	return r.extend
}

// ColorAt samples the ramp at parameter t after applying the extend mode.
//
// NaN maps to the first stop color, +Inf to the last stop color and -Inf to
// the first stop color, for every extend mode. Colors exactly at a stop
// position are returned unchanged (no interpolation rounding).
func (r *GradientRamp) ColorAt(t float64) color.NRGBA {
	if r == nil || len(r.offsets) == 0 {
		return color.NRGBA{}
	}
	if math.IsInf(t, 1) {
		return r.colors[len(r.colors)-1]
	}
	if math.IsInf(t, -1) {
		return r.colors[0]
	}
	if math.IsNaN(t) {
		return r.colors[0]
	}
	switch r.extend {
	case ExtendRepeat:
		t -= math.Floor(t)
	case ExtendReflect:
		t -= 2 * math.Floor(t/2)
		if t > 1 {
			t = 2 - t
		}
	}
	if t <= r.offsets[0] {
		return r.colors[0]
	}
	last := len(r.offsets) - 1
	if t >= r.offsets[last] {
		return r.colors[last]
	}
	// First stop strictly after t. With duplicate offsets this selects the
	// last stop of the duplicate group, giving a hard transition.
	i := sort.Search(len(r.offsets), func(i int) bool { return r.offsets[i] > t })
	if i <= 0 {
		i = 1
	}
	if i > last {
		i = last
	}
	ao, bo := r.offsets[i-1], r.offsets[i]
	if bo <= ao {
		return r.colors[i]
	}
	u := (t - ao) / (bo - ao)
	return blendPremultiplied(r.colors[i-1], r.colors[i], u)
}

// blendPremultiplied linearly blends two NRGBA colors at fraction u in
// premultiplied space and returns the result in straight NRGBA.
func blendPremultiplied(a, b color.NRGBA, u float64) color.NRGBA {
	if u <= 0 {
		return a
	}
	if u >= 1 {
		return b
	}
	aa := float64(a.A) / 255
	ba := float64(b.A) / 255
	pa := aa + (ba-aa)*u
	if pa <= 0 {
		return color.NRGBA{}
	}
	blend := func(ca, cb uint8, alpha float64) uint8 {
		pa0 := float64(ca) * aa
		pb0 := float64(cb) * ba
		v := (pa0 + (pb0-pa0)*u) / (alpha * 255)
		return clampChannel(v * 255)
	}
	return color.NRGBA{
		R: blend(a.R, b.R, pa),
		G: blend(a.G, b.G, pa),
		B: blend(a.B, b.B, pa),
		A: clampChannel(pa * 255),
	}
}

func clampChannel(v float64) uint8 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}
