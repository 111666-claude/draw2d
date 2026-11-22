// Copyright 2018 The draw2d Authors. All rights reserved.

package draw2d

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d/cache"
)

// DefaultFontCacheCapacity is the capacity of the global font cache created
// at package initialization. The number of distinct fonts used by an
// application is small, but the bound keeps the cache finite when arbitrary
// FontData values arrive.
const DefaultFontCacheCapacity = 256

// fontCacheKey uniquely identifies a font. The namer result is stored
// explicitly so renaming via SetFontNamer never aliases two fonts in the same
// cache entry.
type fontCacheKey struct {
	name   string
	family FontFamily
	style  FontStyle
	file   string
}

func newFontCacheKey(d FontData, namer FontFileNamer) fontCacheKey {
	return fontCacheKey{
		name:   d.Name,
		family: d.Family,
		style:  d.Style,
		file:   namer(d),
	}
}

// FontLoader resolves and parses a font. Implementations must be safe for
// concurrent use.
type FontLoader interface {
	// LoadFont reads and parses the font identified by fontData. A non-nil
	// error means the font could not be loaded; such results are not cached
	// and the next request retries the load.
	LoadFont(fontData FontData) (*truetype.Font, error)
}

// FontLoaderContext is implemented by loaders that accept a context, for
// example to abort a blocked disk read when the caller is cancelled.
type FontLoaderContext interface {
	LoadFontContext(ctx context.Context, fontData FontData) (*truetype.Font, error)
}

// FolderLoader loads TTF files from a directory using a FontFileNamer. It is
// the default loader and is safe for concurrent use; the folder and namer may
// be reconfigured while loads are in flight.
type FolderLoader struct {
	mu     sync.RWMutex
	folder string
	namer  FontFileNamer
}

// NewFolderLoader creates a FolderLoader reading fonts from folder.
func NewFolderLoader(folder string) *FolderLoader {
	return &FolderLoader{folder: folder, namer: FontFileName}
}

// SetFolder changes the folder fonts are read from.
func (l *FolderLoader) SetFolder(folder string) {
	l.mu.Lock()
	l.folder = filepath.Clean(folder)
	l.mu.Unlock()
}

// SetNamer changes the function mapping FontData to a file name. A nil namer
// restores the default FontFileName.
func (l *FolderLoader) SetNamer(namer FontFileNamer) {
	if namer == nil {
		namer = FontFileName
	}
	l.mu.Lock()
	l.namer = namer
	l.mu.Unlock()
}

// Folder returns the configured font folder.
func (l *FolderLoader) Folder() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.folder
}

// LoadFont implements FontLoader.
func (l *FolderLoader) LoadFont(fontData FontData) (*truetype.Font, error) {
	return l.LoadFontContext(context.Background(), fontData)
}

// LoadFontContext implements FontLoaderContext. The context is honored while
// reading the file; parsing itself is CPU-bound and not interruptible.
func (l *FolderLoader) LoadFontContext(ctx context.Context, fontData FontData) (*truetype.Font, error) {
	l.mu.RLock()
	folder := l.folder
	namer := l.namer
	l.mu.RUnlock()

	file := namer(fontData)
	path := filepath.Join(folder, file)
	// ioutil.ReadFile does not take a context, so run it on a goroutine and
	// race it against cancellation. A read abandoned this way still completes
	// in the background; its result is simply discarded.
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		data, err := ioutil.ReadFile(path)
		ch <- readResult{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		font, err := truetype.Parse(res.data)
		if err != nil {
			return nil, fmt.Errorf("draw2d: parse font %q: %w", path, err)
		}
		return font, nil
	}
}

// FontCacheConfig configures a [SyncFontCache].
type FontCacheConfig struct {
	// Capacity bounds the number of parsed fonts kept in memory. Zero or
	// negative means unlimited.
	Capacity int
	// TTL, when positive, expires cached fonts after the given duration so
	// long-lived processes can pick up fonts replaced on disk.
	TTL time.Duration
}

// SyncFontCache is a concurrency-safe, bounded FontCache with single-flight
// loading, LRU and optional TTL eviction.
//
// Concurrent Load calls for the same FontData result in exactly one loader
// execution. Errors are not cached, so a failed or corrupted read is retried
// on the next request. Evicted entries are transparently reloaded on demand.
//
// SyncFontCache implements the FontCache interface and can be installed
// globally with SetFontCache.
type SyncFontCache struct {
	loader FontLoader
	namer  FontFileNamer
	lru    *cache.LRU
	group  cache.Group
}

