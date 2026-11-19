// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"image/color"
	"math"
	"testing"
)

func nrgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: a} }

func TestExtendModeString(t *testing.T) {
	cases := map[ExtendMode]string{
		ExtendPad:     "pad",
		ExtendRepeat:  "repeat",
		ExtendReflect: "reflect",
	}
	for mode, want := range cases {
		if got := mode.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", mode, got, want)
		}
	}
}

func TestLinearGradientNormalize(t *testing.T) {
	g := &LinearGradient{
		X0: 1, Y0: 2, X1: 3, Y1: 4,
		Extend: ExtendReflect,
		Stops: []GradientStop{
			{Offset: 1, Color: nrgba(0, 0, 255, 255)},
			{Offset: math.NaN(), Color: nrgba(1, 2, 3, 255)},
			{Offset: 0, Color: nrgba(255, 0, 0, 255)},
			{Offset: 2, Color: nrgba(0, 255, 0, 255)},
			{Offset: math.Inf(1), Color: nrgba(9, 9, 9, 255)},
			{Offset: -1, Color: nil},
			{Offset: 0.5, Color: nrgba(0, 0, 0, 128)},
		},
	}
	got := g.Normalize()
	if got == nil {
		t.Fatal("Normalize returned nil")
	}
	if got.X0 != 1 || got.Y0 != 2 || got.X1 != 3 || got.Y1 != 4 || got.Extend != ExtendReflect {
		t.Error("axis or extend mode was not preserved")
	}
	wantOffsets := []float64{0, 0, 0.5, 1, 1}
	if len(got.Stops) != len(wantOffsets) {
		t.Fatalf("got %d stops, want %d: %+v", len(got.Stops), len(wantOffsets), got.Stops)
	}
	for i, want := range wantOffsets {
		if got.Stops[i].Offset != want {
			t.Errorf("stop %d offset = %v, want %v", i, got.Stops[i].Offset, want)
		}
	}
	if got.Stops[0].Color != color.Color(nrgba(255, 0, 0, 255)) {
		t.Errorf("nil color stop should sort to offset 0, got %v", got.Stops[0].Color)
	}
	if got.Stops[1].Color != color.Color(nrgba(0, 0, 0, 0)) {
		t.Errorf("nil color should become transparent, got %v", got.Stops[1].Color)
	}
	// The original must not be mutated: NaN offsets survive and the slice
	// length is unchanged.
	if len(g.Stops) != 7 {
		t.Fatalf("Normalize mutated the receiver, %d stops left", len(g.Stops))
	}
	if !math.IsNaN(g.Stops[1].Offset) {
		t.Error("Normalize mutated the receiver's NaN offset")
	}
}

func TestLinearGradientNormalizeNil(t *testing.T) {
	var g *LinearGradient
	if g.Normalize() != nil {
		t.Error("nil gradient Normalize should return nil")
	}
	if g.Ramp() != nil {
		t.Error("nil gradient Ramp should return nil")
	}
	if _, ok := g.ColorAtPoint(0, 0); ok {
		t.Error("nil gradient ColorAtPoint should report !ok")
	}
	if g.IsFinite() {
		t.Error("nil gradient IsFinite should be false")
	}
}

func TestLinearGradientPositionAt(t *testing.T) {
	g := &LinearGradient{X0: 10, Y0: 10, X1: 30, Y1: 10}
	if got := g.PositionAt(10, 10); got != 0 {
		t.Errorf("start position = %v, want 0", got)
	}
	if got := g.PositionAt(20, 50); got != 0.5 {
		t.Errorf("mid position = %v, want 0.5", got)
	}
	if got := g.PositionAt(30, 0); got != 1 {
		t.Errorf("end position = %v, want 1", got)
	}
	zero := &LinearGradient{X0: 5, Y0: 5, X1: 5, Y1: 5}
	if got := zero.PositionAt(5, 5); !math.IsNaN(got) {
		t.Errorf("zero length axis position = %v, want NaN", got)
	}
}

func TestGradientRampPad(t *testing.T) {
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(0, 0, 255, 255)},
		{Offset: 1, Color: nrgba(255, 128, 0, 255)},
	}, ExtendPad)
	check := func(at float64, want color.NRGBA) {
		t.Helper()
		if got := ramp.ColorAt(at); got != want {
			t.Errorf("ColorAt(%v) = %v, want %v", at, got, want)
		}
	}
	check(-10, nrgba(0, 0, 255, 255))
	check(0, nrgba(0, 0, 255, 255))
	check(0.5, nrgba(128, 64, 128, 255))
	check(1, nrgba(255, 128, 0, 255))
	check(10, nrgba(255, 128, 0, 255))
}

