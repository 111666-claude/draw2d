package draw2ddiff

import (
	"image"
	"image/color"
	"math"

	"github.com/llgcode/draw2d"
)

// CanvasWidth and CanvasHeight are the fixed logical canvas size used by
// every scene on every backend, so snapshots are comparable.
const (
	CanvasWidth  = 297
	CanvasHeight = 210
)

// SceneKind classifies a scene. Normal scenes expect CategoryOK;
// edge-case kinds map to their own stable category.
type SceneKind int

const (
	KindNormal SceneKind = iota
	KindEmpty
	KindNonZeroBounds
	KindMissingGlyph
	KindNonFinite
)

func (k SceneKind) category() Category {
	switch k {
	case KindEmpty:
		return CategoryEmptyScene
	case KindNonZeroBounds:
		return CategoryNonZeroBounds
	case KindMissingGlyph:
		return CategoryMissingGlyph
	case KindNonFinite:
		return CategoryNonFinite
	default:
		return CategoryOK
	}
}

// Scene is a fixed drawing program driving all three backends.
type Scene struct {
	Name string
	Kind SceneKind
	// Font, when non-nil, is the pinned font the scene needs. The gate
	// verifies it loads from the bundled font folder before rendering;
	// otherwise the result is classified CategoryFontPlatform and fails.
	Font *draw2d.FontData
	Draw func(gc draw2d.GraphicContext)
}

// gateFont is the single font used by text scenes. It is bundled in
// resource/font (including the gofpdf luximbi.json/z definitions), which
// keeps text rendering reproducible across Linux and macOS.
var gateFont = draw2d.FontData{
	Name:   "luxi",
	Family: draw2d.FontFamilyMono,
	Style:  draw2d.FontStyleBold | draw2d.FontStyleItalic,
}

// Scenes is the fixed, ordered scene registry driving the gate.
var Scenes = []Scene{
	{Name: "paths", Kind: KindNormal, Draw: drawPaths},
	{Name: "overlap", Kind: KindNormal, Draw: drawOverlap},
	{Name: "text", Kind: KindNormal, Font: &gateFont, Draw: drawText},
	{Name: "image", Kind: KindNormal, Draw: drawImage},
	{Name: "transform", Kind: KindNormal, Draw: drawTransform},
	{Name: "alpha", Kind: KindNormal, Draw: drawAlpha},
	{Name: "empty", Kind: KindEmpty, Draw: drawEmpty},
	{Name: "nonzero-bounds", Kind: KindNonZeroBounds, Draw: drawNonZeroBounds},
	{Name: "missing-glyph", Kind: KindMissingGlyph, Font: &gateFont, Draw: drawMissingGlyph},
	{Name: "nonfinite", Kind: KindNonFinite, Draw: drawNonFinite},
}

// FindScene returns the scene with the given name.
func FindScene(name string) *Scene {
	for i := range Scenes {
		if Scenes[i].Name == name {
			return &Scenes[i]
		}
	}
	return nil
}

// rectPath builds a closed rectangle path.
func rectPath(x1, y1, x2, y2 float64) *draw2d.Path {
	p := &draw2d.Path{}
	p.MoveTo(x1, y1)
	p.LineTo(x2, y1)
	p.LineTo(x2, y2)
	p.LineTo(x1, y2)
	p.Close()
	return p
}

// drawPaths exercises every path segment type plus stroke attributes.
func drawPaths(gc draw2d.GraphicContext) {
	gc.SetFillColor(opaque(0x24, 0x6a, 0xc2))
	gc.SetStrokeColor(opaque(0x10, 0x20, 0x30))
	gc.SetLineWidth(3)
	gc.SetLineCap(draw2d.RoundCap)
	gc.SetLineJoin(draw2d.BevelJoin)
	p := &draw2d.Path{}
	p.MoveTo(20, 20)
	p.LineTo(120, 30)
	p.QuadCurveTo(150, 80, 110, 100)
	p.CubicCurveTo(60, 120, 180, 160, 140, 180)
	p.ArcTo(80, 150, 30, 20, 0, math.Pi)
	p.Close()
	gc.FillStroke(p)

	gc.SetLineDash([]float64{6, 3}, 1)
	gc.SetLineWidth(1.5)
	gc.SetStrokeColor(opaque(0xc2, 0x24, 0x6a))
	q := &draw2d.Path{}
	q.MoveTo(200, 40)
	q.LineTo(270, 60)
	q.LineTo(240, 120)
	q.Close()
	gc.Stroke(q)
}

