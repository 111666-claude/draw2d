// Copyright 2010 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dkit"
)

// drawOp is one reproducible drawing submission applied to a graphic context.
// Every op must produce identical pixels in full and incremental mode.
type drawOp struct {
	name string
	fn   func(gc *GraphicContext)
}

func standardOps() []drawOp {
	ops := []drawOp{
		{
			name: "rect-fill",
			fn: func(gc *GraphicContext) {
				gc.SetFillColor(color.RGBA{0x33, 0x66, 0x99, 0xff})
				draw2dkit.Rectangle(gc, 20.5, 30.25, 120.75, 90.5)
				gc.Fill()
			},
		},
		{
			name: "rect-stroke",
			fn: func(gc *GraphicContext) {
				gc.SetStrokeColor(color.RGBA{0x10, 0x10, 0x10, 0xff})
				gc.SetLineWidth(3.5)
				draw2dkit.Rectangle(gc, 40, 50, 150, 110)
				gc.Stroke()
			},
		},
		{
			name: "circle-fillstroke",
			fn: func(gc *GraphicContext) {
				gc.SetFillColor(color.RGBA{0xaa, 0x22, 0x22, 0xee})
				gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
				gc.SetLineWidth(2.25)
				draw2dkit.Circle(gc, 140.5, 140.5, 37.75)
				gc.FillStroke()
			},
		},
		{
			name: "rotated-curve",
			fn: func(gc *GraphicContext) {
				gc.Save()
				gc.Rotate(0.37)
				gc.Translate(60, 40)
				gc.SetStrokeColor(color.RGBA{0x22, 0x77, 0x22, 0xff})
				gc.SetLineWidth(4.0)
				gc.MoveTo(0, 0)
				gc.CubicCurveTo(30, -40, 90, 60, 130, 10)
				gc.QuadCurveTo(160, -20, 180, 40)
				gc.Stroke()
				gc.Restore()
			},
		},
		{
			name: "arc-fill",
			fn: func(gc *GraphicContext) {
				gc.SetFillColor(color.RGBA{0x55, 0x55, 0xdd, 0xcc})
				gc.MoveTo(200, 160)
				gc.ArcTo(200, 160, 45, 30, 0.2, 3.4)
				gc.Close()
				gc.Fill()
			},
		},
		{
			name: "dash-stroke",
			fn: func(gc *GraphicContext) {
				gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
				gc.SetLineWidth(2)
				gc.SetLineDash([]float64{8, 4, 2, 4}, 1.5)
				gc.MoveTo(10, 190)
				gc.LineTo(280, 175)
				gc.Stroke()
				gc.SetLineDash(nil, 0)
			},
		},
		{
			name: "thin-hairline",
			fn: func(gc *GraphicContext) {
				gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
				gc.SetLineWidth(0)
				gc.MoveTo(5.5, 5.5)
				gc.LineTo(290.5, 8.5)
				gc.Stroke()
			},
		},
		{
			name: "scaled-shape",
			fn: func(gc *GraphicContext) {
				gc.Save()
				gc.Translate(210, 30)
				gc.Scale(1.4, 0.8)
				gc.SetFillColor(color.RGBA{0x99, 0x33, 0x99, 0x99})
				draw2dkit.Rectangle(gc, 0, 0, 40, 25)
				gc.Fill()
				gc.Restore()
			},
		},
		{
			name: "negative-coords",
			fn: func(gc *GraphicContext) {
				gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
				gc.SetLineWidth(5)
				gc.MoveTo(-20, -10)
				gc.LineTo(60, 40)
				gc.Stroke()
			},
		},
		{
			name: "off-canvas-shape",
			fn: func(gc *GraphicContext) {
				gc.SetFillColor(color.RGBA{0x12, 0x34, 0x56, 0xff})
				draw2dkit.Rectangle(gc, 400, 400, 500, 500)
				gc.Fill()
			},
		},
	}
	return ops
}