func TestGradientRampRepeatAndReflect(t *testing.T) {
	stops := []GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(0, 0, 255, 255)},
	}
	tests := []struct {
		mode ExtendMode
		t    float64
		want color.NRGBA
	}{
		{ExtendRepeat, 0.25, nrgba(191, 0, 64, 255)},
		{ExtendRepeat, 1.25, nrgba(191, 0, 64, 255)},
		{ExtendRepeat, -0.25, nrgba(64, 0, 191, 255)},
		{ExtendRepeat, 0.75, nrgba(64, 0, 191, 255)},
		{ExtendReflect, 0.25, nrgba(191, 0, 64, 255)},
		{ExtendReflect, 0.75, nrgba(64, 0, 191, 255)},
		{ExtendReflect, 1.25, nrgba(64, 0, 191, 255)},
		{ExtendReflect, 1.75, nrgba(191, 0, 64, 255)},
		{ExtendReflect, -0.25, nrgba(191, 0, 64, 255)},
		{ExtendReflect, 2.25, nrgba(191, 0, 64, 255)},
	}
	for _, tc := range tests {
		ramp := NewGradientRamp(stops, tc.mode)
		if got := ramp.ColorAt(tc.t); got != tc.want {
			t.Errorf("%v ColorAt(%v) = %v, want %v", tc.mode, tc.t, got, tc.want)
		}
	}
}

func TestGradientRampSeamContinuity(t *testing.T) {
	// With repeat, the color just before a seam must approach the last stop
	// and just after it the first stop; the seam itself is a hard jump.
	stops := []GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(0, 0, 255, 255)},
	}
	repeat := NewGradientRamp(stops, ExtendRepeat)
	before := repeat.ColorAt(1 - 1e-9)
	after := repeat.ColorAt(1 + 1e-9)
	if before != nrgba(0, 0, 255, 255) {
		t.Errorf("just before the seam = %v, want last stop", before)
	}
	if after != nrgba(255, 0, 0, 255) {
		t.Errorf("just after the seam = %v, want first stop", after)
	}

	reflect := NewGradientRamp(stops, ExtendReflect)
	before = reflect.ColorAt(1 - 1e-9)
	after = reflect.ColorAt(1 + 1e-9)
	if before != nrgba(0, 0, 255, 255) || after != nrgba(0, 0, 255, 255) {
		t.Errorf("reflect seam should be continuous, got %v then %v", before, after)
	}
	if got := reflect.ColorAt(2); got != nrgba(255, 0, 0, 255) {
		t.Errorf("reflect at 2 = %v, want first stop", got)
	}
}

func TestGradientRampPremultipliedBlend(t *testing.T) {
	// Blending opaque red into fully transparent white at 50% must give a
	// half alpha red, not a washed out pink.
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(255, 255, 255, 0)},
	}, ExtendPad)
	got := ramp.ColorAt(0.5)
	if got.A != 128 {
		t.Fatalf("alpha = %d, want 128", got.A)
	}
	if got.R != 255 || got.G != 0 || got.B != 0 {
		t.Errorf("premultiplied blend = %v, want opaque red hue at half alpha", got)
	}
}

func TestGradientRampFullyTransparentStops(t *testing.T) {
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: color.NRGBA{}},
		{Offset: 1, Color: color.NRGBA{}},
	}, ExtendPad)
	for _, at := range []float64{0, 0.5, 1} {
		if got := ramp.ColorAt(at); got.A != 0 {
			t.Errorf("transparent gradient sample at %v = %v, want alpha 0", at, got)
		}
	}
}

func TestGradientRampDuplicateOffsets(t *testing.T) {
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 0.5, Color: nrgba(0, 255, 0, 255)},
		{Offset: 0.5, Color: nrgba(0, 0, 255, 255)},
		{Offset: 1, Color: nrgba(255, 255, 255, 255)},
	}, ExtendPad)
	if got := ramp.ColorAt(0.5); got != nrgba(0, 0, 255, 255) {
		t.Errorf("duplicate offset should use the later stop, got %v", got)
	}
	if got := ramp.ColorAt(0.25); got != nrgba(128, 128, 0, 255) {
		t.Errorf("first segment = %v, want yellow", got)
	}
}

