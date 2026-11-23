// Copyright 2010 The draw2d Authors. All rights reserved.

package draw2dimg

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/golang/freetype/raster"
	"github.com/llgcode/draw2d"
)

// dirtyMargin is the device-space margin (in pixels) added around every
// computed affected region. The freetype rasterizer is anti-aliased and a
// segment crossing a pixel boundary partially paints the neighbouring pixel.
// A one pixel margin guarantees that the sub-rasterizer never clamps a cell
// that the full-canvas rasterizer would have kept, which is what makes an
// incremental repaint pixel identical to a full repaint.
const dirtyMargin = 1.0

// fullAreaFraction is the fraction of the canvas area above which repainting
// a merged dirty region is not cheaper than a full-canvas rasterization.
const fullAreaFraction = 3.0 / 4.0

// Stats reports the work performed by a GraphicContext. In incremental mode
// the counters let callers (and tests) verify that only affected pixels are
// rasterized and painted.
type Stats struct {
	// FullRepaints counts full-canvas rasterizations (incremental mode
	// disabled, or an explicit full-canvas fallback).
	FullRepaints int64
	// PartialRepaints counts sub-region rasterizations.
	PartialRepaints int64
	// PaintedPixels counts pixels handed to the Painter through spans. A
	// pixel painted several times is counted several times.
	PaintedPixels int64
	// RasterizedPixels counts the area of the rasterizers consumed:
	// width*height of every (full or partial) rasterization.
	RasterizedPixels int64
}

// countingPainter wraps a Painter and counts the pixels it receives while
// leaving every produced byte untouched.
type countingPainter struct {
	painter Painter
	stats   *Stats
}

func (p countingPainter) Paint(ss []raster.Span, done bool) {
	for _, s := range ss {
		if s.X1 > s.X0 {
			p.stats.PaintedPixels += int64(s.X1 - s.X0)
		}
	}
	p.painter.Paint(ss, done)
}

func (p countingPainter) SetColor(c color.Color) {
	p.painter.SetColor(c)
}

// recMutex is a re-entrant mutual exclusion lock. The graphic context needs
// re-entrancy because the glyph cache re-enters it through Save/Fill/Restore
// while a FillStringAt submission is in progress. Re-entrancy is always
// synchronous: a nested submission runs on the same goroutine that already
// owns the outer submission.
//
// The lock serializes goroutines through depth and an owner token. Resolving
// the (relatively expensive, stack-based) goroutine token happens once per
// outer submission; a nested submission is recognized purely from the depth,
// which is only ever incremented synchronously by the owner, so it reuses the
// recorded owner token without resolving a goroutine id. All fields are
// guarded by mu.
type recMutex struct {
	mu    sync.Mutex
	cond  *sync.Cond
	owner int64 // token of the owning goroutine, 0 when unlocked
	depth int
}

func newRecMutex() *recMutex {
	m := new(recMutex)
	m.cond = sync.NewCond(&m.mu)
	return m
}

// begin is the submission entry point. It is called both by outer
// submissions and, synchronously on the owner goroutine, by nested glyph
// rendering. myToken is the caller's goroutine token (goroutineToken is cheap
// after the first call per goroutine). With the mutex held:
//
//   - depth == 0: an outer submission that became the sole owner after any
//     contention wait; it records its token.
//   - depth > 0 and myToken == owner: a synchronous nested submission on the
//     owner goroutine; the depth is incremented without further work.
//   - depth > 0 and myToken != owner: a different goroutine; it waits on the
//     condition variable until the owner fully unwinds, then takes over.
//
// All fields are read and written under mu, so the depth/owner decision can
// never race.
func (m *recMutex) begin(myToken int64) (int64, func()) {
	m.mu.Lock()
	for m.depth != 0 && m.owner != myToken {
		m.cond.Wait()
	}
	if m.depth == 0 {
		m.owner = myToken
	}
	token := m.owner
	m.depth++
	m.mu.Unlock()
	return token, func() { m.release(token) }
}

