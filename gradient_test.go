// Unit tests for the radial gradient fill pattern:
// stop normalization, spread methods, degenerate geometry.

package draw2d_test

import (
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

func redBlueGradient(spread draw2d.SpreadMethod) *draw2d.RadialGradient {
	g := draw2d.NewRadialGradient(50, 50, 40, 50, 50)
	g.Spread = spread
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	return g
}

func TestRadialGradientColorAtPad(t *testing.T) {
	g := redBlueGradient(draw2d.PadSpread)
	if c := g.ColorAt(0); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(0) = %v, want red", c)
	}
	if c := g.ColorAt(0.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(0.5) = %v, want purple (128,0,128)", c)
	}
	if c := g.ColorAt(1); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ColorAt(1) = %v, want blue", c)
	}
	// pad clamps outside [0, 1]
	if c := g.ColorAt(-3); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(-3) = %v, want red", c)
	}
	if c := g.ColorAt(7); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ColorAt(7) = %v, want blue", c)
	}
}

func TestRadialGradientStopsOutOfOrder(t *testing.T) {
	g := draw2d.NewRadialGradient(0, 0, 10, 0, 0)
	// stops added in reverse order must behave like sorted stops
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	if c := g.ColorAt(0); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(0) = %v, want red", c)
	}
	if c := g.ColorAt(1); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ColorAt(1) = %v, want blue", c)
	}
	if c := g.ColorAt(0.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(0.5) = %v, want purple", c)
	}
}

func TestRadialGradientStopPositionsClamped(t *testing.T) {
	g := draw2d.NewRadialGradient(0, 0, 10, 0, 0)
	g.AddColorStop(-0.5, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1.5, color.RGBA{0, 0, 255, 255})
	if c := g.ColorAt(0); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(0) = %v, want red", c)
	}
	if c := g.ColorAt(1); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ColorAt(1) = %v, want blue", c)
	}
}

func TestRadialGradientDuplicateStops(t *testing.T) {
	g := draw2d.NewRadialGradient(0, 0, 10, 0, 0)
	g.AddColorStop(0, color.RGBA{0, 0, 0, 255})
	g.AddColorStop(0.5, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(0.5, color.RGBA{0, 0, 255, 255})
	g.AddColorStop(1, color.RGBA{255, 255, 255, 255})
	// duplicate positions make a hard edge at 0.5
	if c := g.ColorAt(0.4); c != (color.RGBA{204, 0, 0, 255}) {
		t.Errorf("ColorAt(0.4) = %v, want (204,0,0) interpolating black->red", c)
	}
	if c := g.ColorAt(0.5); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("ColorAt(0.5) = %v, want blue", c)
	}
	if c := g.ColorAt(0.6); c != (color.RGBA{51, 51, 255, 255}) {
		t.Errorf("ColorAt(0.6) = %v, want (51,51,255) interpolating blue->white", c)
	}
}

func TestRadialGradientRepeatSpread(t *testing.T) {
	g := redBlueGradient(draw2d.RepeatSpread)
	if c := g.ColorAt(1); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(1) = %v, want red (wrapped to 0)", c)
	}
	if c := g.ColorAt(2.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(2.5) = %v, want purple", c)
	}
	if c := g.ColorAt(-0.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(-0.5) = %v, want purple", c)
	}
}

func TestRadialGradientReflectSpread(t *testing.T) {
	g := redBlueGradient(draw2d.ReflectSpread)
	if c := g.ColorAt(1.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(1.5) = %v, want purple", c)
	}
	if c := g.ColorAt(2); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ColorAt(2) = %v, want red (reflected to 0)", c)
	}
	if c := g.ColorAt(2.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(2.5) = %v, want purple", c)
	}
	if c := g.ColorAt(-0.5); c != (color.RGBA{128, 0, 128, 255}) {
		t.Errorf("ColorAt(-0.5) = %v, want purple", c)
	}
}

func TestRadialGradientZeroRadius(t *testing.T) {
	g := draw2d.NewRadialGradient(50, 50, 0, 50, 50)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	// a zero radius paints the last stop color everywhere
	for _, pos := range []float64{0, 0.3, 1, 100} {
		if c := g.ColorAt(pos); c != (color.RGBA{0, 0, 255, 255}) {
			t.Errorf("ColorAt(%v) = %v, want blue", pos, c)
		}
	}
}

func TestRadialGradientEmptyStops(t *testing.T) {
	g := draw2d.NewRadialGradient(50, 50, 40, 50, 50)
	if g.IsValid() {
		t.Error("gradient without stops should not be valid")
	}
	if c := g.ColorAt(0.5); c != (color.RGBA{0, 0, 0, 0}) {
		t.Errorf("ColorAt(0.5) = %v, want transparent", c)
	}
}

func TestRadialGradientNonFinite(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		g := draw2d.NewRadialGradient(v, 50, 40, 50, 50)
		g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
		if g.IsValid() {
			t.Errorf("gradient with non-finite coordinate %v should not be valid", v)
		}
		if c := g.ColorAt(0.5); c != (color.RGBA{0, 0, 0, 0}) {
			t.Errorf("ColorAt with non-finite geometry = %v, want transparent", c)
		}
	}
	g := redBlueGradient(draw2d.PadSpread)
	if c := g.ColorAt(math.NaN()); c != (color.RGBA{0, 0, 0, 0}) {
		t.Errorf("ColorAt(NaN) = %v, want transparent", c)
	}
}

func TestRadialGradientFocusClamping(t *testing.T) {
	g := draw2d.NewRadialGradient(0, 0, 10, 30, 0)
	fx, fy := g.Focus()
	if fx != 10 || fy != 0 {
		t.Errorf("Focus() = (%v, %v), want (10, 0) clamped onto the circle", fx, fy)
	}
	g = draw2d.NewRadialGradient(0, 0, 10, 3, 4)
	fx, fy = g.Focus()
	if fx != 3 || fy != 4 {
		t.Errorf("Focus() = (%v, %v), want (3, 4) unchanged inside the circle", fx, fy)
	}
}

func TestRadialGradientParameter(t *testing.T) {
	// concentric: parameter is distance / radius
	g := draw2d.NewRadialGradient(0, 0, 10, 0, 0)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	for _, pt := range [][3]float64{{5, 0, 0.5}, {10, 0, 1}, {20, 0, 2}, {0, 0, 0}} {
		if got := g.Parameter(pt[0], pt[1]); got != pt[2] {
			t.Errorf("Parameter(%v, %v) = %v, want %v", pt[0], pt[1], got, pt[2])
		}
	}
	// eccentric focus: the circle at t=0.5 has center (10,0) radius 20
	g = draw2d.NewRadialGradient(0, 0, 40, 20, 0)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	if got := g.Parameter(30, 0); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("Parameter(30, 0) = %v, want 0.5", got)
	}
	if got := g.Parameter(-10, 0); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("Parameter(-10, 0) = %v, want 0.5", got)
	}
	if got := g.Parameter(20, 0); got != 0 {
		t.Errorf("Parameter at focus = %v, want 0", got)
	}
	// non-finite input and zero radius have defined results
	if got := g.Parameter(math.NaN(), 0); !math.IsNaN(got) {
		t.Errorf("Parameter(NaN, 0) = %v, want NaN", got)
	}
	z := draw2d.NewRadialGradient(0, 0, 0, 0, 0)
	z.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	if got := z.Parameter(1, 1); !math.IsInf(got, 1) {
		t.Errorf("zero radius Parameter = %v, want +Inf", got)
	}
}
