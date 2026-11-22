// Package draw2ddiff implements a cross-backend rendering diff gate for
// draw2d. A fixed set of scenes (paths, text, images, transforms, alpha,
// plus edge-case scenes) is rendered through the PNG (draw2dimg), SVG
// (draw2dsvg) and PDF (draw2dpdf) backends and the resulting raster or
// structural snapshots are compared against versioned golden baselines.
//
// The gate reports pixel error, SVG/PDF element counts and the first
// difference position for every scene/backend pair. Anti-aliasing and
// font tolerances are explicit (see Tolerances). Golden baselines are
// only (re)written when explicitly requested (-update-golden flag or
// DRAW2D_UPDATE_GOLDEN=1); a default `go test` run never modifies the
// golden directory. On failure, diff images and minimal reproduction
// inputs are kept in a separate artifact directory.
package draw2ddiff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"
	"strings"
)

// Backend identifies one rendering backend.
type Backend string

const (
	BackendPNG Backend = "png"
	BackendSVG Backend = "svg"
	BackendPDF Backend = "pdf"
)

// Backends lists all backends covered by the gate, in stable order.
var Backends = []Backend{BackendPNG, BackendSVG, BackendPDF}

// Category is the stable classification of a gate result. Edge-case
// scenes and non-comparable outcomes get their own category and are
// never silently passed: any category other than CategoryOK fails the
// gate unless the golden baseline explicitly records the same outcome.
type Category string

const (
	// CategoryOK means the snapshot matches the golden baseline.
	CategoryOK Category = "ok"
	// CategoryPixelDiff means the raster output differs beyond tolerance.
	CategoryPixelDiff Category = "pixel-diff"
	// CategoryStructureDiff means the SVG/PDF structure differs.
	CategoryStructureDiff Category = "structure-diff"
	// CategoryEmptyScene classifies scenes that draw nothing.
	CategoryEmptyScene Category = "empty-scene"
	// CategoryNonZeroBounds classifies scenes with non-zero image bounds.
	CategoryNonZeroBounds Category = "nonzero-bounds"
	// CategoryMissingGlyph classifies scenes using glyphs the font lacks.
	CategoryMissingGlyph Category = "missing-glyph"
	// CategoryNonFinite classifies scenes with NaN/Inf parameters.
	CategoryNonFinite Category = "non-finite-params"
	// CategoryOutputFailure means a backend failed to render or save.
	CategoryOutputFailure Category = "output-failure"
	// CategoryFontPlatform means a pinned font was unavailable, so
	// output would depend on platform font resolution.
	CategoryFontPlatform Category = "font-platform"
	// CategoryMissingGolden means no golden baseline exists yet.
	CategoryMissingGolden Category = "missing-golden"
	// CategoryVersionMismatch means the golden baseline version is
	// not the one this gate understands.
	CategoryVersionMismatch Category = "baseline-version-mismatch"
)

// Tolerances defines the explicit anti-aliasing/font tolerances for
// raster comparison. Structural (SVG/PDF) comparison is always exact.
type Tolerances struct {
	// ChannelDelta is the per-channel absolute difference tolerated
	// on a pixel before it counts as different (anti-aliasing noise).
	ChannelDelta uint32
	// MaxDiffPixels is the absolute number of pixels allowed to differ.
	MaxDiffPixels int
	// MaxDiffFraction is the fraction of pixels allowed to differ.
	// The effective budget is the max of both pixel budgets.
	MaxDiffFraction float64
}

// DefaultTolerances returns the gate defaults: a single level of
// per-channel anti-aliasing slack and no pixel budget.
func DefaultTolerances() Tolerances {
	return Tolerances{ChannelDelta: 1, MaxDiffPixels: 0, MaxDiffFraction: 0}
}

// allowedPixels returns the effective differing-pixel budget.
func (t Tolerances) allowedPixels(total int) int {
	budget := t.MaxDiffPixels
	if f := int(t.MaxDiffFraction * float64(total)); f > budget {
		budget = f
	}
	return budget
}