// beginDraw is the graphic-context submission entry. It resolves the caller's
// goroutine token and delegates to begin, which under the mutex distinguishes
// an outer submission (depth == 0), a synchronous nested re-entry (depth > 0,
// same owner token) and contention from another goroutine (condition wait).
func (m *recMutex) beginDraw(resolveToken func() int64) (int64, func()) {
	token := resolveToken()
	return m.begin(token)
}

// release drops one lock level held by token and wakes waiters at depth zero.
func (m *recMutex) release(token int64) {
	m.mu.Lock()
	if m.depth == 0 || m.owner != token {
		m.mu.Unlock()
		return
	}
	m.depth--
	if m.depth == 0 {
		m.owner = 0
		m.cond.Broadcast()
	}
	m.mu.Unlock()
}

// lock/unlock are thin wrappers for the management entry points
// (SetIncremental, Invalidate, DirtyRegions, ...). These are not part of a
// nested draw, so resolving a token once is acceptable.
func (m *recMutex) lock() {
	m.begin(goroutineToken())
}

func (m *recMutex) unlock() {
	m.release(goroutineToken())
}

// goroutineToken returns a process-unique token identifying the calling
// goroutine. Portable Go exposes a goroutine id only through its runtime
// stack header, so the id is parsed per call and mapped to a stable token in
// goroutineTokens. A map (rather than a fixed hash table) is used to avoid
// token aliasing when many goroutines draw concurrently.
var (
	goroutineTokens sync.Map // map[int64]int64: runtime goroutine id -> token
	gidTokenCounter int64
)

func goroutineToken() int64 {
	gid := rawGoroutineID()
	if v, ok := goroutineTokens.Load(gid); ok {
		return v.(int64)
	}
	token := atomic.AddInt64(&gidTokenCounter, 1)
	actual, loaded := goroutineTokens.LoadOrStore(gid, token)
	if loaded {
		return actual.(int64)
	}
	return token
}

// rawGoroutineID reads the id out of the runtime stack header, which looks
// like "goroutine 18 [running]:\n".
func rawGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := buf[:n]
	if i := bytes.IndexByte(line, ' '); i >= 0 {
		line = line[i+1:]
	}
	if i := bytes.IndexByte(line, ' '); i >= 0 {
		line = line[:i]
	}
	id, err := strconv.ParseInt(string(line), 10, 64)
	if err != nil {
		return -atomic.AddInt64(&gidTokenCounter, 1)
	}
	return id
}

// SetIncremental enables or disables incremental dirty-rectangle rendering.
//
// When enabled, every drawing submission (Fill, Stroke, FillStroke,
// FillStringAt, StrokeStringAt, DrawImage, Clear and ClearRect) records the
// device-space region it affects in a dirty list; rasterization only touches
// the affected pixels. Accumulated regions are returned by DirtyRegions and
// cleared by ResetDirty.
//
// When disabled (the default) the graphic context behaves exactly like a
// non-incremental context and the produced image is pixel identical to the
// historical full-canvas output.
//
// Switching modes does not clear the image nor the pending dirty regions.
func (gc *GraphicContext) SetIncremental(enabled bool) {
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	gc.incremental = enabled
}

// IsIncremental reports whether incremental dirty-rectangle rendering is on.
func (gc *GraphicContext) IsIncremental() bool {
	return gc.incremental
}

// Invalidate marks a device-space rectangle as dirty. It is the hook used by
// interactive canvases before moving or deleting an object: the caller
// invalidates both the region the object previously occupied and the region
// the redraw will occupy. The rectangle is clipped to the canvas; an empty
// intersection is ignored. Inverted rectangles force a full-canvas
// invalidation, which is always a safe conservative result and can never miss
// a pixel.
func (gc *GraphicContext) Invalidate(r image.Rectangle) {
	if !gc.incremental {
		return
	}
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	gc.addDirtyRect(r)
}

// InvalidateFull marks the whole canvas dirty.
func (gc *GraphicContext) InvalidateFull() {
	if !gc.incremental {
		return
	}
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	gc.markFullDirty()
}