// render runs every op onto a fresh canvas. When incremental is true each op
// runs with dirty tracking enabled; dirty regions are reset between ops the
// way an interactive canvas would after presenting a frame.
func render(t *testing.T, incremental bool, ops []drawOp, size image.Rectangle) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(size)
	gc := NewGraphicContext(img)
	gc.SetIncremental(incremental)
	for _, op := range ops {
		op.fn(gc)
		if incremental {
			// Consume and validate the reported regions, then start a fresh
			// frame like a canvas presenting the update would.
			regions := gc.DirtyRegions()
			for _, r := range regions {
				if !r.In(size) {
					t.Fatalf("op %s: dirty region %v outside canvas %v", op.name, r, size)
				}
			}
			gc.ResetDirty()
		}
	}
	return img
}

// diffPixels returns the count and first differing coordinates between two
// equally sized images.
func diffPixels(a, b *image.RGBA) (int, [][2]int) {
	n := 0
	var first [][2]int
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ca := a.RGBAAt(x, y)
			cb := b.RGBAAt(x, y)
			if ca != cb {
				n++
				if len(first) < 10 {
					first = append(first, [2]int{x, y})
				}
			}
		}
	}
	return n, first
}

// TestIncrementalRawPixParity compares the underlying RGBA byte slices rather
// than decoded colors, which is the strictest reading of "pixel identical":
// every byte of the produced image must match between full and incremental
// rendering for a representative mixed workload.
func TestIncrementalRawPixParity(t *testing.T) {
	size := image.Rect(0, 0, 320, 240)
	scene := func(gc *GraphicContext) {
		gc.SetFillColor(color.RGBA{0xef, 0xef, 0xef, 0xff})
		draw2dkit.Rectangle(gc, 0, 0, 320, 240)
		gc.Fill()
		for _, op := range standardOps() {
			op.fn(gc)
		}
		gc.SetFillColor(color.RGBA{0, 0, 0, 255})
		gc.SetFontSize(16)
		gc.FillStringAt("Parity 0123456789", 12, 200)
	}
	full := render(t, false, []drawOp{{name: "scene", fn: scene}}, size)
	inc := render(t, true, []drawOp{{name: "scene", fn: scene}}, size)
	if len(full.Pix) != len(inc.Pix) {
		t.Fatalf("pix length differs: %d vs %d", len(full.Pix), len(inc.Pix))
	}
	for i, b := range full.Pix {
		if inc.Pix[i] != b {
			t.Fatalf("byte %d differs: full=%d inc=%d", i, b, inc.Pix[i])
		}
	}
}

// TestIncrementalPixelParity is the central correctness test: every drawing
// op, alone and combined, must produce byte identical pixels with and without
// incremental rendering, including across successive frames whose dirty
// regions are reset between submissions.
func TestIncrementalPixelParity(t *testing.T) {
	size := image.Rect(0, 0, 300, 210)
	ops := standardOps()

	// Each op individually, on an otherwise empty canvas.
	for _, op := range ops {
		full := render(t, false, []drawOp{op}, size)
		inc := render(t, true, []drawOp{op}, size)
		if n, at := diffPixels(full, inc); n != 0 {
			t.Fatalf("op %s: %d pixel differences, first at %v", op.name, n, at)
		}
	}

	// All ops composed onto one canvas, the historical sample-test workload.
	full := render(t, false, ops, size)
	inc := render(t, true, ops, size)
	if n, at := diffPixels(full, inc); n != 0 {
		t.Fatalf("combined scene: %d pixel differences, first at %v", n, at)
	}
}