// Snapshot is the comparable output of one scene on one backend.
type Snapshot struct {
	Scene   string
	Backend Backend

	// Raster (BackendPNG): RGBA pixels, 4 bytes per pixel, row-major.
	Width  int
	Height int
	Pixels []byte

	// Structural (BackendSVG, BackendPDF): element/operator counts and
	// a canonical, deterministically ordered textual form.
	Counts    map[string]int
	Canonical []byte

	// Hash is the sha256 of the comparable payload.
	Hash string
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// NewRasterSnapshot builds a raster snapshot from an image. The image is
// normalized to NRGBA so that a golden PNG round-trip (which may decode
// to RGBA or NRGBA depending on opacity) compares equal to a fresh
// render of the same scene.
func NewRasterSnapshot(scene string, img image.Image) *Snapshot {
	nrgba := toNRGBA(img)
	b := nrgba.Bounds()
	s := &Snapshot{
		Scene:   scene,
		Backend: BackendPNG,
		Width:   b.Dx(),
		Height:  b.Dy(),
		Pixels:  nrgba.Pix,
	}
	s.Hash = hashBytes(s.Pixels)
	return s
}

// toNRGBA normalizes any image to non-premultiplied RGBA.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	n := image.NewNRGBA(b)
	draw.Draw(n, b, img, b.Min, draw.Src)
	return n
}

// NewStructuralSnapshot builds a structural snapshot.
func NewStructuralSnapshot(scene string, backend Backend, counts map[string]int, canonical []byte) *Snapshot {
	return &Snapshot{
		Scene:     scene,
		Backend:   backend,
		Counts:    counts,
		Canonical: canonical,
		Hash:      hashBytes(canonical),
	}
}

// Result is the outcome of comparing one snapshot against its golden.
type Result struct {
	Scene    string
	Backend  Backend
	Category Category
	Passed   bool
	Detail   string

	// Raster metrics.
	DiffPixels      int
	TotalPixels     int
	MaxChannelDelta uint32

	// FirstDiff is the human-readable first difference position:
	// "(x,y)" for raster, "line N" for SVG, "token N" for PDF.
	FirstDiff string

	GoldenHash  string
	CurrentHash string

	// DiffImage is a PNG-encoded visualization of the raster
	// difference (nil for structural backends or when passing).
	DiffImage []byte
}

func (r *Result) String() string {
	status := "FAIL"
	if r.Passed {
		status = "ok  "
	}
	s := fmt.Sprintf("%s %-16s %-4s [%s]", status, r.Scene, r.Backend, r.Category)
	if r.Detail != "" {
		s += " " + r.Detail
	}
	return s
}

// Compare compares a current snapshot against its golden baseline.
// The scene's declared kind determines the classification of the result.
func Compare(scene *Scene, golden, current *Snapshot, tol Tolerances) *Result {
	r := &Result{
		Scene:       scene.Name,
		Backend:     current.Backend,
		Category:    CategoryOK,
		GoldenHash:  golden.Hash,
		CurrentHash: current.Hash,
	}
	if current.Backend == BackendPNG {
		compareRaster(golden, current, tol, r)
	} else {
		compareStructure(golden, current, r)
	}
	if r.Passed {
		r.Category = scene.Kind.category()
	}
	return r
}