func TestGradientRampNonFiniteInput(t *testing.T) {
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(0, 0, 255, 255)},
	}, ExtendPad)
	if got := ramp.ColorAt(math.NaN()); got != nrgba(255, 0, 0, 255) {
		t.Errorf("NaN should map to first stop, got %v", got)
	}
	if got := ramp.ColorAt(math.Inf(1)); got != nrgba(0, 0, 255, 255) {
		t.Errorf("+Inf should map to last stop, got %v", got)
	}
	if got := ramp.ColorAt(math.Inf(-1)); got != nrgba(255, 0, 0, 255) {
		t.Errorf("-Inf should map to first stop, got %v", got)
	}

	if r := NewGradientRamp(nil, ExtendPad); r != nil {
		t.Error("empty stops should produce a nil ramp")
	}
	if r := NewGradientRamp([]GradientStop{{Offset: math.NaN(), Color: color.White}}, ExtendPad); r != nil {
		t.Error("all non-finite offsets should produce a nil ramp")
	}
	var r *GradientRamp
	if got := r.ColorAt(0.5); got.A != 0 || got.R != 0 {
		t.Errorf("nil ramp ColorAt = %v, want transparent", got)
	}
}

func TestGradientRampAccessors(t *testing.T) {
	r := NewGradientRamp([]GradientStop{
		{Offset: 0.25, Color: nrgba(1, 2, 3, 4)},
		{Offset: 0.75, Color: nrgba(5, 6, 7, 8)},
	}, ExtendRepeat)
	if r.Len() != 2 {
		t.Errorf("Len() = %d, want 2", r.Len())
	}
	if got := r.Offset(1); got != 0.75 {
		t.Errorf("Offset(1) = %v, want 0.75", got)
	}
	if got := r.Color(0); got != nrgba(1, 2, 3, 4) {
		t.Errorf("Color(0) = %v", got)
	}
	if got := r.FirstColor(); got != nrgba(1, 2, 3, 4) {
		t.Errorf("FirstColor() = %v", got)
	}
	if got := r.LastColor(); got != nrgba(5, 6, 7, 8) {
		t.Errorf("LastColor() = %v", got)
	}
	if r.Extend() != ExtendRepeat {
		t.Errorf("Extend() = %v", r.Extend())
	}
	if r.Offset(-1) != 0 || r.Offset(5) != 0 {
		t.Error("out of range Offset should return 0")
	}
	if r.Color(-1) != (color.NRGBA{}) || r.Color(5) != (color.NRGBA{}) {
		t.Error("out of range Color should return transparent")
	}
}

func TestLinearGradientColorAt(t *testing.T) {
	g := &LinearGradient{
		X0: 0, Y0: 0, X1: 100, Y1: 0,
		Stops: []GradientStop{
			{Offset: 0, Color: nrgba(0, 0, 255, 255)},
			{Offset: 0.5, Color: nrgba(0, 255, 0, 255)},
			{Offset: 1, Color: nrgba(255, 128, 0, 255)},
		},
	}
	if got := g.ColorAt(0.5); got != nrgba(0, 255, 0, 255) {
		t.Errorf("ColorAt(0.5) = %v, want the middle stop", got)
	}
	c, ok := g.ColorAtPoint(50, 1000)
	if !ok || c != nrgba(0, 255, 0, 255) {
		t.Errorf("ColorAtPoint(50, 1000) = %v, %v", c, ok)
	}
}

func TestLinearGradientZeroLengthAxis(t *testing.T) {
	g := &LinearGradient{
		X0: 5, Y0: 5, X1: 5, Y1: 5,
		Stops: []GradientStop{
			{Offset: 0, Color: nrgba(255, 0, 0, 255)},
			{Offset: 1, Color: nrgba(0, 0, 255, 255)},
		},
	}
	c, ok := g.ColorAtPoint(100, 100)
	if !ok {
		t.Fatal("zero length axis should still be paintable")
	}
	if c != nrgba(0, 0, 255, 255) {
		t.Errorf("zero length axis = %v, want last stop (SVG rule)", c)
	}
}

func TestLinearGradientNonFiniteAxis(t *testing.T) {
	for _, g := range []*LinearGradient{
		{X0: math.NaN(), Y1: 1},
		{X1: math.Inf(1)},
		{Y0: math.Inf(-1)},
	} {
		g.Stops = []GradientStop{{Offset: 0, Color: color.White}, {Offset: 1, Color: color.Black}}
		if g.IsFinite() {
			t.Error("gradient with non-finite axis should not be finite")
		}
		if _, ok := g.ColorAtPoint(0, 0); ok {
			t.Error("non-finite axis should report !ok")
		}
	}
}