// TestIncrementalFrameParity models the interactive use case: a persistent
// background with a small rectangle moving every frame. The incremental
// sequence (invalidate old + draw new) must equal a full redraw each frame.
func TestIncrementalFrameParity(t *testing.T) {
	size := image.Rect(0, 0, 320, 240)
	background := func(gc *GraphicContext) {
		gc.SetFillColor(color.RGBA{0xee, 0xee, 0xee, 0xff})
		draw2dkit.Rectangle(gc, 0, 0, 320, 240)
		gc.Fill()
	}
	movingRect := func(gc *GraphicContext, x, y float64) {
		gc.SetFillColor(color.RGBA{0x22, 0x88, 0xcc, 0xff})
		gc.SetStrokeColor(color.RGBA{0x10, 0x10, 0x10, 0xff})
		gc.SetLineWidth(2)
		draw2dkit.Rectangle(gc, x, y, x+28, y+18)
		gc.FillStroke()
	}

	// Reference: full redraw of every frame.
	refFrames := make([]*image.RGBA, 0, 20)
	for frame := 0; frame < 20; frame++ {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		background(gc)
		movingRect(gc, 10+float64(frame)*6, 100+math.Sin(float64(frame)*0.7)*30)
		refFrames = append(refFrames, img)
	}

	// Incremental: one canvas updated in place.
	img := image.NewRGBA(size)
	gc := NewGraphicContext(img)
	gc.SetIncremental(true)
	background(gc)
	gc.ResetDirty()
	prevRect := image.Rectangle{}
	for frame := 0; frame < 20; frame++ {
		x := 10 + float64(frame)*6
		y := 100 + math.Sin(float64(frame)*0.7)*30
		cur := image.Rect(int(x)-3, int(y)-3, int(x+28)+4, int(y+18)+4)
		// The canvas owner repaints the union of the previous and the new
		// position so vacated pixels are restored from the background.
		gc.Invalidate(prevRect)
		gc.Invalidate(cur)
		// Repaint the background within the dirty union before the object.
		// ClearRect reads the current fill color, so restore the object's
		// fill color afterwards to match the reference frame.
		for _, r := range gc.DirtyRegions() {
			gc.SetFillColor(color.RGBA{0xee, 0xee, 0xee, 0xff})
			gc.ClearRect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
		}
		movingRect(gc, x, y)
		if n, at := diffPixels(refFrames[frame], img); n != 0 {
			t.Fatalf("frame %d: %d pixel differences, first at %v", frame, n, at)
		}
		prevRect = cur
		gc.ResetDirty()
	}
}

// TestIncrementalClearAndSaveRestoreParity covers Clear, ClearRect and the
// Save/Restore stack: all must stay pixel identical across modes, and Clear
// must mark the whole canvas dirty while ClearRect marks only its box.
func TestIncrementalClearAndSaveRestoreParity(t *testing.T) {
	size := image.Rect(0, 0, 180, 140)
	scene := func(gc *GraphicContext) {
		gc.SetFillColor(color.RGBA{0x11, 0x22, 0x33, 0xff})
		draw2dkit.Rectangle(gc, 10, 10, 90, 70)
		gc.Fill()
		gc.Save()
		gc.Translate(20, 20)
		gc.SetFillColor(color.RGBA{0x44, 0x55, 0x66, 0xff})
		draw2dkit.Circle(gc, 30, 30, 15)
		gc.Fill()
		gc.Restore()
		gc.SetFillColor(color.RGBA{0x77, 0x88, 0x99, 0xff})
		gc.ClearRect(100, 80, 160, 120)
	}
	full := render(t, false, []drawOp{{name: "clear-save", fn: scene}}, size)
	inc := render(t, true, []drawOp{{name: "clear-save", fn: scene}}, size)
	if n, at := diffPixels(full, inc); n != 0 {
		t.Fatalf("clear/save/restore: %d differences, first at %v", n, at)
	}

	// Clear must report the whole canvas, ClearRect only the clipped box.
	gc := NewGraphicContext(image.NewRGBA(size))
	gc.SetIncremental(true)
	gc.ClearRect(10, 10, 40, 40)
	if got := gc.DirtyRegions(); len(got) != 1 || got[0] != image.Rect(10, 10, 40, 40) {
		t.Fatalf("ClearRect region = %v", got)
	}
	gc.ResetDirty()
	gc.Clear()
	if got := gc.DirtyRegions(); len(got) != 1 || got[0] != size {
		t.Fatalf("Clear region = %v, want %v", got, size)
	}
}