// compareRaster fills r with the raster comparison outcome.
func compareRaster(golden, current *Snapshot, tol Tolerances, r *Result) {
	if golden.Width != current.Width || golden.Height != current.Height {
		r.Category = CategoryPixelDiff
		r.Detail = fmt.Sprintf("dimension mismatch: golden %dx%d, current %dx%d",
			golden.Width, golden.Height, current.Width, current.Height)
		return
	}
	total := golden.Width * golden.Height
	r.TotalPixels = total
	gp, cp := golden.Pixels, current.Pixels
	var maxDelta uint32
	diffPixels := 0
	firstX, firstY := -1, -1
	// diff visualization: start from the current image, mark offending
	// pixels in opaque red.
	diffPix := make([]byte, len(cp))
	copy(diffPix, cp)
	for i := 0; i < total; i++ {
		o := i * 4
		var d uint32
		for c := 0; c < 4; c++ {
			delta := absDiff(uint32(gp[o+c]), uint32(cp[o+c]))
			if delta > d {
				d = delta
			}
		}
		if d > maxDelta {
			maxDelta = d
		}
		if d > tol.ChannelDelta {
			diffPixels++
			if firstX < 0 {
				firstX, firstY = i%golden.Width, i/golden.Width
			}
			diffPix[o+0] = 0xff
			diffPix[o+1] = 0x00
			diffPix[o+2] = 0x00
			diffPix[o+3] = 0xff
		}
	}
	r.DiffPixels = diffPixels
	r.MaxChannelDelta = maxDelta
	if diffPixels > tol.allowedPixels(total) {
		r.Category = CategoryPixelDiff
		r.Passed = false
		r.FirstDiff = fmt.Sprintf("(%d,%d)", firstX, firstY)
		r.Detail = fmt.Sprintf("%d/%d pixels differ (max channel delta %d, budget %d), first at %s",
			diffPixels, total, maxDelta, tol.allowedPixels(total), r.FirstDiff)
		img := image.NewNRGBA(image.Rect(0, 0, golden.Width, golden.Height))
		img.Pix = diffPix
		r.DiffImage = encodePNG(img)
		return
	}
	r.Passed = true
	if diffPixels > 0 {
		r.FirstDiff = fmt.Sprintf("(%d,%d)", firstX, firstY)
		r.Detail = fmt.Sprintf("%d/%d pixels differ within tolerance (max channel delta %d)",
			diffPixels, total, maxDelta)
	}
}

// compareStructure fills r with the structural comparison outcome.
func compareStructure(golden, current *Snapshot, r *Result) {
	// Element counts must match exactly.
	keys := map[string]bool{}
	for k := range golden.Counts {
		keys[k] = true
	}
	for k := range current.Counts {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		g, c := golden.Counts[k], current.Counts[k]
		if g != c {
			r.Category = CategoryStructureDiff
			r.Detail = fmt.Sprintf("element count mismatch for %q: golden %d, current %d", k, g, c)
			r.FirstDiff = k
			return
		}
	}
	// Canonical form must match exactly.
	if bytes.Equal(golden.Canonical, current.Canonical) {
		r.Passed = true
		return
	}
	r.Category = CategoryStructureDiff
	gl := strings.Split(string(golden.Canonical), "\n")
	cl := strings.Split(string(current.Canonical), "\n")
	n := len(gl)
	if len(cl) < n {
		n = len(cl)
	}
	for i := 0; i < n; i++ {
		if gl[i] != cl[i] {
			r.FirstDiff = fmt.Sprintf("line %d", i+1)
			r.Detail = fmt.Sprintf("first difference at line %d:\n  golden:  %s\n  current: %s",
				i+1, truncate(gl[i]), truncate(cl[i]))
			return
		}
	}
	r.FirstDiff = fmt.Sprintf("line %d", n+1)
	r.Detail = fmt.Sprintf("canonical forms have different lengths (%d vs %d lines)", len(gl), len(cl))
}

func truncate(s string) string {
	const max = 120
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// Blank reports whether a raster snapshot is fully transparent, used to
// classify empty scenes instead of silently passing them.
func (s *Snapshot) Blank() bool {
	if s.Backend != BackendPNG {
		return len(s.Canonical) == 0 || s.totalElements() == 0
	}
	for i := 3; i < len(s.Pixels); i += 4 {
		if s.Pixels[i] != 0 {
			return false
		}
	}
	return true
}

func (s *Snapshot) totalElements() int {
	n := 0
	for _, c := range s.Counts {
		n += c
	}
	return n
}

// opaque is a helper for scene construction.
func opaque(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }
