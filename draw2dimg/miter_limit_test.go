// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
)

// render strokes the path built by draw on a white canvas and returns it.
func render(width, height int, lineWidth, miterLimit float64, join draw2d.LineJoin, dash []float64, tr *draw2d.Matrix, draw func(gc draw2d.GraphicContext)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	gc := draw2dimg.NewGraphicContext(img)
	gc.Clear() // fills with the default white fill color
	gc.SetStrokeColor(color.Black)
	gc.SetLineWidth(lineWidth)
	gc.SetLineJoin(join)
	gc.SetMiterLimit(miterLimit)
	if dash != nil {
		gc.SetLineDash(dash, 0)
	}
	if tr != nil {
		gc.SetMatrixTransform(*tr)
	}
	draw(gc)
	gc.Stroke()
	return img
}

func isDark(img *image.RGBA, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r < 0x8000 && g < 0x8000 && b < 0x8000
}

// darkBounds returns the bounding box of the dark (stroked) pixels.
func darkBounds(img *image.RGBA) (minX, minY, maxX, maxY int) {
	b := img.Bounds()
	minX, minY, maxX, maxY = b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isDark(img, x, y) {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	return
}

// maxDarkRadius returns the largest distance from (cx, cy) covered by a dark pixel.
func maxDarkRadius(img *image.RGBA, cx, cy float64) float64 {
	max := 0.0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isDark(img, x, y) {
				if d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy); d > max {
					max = d
				}
			}
		}
	}
	return max
}

// acuteSpike is a path with a sharp join at (150, 100): the turn angle is
// ~146.3°, giving a miter ratio of ~3.45. With a half line width of 5 the
// miter tip lands on (166.51, 105); a bevel join stays below x = 153.
func acuteSpike(gc draw2d.GraphicContext) {
	gc.MoveTo(60, 100)
	gc.LineTo(150, 100)
	gc.LineTo(60, 40)
}

func TestMiterLimitAcuteSpikeBounds(t *testing.T) {
	mitered := render(220, 220, 10, 10, draw2d.MiterJoin, nil, nil, acuteSpike)
	_, _, maxX, _ := darkBounds(mitered)
	// miter tip at x = 166.51 (ratio 3.45 < limit 10)
	if maxX < 165 || maxX > 168 {
		t.Errorf("mitered spike maxX = %d, want within [165, 168]", maxX)
	}

	beveled := render(220, 220, 10, 1, draw2d.MiterJoin, nil, nil, acuteSpike)
	_, _, maxX, _ = darkBounds(beveled)
	// bevel fallback: the spike is cut near the half line width
	if maxX < 151 || maxX > 154 {
		t.Errorf("beveled spike maxX = %d, want within [151, 154]", maxX)
	}
}

// closedStar builds a closed 5-point star centered on (cx, cy).
func closedStar(cx, cy, r1, r2 float64) func(gc draw2d.GraphicContext) {
	return func(gc draw2d.GraphicContext) {
		for i := 0; i < 10; i++ {
			r := r1
			if i%2 == 1 {
				r = r2
			}
			a := math.Pi/2 + float64(i)*math.Pi/5
			x, y := cx+r*math.Cos(a), cy-r*math.Sin(a)
			if i == 0 {
				gc.MoveTo(x, y)
			} else {
				gc.LineTo(x, y)
			}
		}
		gc.Close()
	}
}

func TestMiterLimitClosedStar(t *testing.T) {
	const cx, cy, r1, r2 = 120.0, 120.0, 80.0, 32.0
	// star tips turn by 144°: miter ratio 3.24
	mitered := render(240, 240, 8, 10, draw2d.MiterJoin, nil, nil, closedStar(cx, cy, r1, r2))
	got := maxDarkRadius(mitered, cx, cy)
	// tips extend to r1 + halfWidth*ratio = 80 + 4*3.24 = 92.94
	if got < 91 || got > 95 {
		t.Errorf("mitered star radius = %v, want within [91, 95]", got)
	}

	beveled := render(240, 240, 8, 2, draw2d.MiterJoin, nil, nil, closedStar(cx, cy, r1, r2))
	got = maxDarkRadius(beveled, cx, cy)
	// bevel fallback: tips cannot extend further than r1 + halfWidth
	if got > 85 {
		t.Errorf("beveled star radius = %v, want <= 85", got)
	}
}