// TestDirtyRegionMerge verifies that intersecting and adjacent submissions
// merge into a single non-overlapping region while disjoint ones stay apart.
func TestDirtyRegionMerge(t *testing.T) {
	size := image.Rect(0, 0, 200, 200)
	img := image.NewRGBA(size)
	gc := NewGraphicContext(img)
	gc.SetIncremental(true)

	gc.Invalidate(image.Rect(10, 10, 40, 40))
	gc.Invalidate(image.Rect(30, 20, 60, 50)) // overlaps -> merge
	if got := gc.DirtyRegions(); len(got) != 1 {
		t.Fatalf("overlapping regions: got %v, want 1 merged", got)
	}
	gc.Invalidate(image.Rect(41, 40, 80, 80)) // edge adjacent -> merge
	if got := gc.DirtyRegions(); len(got) != 1 {
		t.Fatalf("adjacent regions: got %v, want 1 merged", got)
	}
	gc.Invalidate(image.Rect(100, 100, 130, 130)) // disjoint
	if got := gc.DirtyRegions(); len(got) != 2 {
		t.Fatalf("disjoint regions: got %v, want 2", got)
	}
	// Regions must never overlap and must stay inside the canvas.
	regions := gc.DirtyRegions()
	for i, a := range regions {
		if !a.In(size) {
			t.Fatalf("region %v outside canvas", a)
		}
		for j, b := range regions {
			if i != j && a.Overlaps(b) {
				t.Fatalf("regions overlap: %v %v", a, b)
			}
		}
	}
	gc.ResetDirty()
	if got := gc.DirtyRegions(); got != nil {
		t.Fatalf("after reset: got %v, want nil", got)
	}
}

// TestDirtyEdgeCases exercises the required degenerate inputs. None of them
// may panic, miss pixels or write outside the canvas.
func TestDirtyEdgeCases(t *testing.T) {
	size := image.Rect(0, 0, 120, 120)

	t.Run("empty-region-no-op", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		gc.Invalidate(image.Rect(50, 50, 50, 60)) // canonical empty
		if got := gc.DirtyRegions(); got != nil {
			t.Fatalf("empty invalidate produced %v", got)
		}
		// Fully off-canvas shapes record nothing.
		gc.SetFillColor(color.RGBA{1, 2, 3, 255})
		draw2dkit.Rectangle(gc, 300, 300, 400, 400)
		gc.Fill()
		if got := gc.DirtyRegions(); got != nil {
			t.Fatalf("off-canvas fill produced %v", got)
		}
	})

	t.Run("canonical-empty-is-noop", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		// A zero-area rectangle (equal edges) is canonical empty and must be
		// ignored.
		gc.Invalidate(image.Rect(50, 50, 50, 60))
		gc.Invalidate(image.Rect(20, 70, 80, 70))
		if got := gc.DirtyRegions(); got != nil {
			t.Fatalf("empty rectangle: got %v, want nil", got)
		}
	})

	t.Run("rect-canonicalization", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		// image.Rect swaps reversed edges rather than producing an empty or
		// inverted box; the canonical rectangle is recorded normally.
		gc.Invalidate(image.Rect(80, 80, 20, 20))
		got := gc.DirtyRegions()
		want := image.Rect(20, 20, 80, 80)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("canonicalized rect: got %v, want %v", got, want)
		}
	})

	t.Run("huge-rect-full-fallback", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		// Coordinates far outside the representable pixel range take the
		// conservative full-canvas branch.
		gc.Invalidate(image.Rect(-2e9, -2e9, 2e9, 2e9))
		got := gc.DirtyRegions()
		if len(got) != 1 || got[0] != size {
			t.Fatalf("huge rectangle: got %v, want full canvas %v", got, size)
		}
	})

	t.Run("non-finite-path-full-fallback", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		gc.SetStrokeColor(color.RGBA{0, 0, 0, 255})
		gc.SetLineWidth(2)
		gc.MoveTo(10, 10)
		gc.LineTo(math.NaN(), 40)
		gc.Stroke()
		got := gc.DirtyRegions()
		if len(got) != 1 || got[0] != size {
			t.Fatalf("NaN path: got %v, want full canvas", got)
		}
	})

	t.Run("huge-coords-full-fallback", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		gc.SetFillColor(color.RGBA{9, 9, 9, 255})
		draw2dkit.Rectangle(gc, -1e12, -1e12, 1e12, 1e12)
		gc.Fill()
		got := gc.DirtyRegions()
		if len(got) != 1 || got[0] != size {
			t.Fatalf("huge rect: got %v, want full canvas", got)
		}
	})

	t.Run("clipped-negative-rect", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		// A rect straddling the origin is valid and must be clipped, not
		// treated as inverted.
		gc.Invalidate(image.Rect(-10, -10, 30, 30))
		got := gc.DirtyRegions()
		want := image.Rect(0, 0, 30, 30)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("clipped rect: got %v, want %v", got, want)
		}
	})

	t.Run("large-dirty-area-fallback", func(t *testing.T) {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.SetIncremental(true)
		gc.EnableStats(true)
		gc.SetFillColor(color.RGBA{1, 2, 3, 255})
		draw2dkit.Rectangle(gc, 0, 0, 120, 120)
		gc.Fill()
		if gc.Stats().PartialRepaints != 0 || gc.Stats().FullRepaints == 0 {
			t.Fatalf("large region should use full fallback: %+v", gc.Stats())
		}
	})
}

