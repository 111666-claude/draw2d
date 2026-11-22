// Command dashverify draws the same dashed scenes through the pdf, image
// and svg backends so their rendered output can be compared. The scenes
// cover straight lines (offset and odd-length patterns), bezier curves,
// multiple subpaths, save/restore and matrix scaling.
package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/jung-kurt/gofpdf"
	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dpdf"
	"github.com/llgcode/draw2d/draw2dsvg"
)

const (
	width  = 600.0
	height = 480.0
)

// scenes returns one drawing function per scene. Every scene is fully
// deterministic and uses the same coordinates on every backend.
func scenes() map[string]func(gc draw2d.GraphicContext) {
	return map[string]func(gc draw2d.GraphicContext){
		// horizontal lines with varying patterns and offsets
		"line": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(4)
			gc.SetLineCap(draw2d.ButtCap)
			patterns := []struct {
				y      float64
				dash   []float64
				offset float64
			}{
				{40, []float64{20, 10}, 0},
				{80, []float64{20, 10}, 7},
				{120, []float64{15, 8, 5}, 0},   // odd length
				{160, []float64{15, 8, 5}, -11}, // odd length, negative offset
				{200, []float64{30, 6}, 300},    // offset larger than the pattern
			}
			for _, p := range patterns {
				gc.SetLineDash(p.dash, p.offset)
				gc.MoveTo(30, p.y)
				gc.LineTo(570, p.y)
				gc.Stroke()
			}
			// solid reference line at the bottom
			gc.SetLineDash(nil, 0)
			gc.MoveTo(30, 250)
			gc.LineTo(570, 250)
			gc.Stroke()
		},
		// cubic bezier curve
		"curve": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(4)
			gc.SetLineCap(draw2d.ButtCap)
			gc.SetLineDash([]float64{18, 9}, 5)
			gc.MoveTo(40, 360)
			gc.CubicCurveTo(160, 40, 440, 440, 560, 120)
			gc.Stroke()
		},
		// several independent subpaths in one path
		"multipath": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(4)
			gc.SetLineCap(draw2d.ButtCap)
			gc.SetLineDash([]float64{16, 8}, 3)
			// subpath 1: line
			gc.MoveTo(40, 60)
			gc.LineTo(260, 60)
			// subpath 2: line
			gc.MoveTo(330, 120)
			gc.LineTo(560, 120)
			// subpath 3: cubic bezier
			gc.MoveTo(60, 300)
			gc.CubicCurveTo(200, 180, 380, 420, 540, 300)
			// subpath 4: closed triangle
			gc.MoveTo(120, 420)
			gc.LineTo(240, 360)
			gc.LineTo(240, 460)
			gc.Close()
			gc.Stroke()
		},
		// save/restore must bring back the outer dash pattern
		"restore": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(4)
			gc.SetLineCap(draw2d.ButtCap)
			gc.SetLineDash([]float64{24, 8}, 0)
			gc.MoveTo(30, 50)
			gc.LineTo(570, 50)
			gc.Stroke()
			gc.Save()
			gc.SetLineDash([]float64{4, 6}, 0)
			gc.MoveTo(30, 110)
			gc.LineTo(570, 110)
			gc.Stroke()
			gc.Restore()
			// outer pattern again
			gc.MoveTo(30, 170)
			gc.LineTo(570, 170)
			gc.Stroke()
		},
		// dash lengths are user units, the pdf CTM must scale them too
		"scale": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(2)
			gc.SetLineCap(draw2d.ButtCap)
			gc.Save()
			gc.Translate(40, 200)
			gc.Scale(2, 2)
			gc.SetLineDash([]float64{15, 7}, 0)
			gc.MoveTo(0, 0)
			gc.LineTo(250, 0)
			gc.Stroke()
			gc.Restore()
			// unscaled reference with the same pattern
			gc.SetLineDash([]float64{15, 7}, 0)
			gc.MoveTo(40, 260)
			gc.LineTo(540, 260)
			gc.Stroke()
		},
		// line caps and joins must be honored on dashed strokes
		"capjoin": func(gc draw2d.GraphicContext) {
			gc.SetStrokeColor(image.Black)
			gc.SetLineWidth(8)
			gc.SetLineDash([]float64{22, 12}, 0)
			// round caps
			gc.SetLineCap(draw2d.RoundCap)
			gc.MoveTo(30, 50)
			gc.LineTo(570, 50)
			gc.Stroke()
			// square caps
			gc.SetLineCap(draw2d.SquareCap)
			gc.MoveTo(30, 110)
			gc.LineTo(570, 110)
			gc.Stroke()
			// miter joins on a dashed zig-zag
			gc.SetLineCap(draw2d.ButtCap)
			gc.SetLineJoin(draw2d.MiterJoin)
			gc.MoveTo(30, 200)
			gc.LineTo(120, 260)
			gc.LineTo(210, 200)
			gc.LineTo(300, 260)
			gc.LineTo(390, 200)
			gc.LineTo(480, 260)
			gc.LineTo(570, 200)
			gc.Stroke()
			// round joins on the same shape
			gc.SetLineJoin(draw2d.RoundJoin)
			gc.MoveTo(30, 330)
			gc.LineTo(120, 390)
			gc.LineTo(210, 330)
			gc.LineTo(300, 390)
			gc.LineTo(390, 330)
			gc.LineTo(480, 390)
			gc.LineTo(570, 330)
			gc.Stroke()
		},
	}
}

func writePng(outDir string, name string, draw func(gc draw2d.GraphicContext)) error {
	dest := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	gc := draw2dimg.NewGraphicContext(dest)
	draw(gc)
	return draw2dimg.SaveToPngFile(filepath.Join(outDir, name+".png"), dest)
}

func writeSvg(outDir string, name string, draw func(gc draw2d.GraphicContext)) error {
	svg := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(svg)
	draw(gc)
	return draw2dsvg.SaveToSvgFile(filepath.Join(outDir, name+".svg"), svg)
}

func writePdf(outDir string, name string, draw func(gc draw2d.GraphicContext)) error {
	pdf := newCustomPdf(width, height)
	gc := draw2dpdf.NewGraphicContext(pdf)
	draw(gc)
	return draw2dpdf.SaveToPdfFile(filepath.Join(outDir, name+".pdf"), pdf)
}

// newCustomPdf creates a single-page pdf document of the given pixel-like
// point size, matching the defaults of draw2dpdf.NewPdf.
func newCustomPdf(w, h float64) *gofpdf.Fpdf {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "pt",
		Size:    gofpdf.SizeType{Wd: w, Ht: h},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetLineCapStyle("butt")
	pdf.SetLineJoinStyle("round")
	pdf.SetLineWidth(1)
	pdf.AddPage()
	return pdf
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dashverify <output-dir>")
		os.Exit(2)
	}
	outDir := os.Args[1]
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, draw := range scenes() {
		for _, w := range []struct {
			fn  func(string, string, func(gc draw2d.GraphicContext)) error
			ext string
		}{
			{writePng, "png"}, {writeSvg, "svg"}, {writePdf, "pdf"},
		} {
			if err := w.fn(outDir, name, draw); err != nil {
				fmt.Fprintf(os.Stderr, "%s.%s: %v\n", name, w.ext, err)
				os.Exit(1)
			}
		}
	}
	fmt.Println("generated scenes in", outDir)
}