// drawOverlap draws two overlapping translucent rectangles. The blended
// overlap region makes the output sensitive to drawing order, so a path
// order mutation is caught by the gate.
func drawOverlap(gc draw2d.GraphicContext) {
	gc.SetFillColor(color.RGBA{0xe0, 0x30, 0x30, 0xb0})
	gc.Fill(rectPath(40, 40, 140, 120))
	gc.SetFillColor(color.RGBA{0x30, 0x30, 0xe0, 0xb0})
	gc.Fill(rectPath(90, 70, 190, 150))
}

// drawText renders pinned-font text at two sizes.
func drawText(gc draw2d.GraphicContext) {
	gc.SetFontData(gateFont)
	gc.SetFillColor(opaque(0x33, 0x33, 0x33))
	gc.SetFontSize(14)
	gc.FillStringAt("draw2d gate 0123", 20, 60)
	gc.SetFontSize(24)
	gc.FillStringAt("Pack my box", 20, 120)
}

// drawImage draws a synthesized, deterministic gradient image.
func drawImage(gc draw2d.GraphicContext) {
	gc.DrawImage(gradientImage(image.Rect(0, 0, 64, 48)))
}

// drawTransform exercises save/restore and matrix composition.
func drawTransform(gc draw2d.GraphicContext) {
	gc.Save()
	gc.Translate(150, 100)
	gc.Rotate(math.Pi / 6)
	gc.Scale(1.5, 0.75)
	gc.SetFillColor(opaque(0x2a, 0x9d, 0x8f))
	gc.Fill(rectPath(-40, -25, 40, 25))
	gc.Restore()
	gc.SetFillColor(opaque(0xe9, 0xc4, 0x6a))
	gc.Fill(rectPath(10, 10, 30, 30))
}

// drawAlpha draws overlapping semi-transparent fill and stroke.
func drawAlpha(gc draw2d.GraphicContext) {
	gc.SetFillColor(color.RGBA{0xff, 0x00, 0x00, 0x80})
	gc.Fill(rectPath(50, 50, 150, 130))
	gc.SetStrokeColor(color.RGBA{0x00, 0x00, 0xff, 0x80})
	gc.SetLineWidth(8)
	gc.Stroke(rectPath(100, 80, 220, 160))
}

// drawEmpty draws nothing: the empty-scene classification.
func drawEmpty(gc draw2d.GraphicContext) {}

// drawNonZeroBounds draws an image whose bounds do not start at (0,0)
// and a path partially outside the canvas.
func drawNonZeroBounds(gc draw2d.GraphicContext) {
	gc.DrawImage(gradientImage(image.Rect(7, 11, 71, 59)))
	gc.SetStrokeColor(opaque(0x00, 0x80, 0x40))
	gc.SetLineWidth(2)
	p := &draw2d.Path{}
	p.MoveTo(-20, -20)
	p.LineTo(60, 40)
	p.LineTo(10, 90)
	gc.Stroke(p)
}

// drawMissingGlyph renders text containing runes the bundled luxi font
// does not cover (CJK, private use), exercising the missing-glyph
// fallback path of every backend.
func drawMissingGlyph(gc draw2d.GraphicContext) {
	gc.SetFontData(gateFont)
	gc.SetFillColor(opaque(0x33, 0x33, 0x33))
	gc.SetFontSize(16)
	gc.FillStringAt("A中文�Z", 20, 100)
}

// drawNonFinite feeds NaN and Inf coordinates into a path. Backends
// must either reject it deterministically or produce a stable snapshot;
// either outcome is recorded in the golden baseline.
func drawNonFinite(gc draw2d.GraphicContext) {
	gc.SetFillColor(opaque(0x80, 0x80, 0x80))
	p := &draw2d.Path{}
	p.MoveTo(10, 10)
	p.LineTo(math.NaN(), 50)
	p.LineTo(100, math.Inf(1))
	p.CubicCurveTo(0, 0, math.Inf(-1), 0, 50, 50)
	p.Close()
	gc.FillStroke(p)
}

// gradientImage builds a deterministic test image with the given bounds.
func gradientImage(bounds image.Rectangle) *image.RGBA {
	img := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / maxInt(1, bounds.Dx())),
				G: uint8((y * 255) / maxInt(1, bounds.Dy())),
				B: uint8(((x + y) * 255) / maxInt(1, bounds.Dx()+bounds.Dy())),
				A: 0xff,
			})
		}
	}
	return img
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
