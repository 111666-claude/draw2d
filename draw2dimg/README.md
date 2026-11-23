draw2d/draw2dimg
=================

[![Coverage](http://gocover.io/_badge/github.com/llgcode/draw2d/draw2dimg?0)](http://gocover.io/github.com/llgcode/draw2d/draw2dimg)
[![GoDoc](https://godoc.org/github.com/llgcode/draw2d/draw2dimg?status.svg)](https://godoc.org/github.com/llgcode/draw2d/draw2dimg)


draw2d implementation that generates raster images using https://github.com/golang/freetype package.

## Incremental dirty-rectangle rendering

An interactive canvas normally re-rasterizes the whole image for every edit,
which makes dragging even a single object scale with canvas size. The graphic
context can instead track the device-space region each submission affects and
rasterize only that region.

```go
gc := draw2dimg.NewGraphicContext(img)
gc.SetIncremental(true)          // off by default; off == historical output

// ... draw as usual: Fill, Stroke, FillStroke, FillStringAt, DrawImage, ...

regions := gc.DirtyRegions()     // merged, canvas-clipped, non-overlapping
gc.ResetDirty()                  // consume them after presenting a frame
```

When moving or deleting an object, also invalidate the region it previously
occupied so the vacated pixels can be restored:

```go
gc.Invalidate(oldBounds)         // clip/merge one rectangle
gc.InvalidateFull()              // conservative whole-canvas fallback
```

Guarantees:

- Paths, text and images record their transformed device-space bounds, grown by
  the stroke width and a one-pixel anti-aliasing margin, so no affected pixel is
  missed. Intersecting or adjacent regions are merged.
- With incremental mode disabled the output is byte-for-byte identical to the
  full-canvas renderer (the default code path is unchanged).
- Empty regions are ignored; non-finite coordinates and oversized regions fall
  back to a full-canvas repaint rather than clipping or overflowing.
- Submissions are serialized by a re-entrant lock, so concurrent use and the
  glyph cache's nested Save/Fill/Restore are race free.

`EnableStats`/`Stats` expose full vs partial repaint counts and the number of
rasterized and painted pixels for measuring the work saved.

