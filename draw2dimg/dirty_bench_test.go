// Copyright 2010 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d/draw2dkit"
)

// drawMovingRect is the single primitive both modes execute per iteration, so
// the benchmark isolates the cost of the rendering mode rather than the number
// of submissions.
func drawMovingRect(gc *GraphicContext, x, y float64) {
	gc.SetFillColor(color.RGBA{0x22, 0x88, 0xcc, 0xff})
	gc.SetStrokeColor(color.RGBA{0x10, 0x10, 0x10, 0xff})
	gc.SetLineWidth(2)
	draw2dkit.Rectangle(gc, x, y, x+40, y+24)
	gc.FillStroke()
}

// BenchmarkFullCanvasRepaint repaints a whole large canvas for every small
// update, the historical behaviour an interactive canvas suffered from.
func BenchmarkFullCanvasRepaint(b *testing.B) {
	const w, h = 1024, 768
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	gc := NewGraphicContext(img)
	gc.EnableStats(true)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		drawMovingRect(gc, float64(20+(i%50)*4), 300)
	}
	b.StopTimer()
	reportWork(b, gc)
}

// BenchmarkIncrementalRepaint rasterizes only the affected region per update.
func BenchmarkIncrementalRepaint(b *testing.B) {
	const w, h = 1024, 768
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	gc := NewGraphicContext(img)
	gc.SetIncremental(true)
	gc.EnableStats(true)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		drawMovingRect(gc, float64(20+(i%50)*4), 300)
		// A presented frame consumes its dirty regions.
		gc.ResetDirty()
	}
	b.StopTimer()
	reportWork(b, gc)
}

// BenchmarkIncrementalComplexScene models a dense canvas (hundreds of static
// primitives) where one small curve is edited. The full mode re-rasterizes the
// whole canvas; the incremental mode only touches the edited curve's region,
// which is where dirty-rectangle rendering actually pays off.
func BenchmarkIncrementalComplexScene(b *testing.B) {
	const w, h = 1600, 1200
	build := func(gc *GraphicContext) {
		gc.SetStrokeColor(color.RGBA{0x33, 0x33, 0x33, 0xff})
		gc.SetLineWidth(1)
		for i := 0; i < 300; i++ {
			x := float64((i * 37) % w)
			y := float64((i * 53) % h)
			draw2dkit.Circle(gc, x, y, 6)
			gc.Stroke()
		}
	}

	b.Run("full", func(b *testing.B) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		gc := NewGraphicContext(img)
		gc.EnableStats(true)
		build(gc)
		gc.ResetStats()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Editing one curve still rebuilds the whole scene in full mode.
			build(gc)
			gc.SetStrokeColor(color.RGBA{0xcc, 0x22, 0x22, 0xff})
			gc.SetLineWidth(3)
			gc.MoveTo(100, 100)
			gc.CubicCurveTo(120, 60, 180, 160, 220, 110)
			gc.Stroke()
		}
		b.StopTimer()
		reportWork(b, gc)
	})

	b.Run("incremental", func(b *testing.B) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		gc.EnableStats(true)
		build(gc)
		gc.ResetDirty()
		gc.ResetStats()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Only the edited curve is re-rasterized.
			gc.SetStrokeColor(color.RGBA{0xcc, 0x22, 0x22, 0xff})
			gc.SetLineWidth(3)
			gc.MoveTo(100, 100)
			gc.CubicCurveTo(120, 60, 180, 160, 220, 110)
			gc.Stroke()
			gc.ResetDirty()
		}
		b.StopTimer()
		reportWork(b, gc)
	})
}

func reportWork(b *testing.B, gc *GraphicContext) {
	s := gc.Stats()
	b.ReportMetric(float64(s.RasterizedPixels)/float64(b.N), "rasterized-px/op")
	b.ReportMetric(float64(s.PaintedPixels)/float64(b.N), "painted-px/op")
}