// DirtyRegions returns a copy of the accumulated device-space dirty
// regions. Regions that intersected or touched when they were recorded have
// been merged; returned rectangles are always clipped to the canvas and never
// overlap. The returned slice may be modified by the caller and is not
// affected by later drawing. A nil result means nothing is dirty.
func (gc *GraphicContext) DirtyRegions() []image.Rectangle {
	if !gc.incremental {
		return nil
	}
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	if gc.fullDirty {
		return []image.Rectangle{gc.canvasRect()}
	}
	if len(gc.dirty) == 0 {
		return nil
	}
	out := make([]image.Rectangle, len(gc.dirty))
	copy(out, gc.dirty)
	return out
}

// ResetDirty clears every accumulated dirty region. It does not modify the
// image pixels.
func (gc *GraphicContext) ResetDirty() {
	if !gc.incremental {
		return
	}
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	gc.resetDirty()
}

// Stats returns a snapshot of the rendering counters. Counters are only
// collected when stats have been enabled with EnableStats, so regular
// rendering pays no wrapper overhead.
func (gc *GraphicContext) Stats() Stats {
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	return gc.stats
}

// EnableStats turns work counters on or off. Enabling it wraps the painter
// with a pixel counting decorator; disabling restores the original painter.
// The image pixels and dirty state are unaffected. It works in both rendering
// modes so full and incremental runs can be compared.
func (gc *GraphicContext) EnableStats(enabled bool) {
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	if enabled {
		if _, ok := gc.painter.(countingPainter); !ok {
			gc.painter = countingPainter{painter: gc.basePainter, stats: &gc.stats}
		}
	} else {
		gc.painter = gc.basePainter
	}
}

// ResetStats zeroes the rendering counters.
func (gc *GraphicContext) ResetStats() {
	gc.dirtyMu.lock()
	defer gc.dirtyMu.unlock()
	gc.stats = Stats{}
	if cp, ok := gc.painter.(countingPainter); ok {
		cp.stats = &gc.stats
	}
}

// canvasRect returns the device bounds of the image.
func (gc *GraphicContext) canvasRect() image.Rectangle {
	return gc.img.Bounds()
}

func (gc *GraphicContext) resetDirty() {
	gc.dirty = gc.dirty[:0]
	gc.fullDirty = false
}

func (gc *GraphicContext) markFullDirty() {
	gc.resetDirty()
	gc.fullDirty = true
}

// addDirtyRect clips r to the canvas and merges it into the dirty list.
// Degenerate input has well defined results: a canonical empty rectangle is a
// no-op, while a rectangle whose coordinates escape the representable or
// supported range degenerates to a full-canvas invalidation. Over-repainting
// is harmless whereas missing a region is not, so the conservative branch is
// always preferred over trying to be clever.
func (gc *GraphicContext) addDirtyRect(r image.Rectangle) {
	if r.Empty() {
		return
	}
	// Coordinates beyond ~1e9 cannot be turned into meaningful pixel boxes
	// without risking overflow in later arithmetic.
	if r.Min.X < -1e9 || r.Min.Y < -1e9 || r.Max.X > 1e9 || r.Max.Y > 1e9 {
		gc.markFullDirty()
		return
	}
	canvas := gc.canvasRect()
	r = r.Intersect(canvas)
	if r.Empty() {
		return
	}
	if gc.fullDirty {
		return
	}
	for i, d := range gc.dirty {
		// Merge on intersection or adjacency so touching updates produce a
		// single repaint region.
		if regionsTouch(r, d) {
			gc.dirty[i] = d.Union(r)
			return
		}
	}
	gc.dirty = append(gc.dirty, r)
}

// regionsTouch reports whether two canvas-clipped rectangles overlap or are
// edge adjacent, in which case repainting them together is at least as cheap
// and keeps the dirty list non-overlapping.
func regionsTouch(a, b image.Rectangle) bool {
	if a.Overlaps(b) {
		return true
	}
	return !a.Intersect(b.Add(image.Point{1, 1})).Empty() ||
		!b.Intersect(a.Add(image.Point{1, 1})).Empty()
}

