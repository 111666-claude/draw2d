// Pixel-exact fixtures for radial gradient fills on the image backend:
// concentric circles, eccentric focus, transparent stops, repeat seams,
// transforms and nested Save/Restore.

package draw2dimg_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
)

// newCanvas returns a white-opaque RGBA canvas and its graphic context.
func newCanvas(w, h int) (*image.RGBA, *draw2dimg.GraphicContext) {
	dest := image.NewRGBA(image.Rect(0, 0, w, h))
	gc := draw2dimg.NewGraphicContext(dest)
	gc.Clear() // fills with the default white fill color
	return dest, gc
}

// fillAll fills a rectangle larger than the canvas so every pixel is
// painted with full coverage (no antialiased edges).
func fillAll(gc *draw2dimg.GraphicContext, w, h int) {
	gc.BeginPath()
	gc.MoveTo(-10, -10)
	gc.LineTo(float64(w)+10, -10)
	gc.LineTo(float64(w)+10, float64(h)+10)
	gc.LineTo(-10, float64(h)+10)
	gc.Close()
	gc.Fill()
}

func fillRect(gc *draw2dimg.GraphicContext, x0, y0, x1, y1 float64) {
	gc.BeginPath()
	gc.MoveTo(x0, y0)
	gc.LineTo(x1, y0)
	gc.LineTo(x1, y1)
	gc.LineTo(x0, y1)
	gc.Close()
	gc.Fill()
}

func checkPixel(t *testing.T, img *image.RGBA, x, y int, want color.RGBA) {
	t.Helper()
	got := img.RGBAAt(x, y)
	if got != want {
		t.Errorf("pixel (%d, %d) = %v, want %v", x, y, got, want)
	}
}

// redBlue returns a red (t=0) to blue (t=1) gradient on the circle
// centered at (50.5, 50.5) — half-integer coordinates make pixel
// centers land on exact gradient parameters.
func redBlue(radius float64, spread draw2d.SpreadMethod) *draw2d.RadialGradient {
	g := draw2d.NewRadialGradient(50.5, 50.5, radius, 50.5, 50.5)
	g.Spread = spread
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	return g
}

func TestGradientConcentricCircles(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	gc.SetFillGradient(redBlue(40, draw2d.PadSpread))
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 50, 50, color.RGBA{255, 0, 0, 255})   // t = 0
	checkPixel(t, dest, 70, 50, color.RGBA{128, 0, 128, 255}) // t = 0.5
	checkPixel(t, dest, 90, 50, color.RGBA{0, 0, 255, 255})   // t = 1
	checkPixel(t, dest, 95, 50, color.RGBA{0, 0, 255, 255})   // t > 1, padded
	checkPixel(t, dest, 30, 50, color.RGBA{128, 0, 128, 255}) // symmetric
	checkPixel(t, dest, 50, 70, color.RGBA{128, 0, 128, 255}) // concentric on y too
}

func TestGradientEccentricFocus(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	g := redBlue(40, draw2d.PadSpread)
	g.Fx, g.Fy = 70.5, 50.5 // focus shifted 20px right of center
	gc.SetFillGradient(g)
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 70, 50, color.RGBA{255, 0, 0, 255})   // t = 0 at the focus
	checkPixel(t, dest, 90, 50, color.RGBA{0, 0, 255, 255})   // on the circle
	checkPixel(t, dest, 10, 50, color.RGBA{0, 0, 255, 255})   // on the circle
	checkPixel(t, dest, 60, 50, color.RGBA{213, 0, 42, 255})  // t = 1/6
	checkPixel(t, dest, 80, 50, color.RGBA{128, 0, 128, 255}) // t = 1/2
}