func TestMiterLimitNegativeScaleAndRotation(t *testing.T) {
	reference := render(220, 220, 10, 10, draw2d.MiterJoin, nil, nil, acuteSpike)

	// horizontal flip: (x, y) -> (220 - x, y)
	flip := draw2d.NewIdentityMatrix()
	flip.Translate(220, 0)
	flip.Scale(-1, 1)
	flipped := render(220, 220, 10, 10, draw2d.MiterJoin, nil, &flip, acuteSpike)
	for y := 0; y < 220; y++ {
		for x := 0; x < 220; x++ {
			if isDark(flipped, 219-x, y) != isDark(reference, x, y) {
				t.Fatalf("negative scale mismatch at (%d, %d)", x, y)
			}
		}
	}

	// 90° rotation about the canvas center
	rot := draw2d.NewIdentityMatrix()
	rot.Translate(110, 110)
	rot.Rotate(math.Pi / 2)
	rot.Translate(-110, -110)
	rotated := render(220, 220, 10, 10, draw2d.MiterJoin, nil, &rot, acuteSpike)
	for y := 0; y < 220; y++ {
		for x := 0; x < 220; x++ {
			tx, ty := rot.TransformPoint(float64(x)+0.5, float64(y)+0.5)
			rx, ry := int(math.Round(tx-0.5)), int(math.Round(ty-0.5))
			if isDark(rotated, rx, ry) != isDark(reference, x, y) {
				t.Fatalf("rotation mismatch at (%d, %d)", x, y)
			}
		}
	}
}

func TestMiterLimitDashedPathStaysBounded(t *testing.T) {
	// the dash pattern strokes the join: the miter limit must still apply
	img := render(220, 220, 10, 10, draw2d.MiterJoin, []float64{25, 15}, nil, acuteSpike)
	minX, minY, maxX, maxY := darkBounds(img)
	if minX < 0 || minY < 0 || maxX >= 220 || maxY >= 220 {
		t.Errorf("dashed miter stroke escapes canvas: (%d, %d)-(%d, %d)", minX, minY, maxX, maxY)
	}
	if maxX > 168 {
		t.Errorf("dashed miter stroke extends to x = %d, beyond the miter limit bound 168", maxX)
	}

	// with a bevel-forcing limit the dashed spike must be cut
	img = render(220, 220, 10, 1, draw2d.MiterJoin, []float64{25, 15}, nil, acuteSpike)
	_, _, maxX, _ = darkBounds(img)
	if maxX > 154 {
		t.Errorf("dashed beveled stroke extends to x = %d, want <= 154", maxX)
	}
}

func TestMiterLimitDoesNotChangeBevelAndRoundJoins(t *testing.T) {
	for _, join := range []draw2d.LineJoin{draw2d.BevelJoin, draw2d.RoundJoin} {
		a := render(220, 220, 10, 1, join, nil, nil, acuteSpike)
		b := render(220, 220, 10, 1000, join, nil, nil, acuteSpike)
		for y := 0; y < 220; y++ {
			for x := 0; x < 220; x++ {
				if a.At(x, y) != b.At(x, y) {
					t.Fatalf("join %v: pixel (%d, %d) changed with miter limit", join, x, y)
				}
			}
		}
	}
}

func TestMiterLimitDegenerateValuesStayBounded(t *testing.T) {
	for _, limit := range []float64{0, -3, math.NaN(), math.Inf(1), math.Inf(-1), 1e300} {
		img := render(220, 220, 10, limit, draw2d.MiterJoin, nil, nil, acuteSpike)
		minX, minY, maxX, maxY := darkBounds(img)
		if minX < 0 || minY < 0 || maxX >= 220 || maxY >= 220 {
			t.Errorf("limit %v: stroke escapes canvas: (%d, %d)-(%d, %d)", limit, minX, minY, maxX, maxY)
		}
	}
}