// addDirtyFloat is the float64 entry point used by the drawing primitives.
// The bounds are grown outward before rounding so anti-aliased edge pixels
// are included. Any non-finite value forces a full-canvas fallback.
func (gc *GraphicContext) addDirtyFloat(x0, y0, x1, y1 float64) {
	if !finite(x0) || !finite(y0) || !finite(x1) || !finite(y1) || x1 < x0 || y1 < y0 {
		gc.markFullDirty()
		return
	}
	// Guard against absurdly large values that would overflow int.
	if x0 < -1e9 || y0 < -1e9 || x1 > 1e9 || y1 > 1e9 {
		gc.markFullDirty()
		return
	}
	r := image.Rect(
		int(math.Floor(x0-dirtyMargin)),
		int(math.Floor(y0-dirtyMargin)),
		int(math.Ceil(x1+dirtyMargin)),
		int(math.Ceil(y1+dirtyMargin)),
	)
	gc.addDirtyRect(r)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// pathDeviceBounds computes the axis aligned device-space bounding box of the
// given paths transformed by tr. Control points of curves are included, which
// gives a conservative box for the flattened segments the rasterizer actually
// receives. ok is false if a path is empty or contains a non-finite point.
func pathDeviceBounds(paths []*draw2d.Path, tr draw2d.Matrix) (x0, y0, x1, y1 float64, ok bool) {
	first := true
	valid := true
	absorb := func(u, v float64) {
		if !finite(u) || !finite(v) {
			valid = false
			return
		}
		if first {
			x0, x1 = u, u
			y0, y1 = v, v
			first = false
			return
		}
		if u < x0 {
			x0 = u
		}
		if u > x1 {
			x1 = u
		}
		if v < y0 {
			y0 = v
		}
		if v > y1 {
			y1 = v
		}
	}
	anyPoint := false
	for _, p := range paths {
		if p == nil || len(p.Components) == 0 {
			continue
		}
		anyPoint = true
		i := 0
		for _, cmp := range p.Components {
			switch cmp {
			case draw2d.MoveToCmp, draw2d.LineToCmp:
				u, v := tr.TransformPoint(p.Points[i], p.Points[i+1])
				absorb(u, v)
				i += 2
			case draw2d.QuadCurveToCmp:
				u, v := tr.TransformPoint(p.Points[i], p.Points[i+1])
				absorb(u, v)
				u, v = tr.TransformPoint(p.Points[i+2], p.Points[i+3])
				absorb(u, v)
				i += 4
			case draw2d.CubicCurveToCmp:
				for k := 0; k < 3; k++ {
					u, v := tr.TransformPoint(p.Points[i+2*k], p.Points[i+2*k+1])
					absorb(u, v)
				}
				i += 6
			case draw2d.ArcToCmp:
				// Center, radii and angles: absorb the four control corners
				// of the ellipse bounding box plus the arc end points.
				cx, cy := p.Points[i], p.Points[i+1]
				rx, ry := p.Points[i+2], p.Points[i+3]
				startAngle, angle := p.Points[i+4], p.Points[i+5]
				for _, q := range [][2]float64{
					{cx - rx, cy - ry}, {cx + rx, cy - ry},
					{cx - rx, cy + ry}, {cx + rx, cy + ry},
					{cx + math.Cos(startAngle)*rx, cy + math.Sin(startAngle)*ry},
					{cx + math.Cos(startAngle+angle)*rx, cy + math.Sin(startAngle+angle)*ry},
				} {
					u, v := tr.TransformPoint(q[0], q[1])
					absorb(u, v)
				}
				i += 6
			case draw2d.CloseCmp:
				// no points
			}
			if !valid {
				return 0, 0, 0, 0, false
			}
		}
	}
	if !anyPoint || first {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

// strokeExpand returns the device-space half width of a stroked path. It
// measures the current matrix on both device axes instead of assuming an
// isotropic scale, so anisotropic transformations keep a conservative box.
func strokeExpand(tr draw2d.Matrix, lineWidth float64) float64 {
	if !finite(lineWidth) || lineWidth <= 0 {
		// A zero width stroke is still rasterized as a one pixel hairline.
		lineWidth = 1
	}
	// Length of a unit user-space vector mapped on each device axis.
	fx := math.Hypot(tr[0], tr[1])
	fy := math.Hypot(tr[2], tr[3])
	f := fx
	if fy > f {
		f = fy
	}
	expand := lineWidth * f * 0.5
	if !finite(expand) || expand < 0.5 {
		expand = 0.5
	}
	return expand
}

// recordFill computes and records the region a Fill submission affects.
func (gc *GraphicContext) recordFill(paths []*draw2d.Path) {
	x0, y0, x1, y1, ok := pathDeviceBounds(paths, gc.Current.Tr)
	if !ok {
		gc.markFullDirty()
		return
	}
	gc.addDirtyFloat(x0, y0, x1, y1)
}

// recordStroke computes and records the region a Stroke submission affects,
// taking line width and joins/caps overshoot into account.
func (gc *GraphicContext) recordStroke(paths []*draw2d.Path) {
	x0, y0, x1, y1, ok := pathDeviceBounds(paths, gc.Current.Tr)
	if !ok {
		gc.markFullDirty()
		return
	}
	e := strokeExpand(gc.Current.Tr, gc.Current.LineWidth)
	gc.addDirtyFloat(x0-e, y0-e, x1+e, y1+e)
}

// recordImage records the device region covered by an image transformed by tr.
func (gc *GraphicContext) recordImage(src image.Image, tr draw2d.Matrix) {
	b := src.Bounds()
	if b.Empty() {
		return
	}
	x0, y0, x1, y1 := tr.TransformRectangle(
		float64(b.Min.X), float64(b.Min.Y),
		float64(b.Max.X), float64(b.Max.Y),
	)
	gc.addDirtyFloat(x0, y0, x1, y1)
}

// shouldUseFullRasterizer decides whether a region is too large to benefit
// from a sub-rasterizer.
func (gc *GraphicContext) shouldUseFullRasterizer(area int) bool {
	canvas := gc.canvasRect()
	return area >= int(float64(canvas.Dx()*canvas.Dy())*fullAreaFraction)
}

// rasterTarget binds the rasterizer that must receive the geometry of one
// submission together with the device-space translation its adder applies.
// In non-incremental mode, and for full-canvas fallbacks, it is the original
// full-size rasterizer with no translation. For a partial submission it is a
// reused sub-rasterizer whose origin is the region corner; the translating
// adder feeds it region-local coordinates.
type rasterTarget struct {
	rasterizer *raster.Rasterizer
	region     image.Rectangle // empty means full canvas
	scratch    **raster.Rasterizer
}

// prepareTarget selects the rasterizer for a submission whose newly recorded
// region is r (empty for full canvas). The returned target is ready to
// receive geometry through its adder and to be finished by finishTarget.
func (gc *GraphicContext) prepareTarget(fullSize *raster.Rasterizer, scratch **raster.Rasterizer, region image.Rectangle) rasterTarget {
	if !gc.incremental || region.Empty() ||
		gc.shouldUseFullRasterizer(region.Dx()*region.Dy()) ||
		!region.In(gc.canvasRect()) {
		return rasterTarget{rasterizer: fullSize, scratch: scratch}
	}
	sub := *scratch
	if sub == nil {
		sub = raster.NewRasterizer(region.Dx(), region.Dy())
		*scratch = sub
	} else {
		sub.SetBounds(region.Dx(), region.Dy())
	}
	// The span offset shifts painted spans back to device coordinates; the
	// translating adder (see lineBuilder) maps device points to local ones.
	sub.Dx, sub.Dy = region.Min.X, region.Min.Y
	// The sub-rasterizer is a distinct object: it must inherit the fill
	// rule that the call site configured on the full-size rasterizer,
	// otherwise a self-intersecting outline (every stroked path is one)
	// rasterizes with the wrong winding and corner pixels change.
	sub.UseNonZeroWinding = fullSize.UseNonZeroWinding
	return rasterTarget{rasterizer: sub, region: region, scratch: scratch}
}

// finishTarget rasterizes the accumulated geometry into the image and clears
// the involved rasterizers, preserving the historical Clear() ordering.
func (gc *GraphicContext) finishTarget(t rasterTarget, c color.Color, fullSize *raster.Rasterizer) {
	gc.painter.SetColor(c)
	canvas := gc.canvasRect()
	if t.region.Empty() {
		gc.stats.FullRepaints++
		gc.stats.RasterizedPixels += int64(canvas.Dx() * canvas.Dy())
		fullSize.Rasterize(gc.painter)
		fullSize.Clear()
		return
	}
	gc.stats.PartialRepaints++
	gc.stats.RasterizedPixels += int64(t.region.Dx() * t.region.Dy())
	t.rasterizer.Rasterize(gc.painter)
	t.rasterizer.Clear()
	// The full-size rasterizer received no geometry in partial mode, but
	// clearing it keeps its state identical to the historical pipeline.
	fullSize.Clear()
}

// lineBuilder returns the FtLineBuilder bound to the selected target. A
// partial target translates device coordinates into region-local coordinates;
// a full-canvas target receives raw device coordinates as before.
func (gc *GraphicContext) lineBuilder(t rasterTarget) FtLineBuilder {
	if !t.region.Empty() {
		return FtLineBuilder{Adder: newTranslatingAdder(t.rasterizer, t.region.Min.X, t.region.Min.Y)}
	}
	return FtLineBuilder{Adder: t.rasterizer}
}

// beginDraw serializes concurrent submissions. In non-incremental mode it is
// a zero-cost no-op. In incremental mode an outer submission resolves its
// goroutine token (the first call per goroutine formats a runtime stack, later
// calls hit the token table); a nested submission re-entered through
// synchronous glyph rendering carries the owner's token and re-enters without
// any stack formatting. A different goroutine blocks on the condition
// variable. All lock state is guarded by the internal mutex.
func (gc *GraphicContext) beginDraw() (int64, func()) {
	if !gc.incremental {
		return 0, func() {}
	}
	return gc.dirtyMu.beginDraw(goroutineToken)
}

// snapshotDirty captures the dirty state before a primitive records its own
// region. The rectangle list is copied defensively so a later append that
// reuses the slice's backing array cannot silently mutate the snapshot.
func (gc *GraphicContext) snapshotDirty() (full bool, rects []image.Rectangle) {
	if gc.fullDirty {
		return true, nil
	}
	before := make([]image.Rectangle, len(gc.dirty))
	copy(before, gc.dirty)
	return false, before
}

// regionSince returns the region contributed since the given snapshot. The
// caller holds the submission lock (drawings are serialized and the lock is
// re-entrant for nested glyph rendering), so the only possible state changes
// are appends and merges made by the current submission itself. An empty
// rectangle requests a full-canvas fallback; nil means the submission did not
// touch the canvas (for example an empty path fully outside the image).
func (gc *GraphicContext) regionSince(full bool, before []image.Rectangle) (region image.Rectangle, touched bool) {
	if full || gc.fullDirty {
		return image.Rectangle{}, true
	}
	canvas := gc.canvasRect()
	// Rectangles appended after the snapshot, or whose box grew through a
	// merge, are the newly affected ones. Compare by index: existing entries
	// that did not merge stay pointer-stable in gc.dirty.
	var r image.Rectangle
	hit := false
	for i, d := range gc.dirty {
		if i < len(before) && before[i] == d {
			continue
		}
		dd := d.Intersect(canvas)
		if dd.Empty() {
			continue
		}
		if !hit {
			r = dd
			hit = true
		} else {
			r = r.Union(dd)
		}
	}
	if !hit {
		return image.Rectangle{}, false
	}
	if gc.shouldUseFullRasterizer(r.Dx() * r.Dy()) {
		return image.Rectangle{}, true
	}
	return r, true
}
