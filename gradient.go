// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"image/color"
	"math"
	"sort"
)

// SpreadMethod defines how a gradient fills the area outside its
// nominal [0, 1] parameter range.
type SpreadMethod int

const (
	// PadSpread clamps the gradient parameter to [0, 1], extending the
	// first and last stop colors to infinity.
	PadSpread SpreadMethod = iota
	// RepeatSpread repeats the gradient periodically: the parameter is
	// wrapped into [0, 1) by taking its fractional part.
	RepeatSpread
	// ReflectSpread mirrors the gradient periodically: the parameter is
	// folded back and forth between 0 and 1.
	ReflectSpread
)

// GradientStop associates a color with a position along a gradient.
// Position is expected in [0, 1]; out-of-range positions are clamped
// when the gradient is evaluated.
type GradientStop struct {
	Position float64
	Color    color.Color
}

// RadialGradient is a fill pattern that interpolates colors radially,
// from a focal point (parameter 0) out to the circle centered at
// (Cx, Cy) with the given Radius (parameter 1). When the focal point
// equals the center the gradient forms concentric circles; an offset
// focus shifts the highlight, which is what gives chart backgrounds a
// soft center-glow without a visible gradient direction.
//
// All coordinates are expressed in the user space in place when the
// gradient is set on a graphic context; the gradient then follows the
// context's transformation matrix like any path geometry.
type RadialGradient struct {
	// Cx, Cy is the center of the outer circle (parameter 1).
	Cx, Cy float64
	// Fx, Fy is the focal point (parameter 0). If it lies outside the
	// outer circle it is clamped onto the circle, as in SVG.
	Fx, Fy float64
	// Radius is the radius of the outer circle. A zero radius paints
	// the whole area with the last stop color.
	Radius float64
	// Spread selects the PadSpread, RepeatSpread or ReflectSpread
	// behavior outside the [0, 1] parameter range.
	Spread SpreadMethod
	// Stops are the color stops. They do not need to be added in
	// order; they are sorted by position when the gradient is used.
	Stops []GradientStop
}

// NewRadialGradient creates a radial gradient with the given outer
// circle and focal point, using PadSpread and no color stops.
func NewRadialGradient(cx, cy, radius, fx, fy float64) *RadialGradient {
	return &RadialGradient{
		Cx:     cx,
		Cy:     cy,
		Fx:     fx,
		Fy:     fy,
		Radius: radius,
		Spread: PadSpread,
	}
}

// AddColorStop appends a color stop at the given position.
// Stops may be added in any order and at any position; positions are
// clamped to [0, 1] and stops are sorted when the gradient is evaluated.
func (g *RadialGradient) AddColorStop(position float64, c color.Color) {
	g.Stops = append(g.Stops, GradientStop{Position: position, Color: c})
}

// IsValid reports whether the gradient can be rendered. A gradient is
// valid when all its geometry is finite, its radius is non-negative and
// it has at least one stop. Invalid gradients paint nothing
// (fully transparent).
func (g *RadialGradient) IsValid() bool {
	if g == nil || len(g.Stops) == 0 || g.Radius < 0 {
		return false
	}
	for _, v := range []float64{g.Cx, g.Cy, g.Fx, g.Fy, g.Radius} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// Focus returns the effective focal point: the configured focus clamped
// onto the outer circle if it lies outside, matching SVG behavior.
func (g *RadialGradient) Focus() (fx, fy float64) {
	fx, fy = g.Fx, g.Fy
	dx, dy := fx-g.Cx, fy-g.Cy
	d := math.Hypot(dx, dy)
	if g.Radius > 0 && d > g.Radius {
		fx = g.Cx + dx*g.Radius/d
		fy = g.Cy + dy*g.Radius/d
	}
	return fx, fy
}

// NormalizedStops returns the stops sorted by ascending position with
// positions clamped to [0, 1]. The sort is stable, so stops sharing a
// position keep their insertion order and produce a hard edge.
func (g *RadialGradient) NormalizedStops() []GradientStop {
	stops := make([]GradientStop, len(g.Stops))
	copy(stops, g.Stops)
	for i := range stops {
		stops[i].Position = clamp01(stops[i].Position)
	}
	sort.SliceStable(stops, func(i, j int) bool {
		return stops[i].Position < stops[j].Position
	})
	return stops
}

// ColorAt returns the gradient color at parameter t. The spread method
// is applied to t first, so any real t is accepted. A NaN t, an invalid
// gradient or an empty stop list yields a fully transparent color.
func (g *RadialGradient) ColorAt(t float64) color.RGBA {
	r, gg, b, a := g.ColorAt16(t)
	return color.RGBA{uint8(r >> 8), uint8(gg >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// ColorAt16 is like ColorAt but returns premultiplied 16-bit channels,
// which is what raster backends composite with.
func (g *RadialGradient) ColorAt16(t float64) (r, gg, b, a uint32) {
	if !g.IsValid() || math.IsNaN(t) {
		return 0, 0, 0, 0
	}
	if g.Radius == 0 {
		// Degenerate circle: the whole plane is past the end of the
		// gradient, paint it with the last stop color (as SVG does).
		stops := g.NormalizedStops()
		return rgba16(stops[len(stops)-1].Color)
	}
	t = g.spread(t)
	stops := g.NormalizedStops()
	if t <= stops[0].Position {
		return rgba16(stops[0].Color)
	}
	last := stops[len(stops)-1]
	if t >= last.Position {
		return rgba16(last.Color)
	}
	for i := 0; i+1 < len(stops); i++ {
		s0, s1 := stops[i], stops[i+1]
		// Half-open intervals: at a shared position the later stop
		// wins, which makes duplicate positions a hard edge.
		if t < s0.Position || t >= s1.Position {
			continue
		}
		r0, g0, b0, a0 := rgba16(s0.Color)
		if s1.Position == s0.Position {
			return rgba16(s1.Color)
		}
		u := (t - s0.Position) / (s1.Position - s0.Position)
		r1, g1, b1, a1 := rgba16(s1.Color)
		lerp := func(c0, c1 uint32) uint32 {
			return uint32(float64(c0) + u*(float64(c1)-float64(c0)) + 0.5)
		}
		return lerp(r0, r1), lerp(g0, g1), lerp(b0, b1), lerp(a0, a1)
	}
	return rgba16(last.Color)
}

// spread maps any finite parameter into [0, 1] according to the
// gradient's spread method.
func (g *RadialGradient) spread(t float64) float64 {
	switch g.Spread {
	case RepeatSpread:
		t = t - math.Floor(t)
		if t >= 1 {
			t = 0
		}
		return t
	case ReflectSpread:
		t = math.Abs(t)
		t = t - 2*math.Floor(t/2)
		if t > 1 {
			t = 2 - t
		}
		return t
	default: // PadSpread
		return clamp01(t)
	}
}

// Parameter returns the raw gradient parameter (before the spread
// method is applied) for the user-space point (x, y). The point where
// the ray from the focus through (x, y) meets the outer circle has
// parameter 1, the focus has parameter 0. Non-finite input or geometry
// yields NaN.
func (g *RadialGradient) Parameter(x, y float64) float64 {
	if !g.IsValid() || math.IsNaN(x) || math.IsNaN(y) ||
		math.IsInf(x, 0) || math.IsInf(y, 0) {
		return math.NaN()
	}
	if g.Radius == 0 {
		return math.Inf(1)
	}
	fx, fy := g.Focus()
	dx, dy := g.Cx-fx, g.Cy-fy
	vx, vy := x-fx, y-fy
	// Solve |v - t*d|^2 = (t*r)^2, i.e. A t^2 - 2B t + C = 0.
	a := dx*dx + dy*dy - g.Radius*g.Radius
	b := vx*dx + vy*dy
	c := vx*vx + vy*vy
	if a == 0 {
		// Focus exactly on the outer circle: linear equation.
		if c == 0 {
			// the point is the focus itself
			return 0
		}
		if b <= 0 {
			return math.Inf(1)
		}
		return c / (2 * b)
	}
	disc := b*b - a*c
	if disc < 0 {
		return math.NaN()
	}
	s := math.Sqrt(disc)
	t1, t2 := (b+s)/a, (b-s)/a
	if t1 > t2 {
		return t1
	}
	return t2
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// rgba16 returns the premultiplied 16-bit channels of c.
func rgba16(c color.Color) (r, g, b, a uint32) {
	if c == nil {
		return 0, 0, 0, 0
	}
	return c.RGBA()
}
