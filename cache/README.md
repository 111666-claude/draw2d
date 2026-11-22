# Bounded concurrent caches for draw2d

The `cache` package provides the concurrency primitives used by draw2d's
bounded font and glyph caches:

- **`Group`** — single-flight: concurrent requests for the same key share one
  loader execution. The loader always runs to completion; a cancelled follower
  returns early without cancelling the shared load, and a panicking loader is
  recovered so waiters never deadlock.
- **`LRU`** — thread-safe, capacity-bounded map with LRU eviction and optional
  per-entry TTL. TTL expiry is enforced lazily on lookup and, when a TTL is
  configured, by a background janitor goroutine that `Close` stops.

The higher-level caches are:

- `draw2d.SyncFontCache` (root package): implements `draw2d.FontCache`, wraps
  any `FontLoader` (the default `FolderLoader` reads TTF files), deduplicates
  concurrent `Load` calls, never caches errors, and supports `LoadContext` for
  cancellation.
- `draw2dbase.SyncGlyphCache`: implements `draw2dbase.GlyphCache`, deduplicates
  concurrent glyph rendering and always returns an independent `*Glyph` copy,
  so callers never observe a shared mutable `Path`.

Both report a `cache.Stats` snapshot (`Hits`, `Misses`, `Loads`, `LoadErrors`,
`LoadDedups`, `Evictions`, `Expirations`).

## Configuration

```go
fontCache, _ := draw2d.NewSyncFontCache(
    draw2d.NewFolderLoader("/usr/share/fonts/ttf"),
    draw2d.FontCacheConfig{Capacity: 128, TTL: time.Hour},
)
draw2d.SetFontCache(fontCache)

glyphCache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{
    Capacity: 8192,
})
gc.SetGlyphCache(glyphCache) // draw2dimg / draw2dsvg / draw2dgl
```

A zero or negative `Capacity` means unbounded. The legacy `FolderFontCache`,
`SyncFolderFontCache` and `GlyphCacheImp` types remain available and still
satisfy the same interfaces, so existing code is unaffected.