// TestIncrementalWorkReduction proves the performance claim: a small
// submission rasterizes and paints far fewer pixels than a full-canvas one.
func TestIncrementalWorkReduction(t *testing.T) {
	size := image.Rect(0, 0, 400, 400)

	run := func(incremental bool) Stats {
		img := image.NewRGBA(size)
		gc := NewGraphicContext(img)
		gc.EnableStats(true)
		gc.SetIncremental(incremental)
		gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
		gc.SetLineWidth(2)
		gc.MoveTo(100, 100)
		gc.LineTo(140, 130)
		gc.Stroke()
		return gc.Stats()
	}

	full := run(false)
	partial := run(true)
	if full.FullRepaints != 1 || partial.PartialRepaints != 1 {
		t.Fatalf("repaint accounting: full=%+v partial=%+v", full, partial)
	}
	// The win is the rasterized area: the sub-rasterizer scans only the
	// affected region instead of every scanline of the canvas.
	if partial.RasterizedPixels >= full.RasterizedPixels/4 {
		t.Fatalf("partial rasterized %d px vs full %d px: expected large reduction",
			partial.RasterizedPixels, full.RasterizedPixels)
	}
	// The painted output must cover exactly the same pixels in both modes:
	// incremental rendering changes how much is scanned, not what is painted.
	if partial.PaintedPixels != full.PaintedPixels {
		t.Fatalf("painted pixel count diverged: partial=%d full=%d",
			partial.PaintedPixels, full.PaintedPixels)
	}
}

// TestIncrementalReentrancy drives the glyph cache path, which re-enters the
// graphic context through Save/Fill/Restore while the outer text submission
// holds the lock. The output must match the non-incremental renderer.
func TestIncrementalReentrancy(t *testing.T) {
	size := image.Rect(0, 0, 260, 120)
	ops := []drawOp{{
		name: "text",
		fn: func(gc *GraphicContext) {
			gc.SetFontSize(18)
			gc.SetFillColor(color.RGBA{0, 0, 0, 255})
			gc.FillStringAt("Reentrant glyph cache 0123", 8, 40)
			gc.SetStrokeColor(color.RGBA{0, 0, 0, 255})
			gc.StrokeStringAt("Stroked text", 8, 80)
		},
	}}
	full := render(t, false, ops, size)
	inc := render(t, true, ops, size)
	if n, at := diffPixels(full, inc); n != 0 {
		t.Fatalf("reentrant text: %d pixel differences, first at %v", n, at)
	}
}