func TestGradientFocusOutsideCircle(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	g := redBlue(40, draw2d.PadSpread)
	g.Fx, g.Fy = 200.5, 50.5 // outside: clamped onto the circle at (90.5, 50.5)
	gc.SetFillGradient(g)
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 90, 50, color.RGBA{255, 0, 0, 255}) // clamped focus, t = 0
	checkPixel(t, dest, 10, 50, color.RGBA{0, 0, 255, 255}) // far edge of the circle
}

func TestGradientTransparentStops(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	g := draw2d.NewRadialGradient(50.5, 50.5, 40, 50.5, 50.5)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.NRGBA{0, 0, 255, 0}) // fades to transparent
	gc.SetFillGradient(g)
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 50, 50, color.RGBA{255, 0, 0, 255})     // opaque red
	checkPixel(t, dest, 70, 50, color.RGBA{255, 127, 127, 255}) // 50% red over white
	checkPixel(t, dest, 90, 50, color.RGBA{255, 255, 255, 255}) // fully transparent
}

func TestGradientRepeatSeam(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	gc.SetFillGradient(redBlue(20, draw2d.RepeatSpread))
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 50, 50, color.RGBA{255, 0, 0, 255})   // t = 0
	checkPixel(t, dest, 70, 50, color.RGBA{255, 0, 0, 255})   // t = 1 wraps to 0
	checkPixel(t, dest, 90, 50, color.RGBA{255, 0, 0, 255})   // t = 2 wraps to 0
	checkPixel(t, dest, 60, 50, color.RGBA{128, 0, 128, 255}) // t = 0.5
	checkPixel(t, dest, 80, 50, color.RGBA{128, 0, 128, 255}) // t = 1.5 wraps to 0.5
	// the seam at t = 1 is continuous: colors mirror around it
	checkPixel(t, dest, 69, 50, color.RGBA{12, 0, 243, 255}) // t = 0.95
	checkPixel(t, dest, 71, 50, color.RGBA{243, 0, 12, 255}) // t = 1.05 -> 0.05
}

func TestGradientReflectSpread(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	gc.SetFillGradient(redBlue(20, draw2d.ReflectSpread))
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 50, 50, color.RGBA{255, 0, 0, 255})    // t = 0
	checkPixel(t, dest, 70, 50, color.RGBA{0, 0, 255, 255})    // t = 1
	checkPixel(t, dest, 90, 50, color.RGBA{255, 0, 0, 255})    // t = 2 reflects to 0
	checkPixel(t, dest, 80, 50, color.RGBA{128, 0, 128, 255})  // t = 1.5 reflects to 0.5
	checkPixel(t, dest, 100, 50, color.RGBA{128, 0, 128, 255}) // t = 2.5 reflects to 0.5
}

func TestGradientTransformed(t *testing.T) {
	dest, gc := newCanvas(101, 101)
	// gradient defined in user space, then translated and scaled:
	// user center (20, 20) radius 10 lands on device (50.5, 50.5) radius 20
	g := draw2d.NewRadialGradient(20, 20, 10, 20, 20)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	gc.Translate(10.5, 10.5)
	gc.Scale(2, 2)
	fillAll(gc, 101, 101)

	checkPixel(t, dest, 50, 50, color.RGBA{255, 0, 0, 255})   // user (20, 20), t = 0
	checkPixel(t, dest, 70, 50, color.RGBA{0, 0, 255, 255})   // user (30, 20), t = 1
	checkPixel(t, dest, 60, 50, color.RGBA{128, 0, 128, 255}) // user (25, 20), t = 0.5
}