// NewSyncFontCache builds a bounded concurrent font cache backed by loader.
// A nil loader is rejected: use NewFolderFontCache-style caches for a
// self-contained folder cache.
func NewSyncFontCache(loader FontLoader, cfg FontCacheConfig) (*SyncFontCache, error) {
	if loader == nil {
		return nil, fmt.Errorf("draw2d: NewSyncFontCache requires a non-nil loader")
	}
	namer := FontFileName
	return &SyncFontCache{
		loader: loader,
		namer:  namer,
		lru: cache.NewLRU(cache.LRUConfig{
			Capacity: cfg.Capacity,
			TTL:      cfg.TTL,
		}),
	}, nil
}

// NewDefaultSyncFontCache builds a font cache reading from folder with the
// default capacity and no TTL. It is equivalent to the cache installed
// globally at package initialization.
func NewDefaultSyncFontCache(folder string) *SyncFontCache {
	return NewDefaultSyncFontCacheFrom(NewFolderLoader(folder))
}

// NewDefaultSyncFontCacheFrom builds a default-capacity, non-expiring font
// cache backed by loader.
func NewDefaultSyncFontCacheFrom(loader FontLoader) *SyncFontCache {
	c, err := NewSyncFontCache(loader, FontCacheConfig{
		Capacity: DefaultFontCacheCapacity,
	})
	if err != nil {
		// Only possible with a nil loader.
		panic(err)
	}
	return c
}

// Load implements FontCache: it returns a cached font or performs a
// single-flight load through the configured loader.
func (c *SyncFontCache) Load(fontData FontData) (*truetype.Font, error) {
	return c.LoadContext(context.Background(), fontData)
}

// LoadContext is Load with cancellation. Cancellation unblocks this caller
// but never interrupts an in-flight load shared with other goroutines.
func (c *SyncFontCache) LoadContext(ctx context.Context, fontData FontData) (*truetype.Font, error) {
	key := newFontCacheKey(fontData, c.namer)
	if v, ok := c.lru.Get(key.string()); ok {
		return v.(*truetype.Font), nil
	}
	// The loader belongs to the leader goroutine. A follower that cancels
	// while a load is in flight must not propagate its (possibly already
	// cancelled) context into the shared disk read.
	loadCtx := context.Background()
	v, leader, err := c.group.Do(ctx, key.string(), func() (interface{}, error) {
		// Re-check under the single-flight lock: another goroutine may have
		// finished loading this key while we were waiting.
		if cached, ok := c.lru.Get(key.string()); ok {
			return cached, nil
		}
		font, err := c.loadFont(loadCtx, fontData)
		if err != nil {
			return nil, err
		}
		c.lru.Add(key.string(), font)
		return font, nil
	})
	if err != nil {
		// Only the leader's failure counts as a load error. A follower whose
		// own context ended while waiting neither executed nor deduplicated a
		// completed load.
		if leader {
			c.lru.AddLoad()
			c.lru.AddLoadError()
		} else if ctx.Err() == nil {
			c.lru.AddLoadDedup()
		}
		return nil, err
	}
	if leader {
		c.lru.AddLoad()
	} else {
		c.lru.AddLoadDedup()
	}
	return v.(*truetype.Font), nil
}

// loadFont dispatches to the context-aware loader when available.
func (c *SyncFontCache) loadFont(ctx context.Context, fontData FontData) (*truetype.Font, error) {
	if lc, ok := c.loader.(FontLoaderContext); ok {
		return lc.LoadFontContext(ctx, fontData)
	}
	return c.loader.LoadFont(fontData)
}

// Store implements FontCache. A nil font is ignored so a broken caller cannot
// poison the cache with an entry that would panic on use. Repeated Store of
// the same FontData replaces the entry and refreshes its LRU/TTL position.
func (c *SyncFontCache) Store(fontData FontData, font *truetype.Font) {
	if font == nil {
		return
	}
	key := newFontCacheKey(fontData, c.namer)
	c.lru.Add(key.string(), font)
}

// Len returns the number of cached fonts.
func (c *SyncFontCache) Len() int {
	return c.lru.Len()
}

// Stats returns a snapshot of the cache counters.
func (c *SyncFontCache) Stats() cache.Stats {
	return c.lru.Snapshot()
}

// Close stops background TTL eviction and clears the cache.
func (c *SyncFontCache) Close() {
	c.lru.Close()
}

// string renders the key without importing fmt on the hot path beyond this
// single allocation.
func (k fontCacheKey) string() string {
	return k.file + "|" + k.name
}