// TestIncrementalConcurrentSubmissions stresses the submission lock: many
// goroutines build their geometry on private contexts and concurrently submit
// their finished paths through one shared graphic context. The submissions
// must be race free and no writes may be lost. Path construction mutates a
// context's current path and is therefore done privately; Fill/Stroke on a
// pre-built path is the documented thread-safe submission boundary.
func TestIncrementalConcurrentSubmissions(t *testing.T) {
	size := image.Rect(0, 0, 256, 256)
	shared := image.NewRGBA(size)
	gc := NewGraphicContext(shared)
	gc.SetIncremental(true)
	gc.SetFillColor(color.RGBA{0x22, 0x44, 0x88, 0xff})

	const goroutines, perG = 16, 25
	type job struct {
		path draw2d.Path
		band int
	}
	jobs := make(chan job, goroutines*perG)

	// Producers build paths on their own graphic contexts, which never share
	// state with other goroutines.
	var prodWg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		prodWg.Add(1)
		go func(g int) {
			defer prodWg.Done()
			local := NewGraphicContext(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			for k := 0; k < perG; k++ {
				y := g * 16
				x := (k * 7) % 200
				draw2dkit.Rectangle(local, float64(x), float64(y), float64(x+8), float64(y+8))
				jobs <- job{path: local.GetPath(), band: g}
				local.BeginPath()
			}
		}(g)
	}
	go func() {
		prodWg.Wait()
		close(jobs)
	}()

	// Consumers all submit through the shared context, exercising the lock
	// under contention.
	var consWg sync.WaitGroup
	consumed := make(chan int, goroutines)
	for c := 0; c < 4; c++ {
		consWg.Add(1)
		go func() {
			defer consWg.Done()
			n := 0
			for j := range jobs {
				p := j.path
				gc.Fill(&p)
				n++
			}
			consumed <- n
		}()
	}
	consWg.Wait()
	close(consumed)
	total := 0
	for n := range consumed {
		total += n
	}
	if total != goroutines*perG {
		t.Fatalf("consumed %d submissions, want %d", total, goroutines*perG)
	}

	// Every band must contain painted pixels: no submission was dropped or
	// corrupted by interleaving.
	for g := 0; g < goroutines; g++ {
		band := shared.SubImage(image.Rect(0, g*16, 256, g*16+16)).(*image.RGBA)
		colored := 0
		for i, a := range band.Pix {
			if i%4 == 3 && a != 0 {
				colored++
			}
		}
		if colored == 0 {
			t.Fatalf("band %d: no pixels painted", g)
		}
	}
}

// TestIncrementalModeSwitch ensures toggling the mode never changes pixels:
// the same sequence split across both modes equals the historical output.
func TestIncrementalModeSwitch(t *testing.T) {
	size := image.Rect(0, 0, 200, 200)
	seq := func(gc *GraphicContext) {
		gc.SetFillColor(color.RGBA{0x40, 0x80, 0xc0, 0xff})
		draw2dkit.Circle(gc, 60, 60, 40)
		gc.Fill()
		gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
		gc.SetLineWidth(3)
		draw2dkit.Circle(gc, 140, 140, 30)
		gc.Stroke()
	}

	ref := image.NewRGBA(size)
	refGC := NewGraphicContext(ref)
	seq(refGC)

	mix := image.NewRGBA(size)
	mixGC := NewGraphicContext(mix)
	mixGC.SetIncremental(true)
	mixGC.SetFillColor(color.RGBA{0x40, 0x80, 0xc0, 0xff})
	draw2dkit.Circle(mixGC, 60, 60, 40)
	mixGC.Fill()
	mixGC.SetIncremental(false)
	mixGC.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
	mixGC.SetLineWidth(3)
	draw2dkit.Circle(mixGC, 140, 140, 30)
	mixGC.Stroke()
	if n, at := diffPixels(ref, mix); n != 0 {
		t.Fatalf("mode switch: %d differences, first at %v", n, at)
	}
}