func TestLinearGradientNoStops(t *testing.T) {
	g := &LinearGradient{X0: 0, Y0: 0, X1: 1, Y1: 0}
	if _, ok := g.ColorAtPoint(0, 0); ok {
		t.Error("gradient without stops should report !ok")
	}
	if g.Ramp() != nil {
		t.Error("gradient without stops should have a nil ramp")
	}
}

func TestLinearGradientStopsAreSnapshotted(t *testing.T) {
	stops := []GradientStop{{Offset: 0, Color: nrgba(255, 0, 0, 255)}, {Offset: 1, Color: nrgba(0, 0, 255, 255)}}
	g := NewLinearGradient(0, 0, 1, 0, stops)
	norm := g.Normalize()
	stops[0].Color = nrgba(0, 255, 0, 255)
	if got := norm.Stops[0].Color; got != color.Color(nrgba(255, 0, 0, 255)) {
		t.Errorf("Normalize should snapshot stops, got %v", got)
	}
}

func TestGradientRampPremultipliedSymmetry(t *testing.T) {
	// Reversing the stops and sampling at 1-t must produce the same color,
	// proving the blend is not order dependent.
	forward := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(10, 200, 30, 40)},
		{Offset: 1, Color: nrgba(220, 20, 90, 200)},
	}, ExtendPad)
	reverse := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(220, 20, 90, 200)},
		{Offset: 1, Color: nrgba(10, 200, 30, 40)},
	}, ExtendPad)
	for _, at := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		a := forward.ColorAt(at)
		b := reverse.ColorAt(1 - at)
		if !nearEqualColor(a, b, 1) {
			t.Errorf("at %v: forward %v vs reversed %v", at, a, b)
		}
	}
}

func nearEqualColor(a, b color.NRGBA, tol int) bool {
	diff := func(x, y uint8) bool {
		d := int(x) - int(y)
		return d <= tol && d >= -tol
	}
	return diff(a.R, b.R) && diff(a.G, b.G) && diff(a.B, b.B) && diff(a.A, b.A)
}

func TestGradientRampSingleStop(t *testing.T) {
	ramp := NewGradientRamp([]GradientStop{{Offset: 0.5, Color: nrgba(1, 2, 3, 4)}}, ExtendPad)
	for _, at := range []float64{0, 0.5, 1, 5} {
		if got := ramp.ColorAt(at); got != nrgba(1, 2, 3, 4) {
			t.Errorf("single stop gradient at %v = %v", at, got)
		}
	}
}

func TestGradientRampHugeParameter(t *testing.T) {
	// Very large parameters must not overflow into NaN.
	ramp := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(0, 0, 255, 255)},
	}, ExtendRepeat)
	for _, at := range []float64{1e18, 1e18 + 0.25, -1e18} {
		got := ramp.ColorAt(at)
		if got.A != 255 || (got.R == 0 && got.G == 0 && got.B == 0) {
			t.Errorf("ColorAt(%v) = %v, want a valid blend", at, got)
		}
	}
	pad := NewGradientRamp([]GradientStop{
		{Offset: 0, Color: nrgba(255, 0, 0, 255)},
		{Offset: 1, Color: nrgba(0, 0, 255, 255)},
	}, ExtendPad)
	if got := pad.ColorAt(1e300); got != nrgba(0, 0, 255, 255) {
		t.Errorf("pad at huge t = %v, want last stop", got)
	}
}

func TestLinearGradientUsable(t *testing.T) {
	cases := map[string]struct {
		g    *LinearGradient
		want bool
	}{
		"valid":            {&LinearGradient{X1: 1, Stops: []GradientStop{{Offset: 0, Color: color.White}}}, true},
		"nil":              {nil, false},
		"no stops":         {&LinearGradient{X1: 1}, false},
		"non-finite axis":  {&LinearGradient{X1: math.Inf(1), Stops: []GradientStop{{Offset: 0, Color: color.White}}}, false},
		"non-finite stops": {&LinearGradient{X1: 1, Stops: []GradientStop{{Offset: math.NaN(), Color: color.White}}}, false},
	}
	for name, tc := range cases {
		if got := tc.g.Usable(); got != tc.want {
			t.Errorf("%s: Usable() = %v, want %v", name, got, tc.want)
		}
	}
}