func TestGradientSaveRestore(t *testing.T) {
	dest, gc := newCanvas(90, 30)
	g1 := draw2d.NewRadialGradient(15.5, 15.5, 10, 15.5, 15.5)
	g1.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g1.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	g2 := draw2d.NewRadialGradient(45.5, 15.5, 10, 45.5, 15.5)
	g2.AddColorStop(0, color.RGBA{0, 0, 0, 255})
	g2.AddColorStop(1, color.RGBA{255, 255, 255, 255})

	gc.SetFillGradient(g1)
	gc.Save()
	gc.SetFillColor(color.RGBA{0, 255, 0, 255}) // replaces the gradient
	gc.Save()
	gc.SetFillGradient(g2)
	fillRect(gc, 30, 0, 60, 30) // middle: g2
	gc.Restore()
	fillRect(gc, 60, 0, 90, 30) // right: solid green
	gc.Restore()
	fillRect(gc, 0, 0, 30, 30) // left: g1 again

	checkPixel(t, dest, 15, 15, color.RGBA{255, 0, 0, 255})     // g1 center
	checkPixel(t, dest, 45, 15, color.RGBA{0, 0, 0, 255})       // g2 center
	checkPixel(t, dest, 75, 15, color.RGBA{0, 255, 0, 255})     // solid green
	checkPixel(t, dest, 25, 15, color.RGBA{0, 0, 255, 255})     // g1 edge, t = 1
	checkPixel(t, dest, 55, 15, color.RGBA{255, 255, 255, 255}) // g2 edge, t = 1
}

func TestGradientSetFillColorRestoresSolid(t *testing.T) {
	dest, gc := newCanvas(30, 30)
	gc.SetFillGradient(redBlue(10, draw2d.PadSpread))
	gc.SetFillColor(color.RGBA{0, 255, 0, 255})
	fillAll(gc, 30, 30)
	checkPixel(t, dest, 15, 15, color.RGBA{0, 255, 0, 255})
}

func TestGradientZeroRadius(t *testing.T) {
	dest, gc := newCanvas(30, 30)
	g := draw2d.NewRadialGradient(15, 15, 0, 15, 15)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	fillAll(gc, 30, 30)
	// zero radius: the whole canvas takes the last stop color
	checkPixel(t, dest, 15, 15, color.RGBA{0, 0, 255, 255})
	checkPixel(t, dest, 0, 0, color.RGBA{0, 0, 255, 255})
	checkPixel(t, dest, 29, 29, color.RGBA{0, 0, 255, 255})
}

func TestGradientEmptyStops(t *testing.T) {
	dest, gc := newCanvas(30, 30)
	gc.SetFillGradient(draw2d.NewRadialGradient(15, 15, 10, 15, 15))
	fillAll(gc, 30, 30)
	// empty stops paint nothing: the white background is untouched
	checkPixel(t, dest, 15, 15, color.RGBA{255, 255, 255, 255})
	checkPixel(t, dest, 0, 0, color.RGBA{255, 255, 255, 255})
}

func TestGradientNonFiniteGeometry(t *testing.T) {
	dest, gc := newCanvas(30, 30)
	g := draw2d.NewRadialGradient(15, 15, 10, 15, 15)
	g.Cx = math.Inf(1) // invalid gradient paints nothing
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	gc.SetFillGradient(g)
	fillAll(gc, 30, 30)
	checkPixel(t, dest, 15, 15, color.RGBA{255, 255, 255, 255})
	checkPixel(t, dest, 0, 0, color.RGBA{255, 255, 255, 255})
}

func TestGradientLargeCanvas(t *testing.T) {
	// a canvas well beyond typical chart sizes must produce the same
	// per-pixel results without overflow
	w, h := 3000, 2000
	dest, gc := newCanvas(w, h)
	g := draw2d.NewRadialGradient(1500.5, 1000.5, 500, 1500.5, 1000.5)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	fillAll(gc, w, h)

	checkPixel(t, dest, 1500, 1000, color.RGBA{255, 0, 0, 255})
	checkPixel(t, dest, 2000, 1000, color.RGBA{0, 0, 255, 255})   // t = 1
	checkPixel(t, dest, 1750, 1000, color.RGBA{128, 0, 128, 255}) // t = 0.5
	checkPixel(t, dest, 0, 0, color.RGBA{0, 0, 255, 255})         // far corner, padded
}