// TestIncrementalImageParity covers image submissions, including translated,
// scaled and rotated images: the recorded device region and the transformed
// draw must match a full-canvas render byte for byte.
func TestIncrementalImageParity(t *testing.T) {
	size := image.Rect(0, 0, 240, 200)
	src := image.NewRGBA(image.Rect(0, 0, 24, 18))
	for y := 0; y < 18; y++ {
		for x := 0; x < 24; x++ {
			src.Set(x, y, color.RGBA{uint8(x * 10), uint8(y * 13), 0x77, 0xff})
		}
	}
	transforms := []struct {
		name string
		tr   draw2d.Matrix
	}{
		{"translate", draw2d.NewTranslationMatrix(40, 50)},
		{"scale", mul(draw2d.NewTranslationMatrix(60, 70), draw2d.NewScaleMatrix(2.5, 1.75))},
		{"rotate", mul(draw2d.NewTranslationMatrix(120, 100), draw2d.NewRotationMatrix(0.6))},
		{"negative-origin", draw2d.NewTranslationMatrix(-10, -8)},
	}
	for _, tf := range transforms {
		draw := func(gc *GraphicContext) {
			gc.SetMatrixTransform(tf.tr)
			gc.DrawImage(src)
		}
		full := render(t, false, []drawOp{{name: tf.name, fn: draw}}, size)
		inc := render(t, true, []drawOp{{name: tf.name, fn: draw}}, size)
		if n, at := diffPixels(full, inc); n != 0 {
			t.Fatalf("image %s: %d pixel differences, first at %v", tf.name, n, at)
		}
	}
}

// mul composes b onto a and is a small helper for building test matrices.
func mul(a, b draw2d.Matrix) draw2d.Matrix {
	out := a.Copy()
	out.Compose(b)
	return out
}

// TestRotatedCurveParity isolates the rotated-curve workload requested for
// the dirty-region comparison.
func TestRotatedCurveParity(t *testing.T) {
	size := image.Rect(0, 0, 260, 260)
	op := drawOp{
		name: "rotated-curve",
		fn: func(gc *GraphicContext) {
			gc.Save()
			gc.Rotate(0.9)
			gc.Translate(70, 30)
			gc.SetStrokeColor(color.RGBA{0x33, 0x66, 0x99, 0xff})
			gc.SetLineWidth(6)
			gc.MoveTo(0, 0)
			gc.CubicCurveTo(20, -60, 120, 80, 160, 20)
			gc.Stroke()
			gc.SetFillColor(color.RGBA{0x99, 0x33, 0x66, 0xdd})
			gc.MoveTo(10, 40)
			gc.QuadCurveTo(80, 120, 150, 90)
			gc.Close()
			gc.Fill()
			gc.Restore()
		},
	}
	full := render(t, false, []drawOp{op}, size)
	inc := render(t, true, []drawOp{op}, size)
	if n, at := diffPixels(full, inc); n != 0 {
		t.Fatalf("rotated curve: %d differences, first at %v", n, at)
	}
}

// TestRecMutexReentrancy checks the lock semantics directly, including that
// another goroutine waits for the full depth to unwind.
func TestRecMutexReentrancy(t *testing.T) {
	m := newRecMutex()
	m.lock()
	m.lock()
	m.lock()
	if m.depth != 3 {
		t.Fatalf("depth = %d, want 3", m.depth)
	}
	acquired := make(chan struct{})
	go func() {
		m.lock()
		m.unlock()
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("other goroutine entered while lock was held")
	case <-func() chan struct{} {
		done := make(chan struct{})
		go func() {
			runtime.Gosched()
			close(done)
		}()
		return done
	}():
	}
	m.unlock()
	m.unlock()
	m.unlock()
	<-acquired
}
