// Copyright 2026 The draw2d Authors. All rights reserved.
// Image resource deduplication for the pdf backend.

package draw2dpdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"sync"
)

const (
	// imageTypePNG is the encoding used for all registered images.
	imageTypePNG = "PNG"

	// DefaultImageCacheLimit is the default maximum number of distinct
	// image resources cached per GraphicContext. Once the limit is
	// reached, further distinct images are still drawn, but registered
	// under unique names without being cached.
	DefaultImageCacheLimit = 1024
)

// maxCachedPixels bounds the size of images that are hashed for
// deduplication. Larger images are still drawn, but registered under
// unique names without being cached, so hashing cost stays bounded.
// It is a variable to allow tests to lower it.
var maxCachedPixels = int64(1 << 24)

// errEmptyImage is returned for images with zero width or height.
var errEmptyImage = errors.New("draw2dpdf: image has empty bounds")

// imageKey is a stable content key for an encoded image. It combines the
// image geometry (bounds, including a non-zero Min), the encoding
// parameters and two independent hashes over the canonical (premultiplied
// 16-bit RGBA) pixel data. Distinct images therefore never share a key
// unless both hashes collide at once; a collision in a single hash yields
// a different key and thus a false miss (no reuse), never a false merge.
type imageKey struct {
	sum  [sha256.Size]byte // sha256 over params + pixels
	tag  uint64            // fnv-1a over params + pixels
	minX int32
	minY int32
	w    int32
	h    int32
}

// imageCache maps image content keys to pdf image resource names. It is
// safe for concurrent use and never retains the source images: only the
// fixed-size keys and resource names are kept, bounded by the cache limit.
type imageCache struct {
	mu       sync.Mutex
	entries  map[imageKey]string
	limit    int
	fallback uint64 // counter for unique, uncached resource names
	closed   bool
}

// newImageCache creates a cache holding at most limit distinct images.
// A non-positive limit selects DefaultImageCacheLimit.
func newImageCache(limit int) *imageCache {
	if limit <= 0 {
		limit = DefaultImageCacheLimit
	}
	return &imageCache{entries: make(map[imageKey]string), limit: limit}
}

// keyFor computes the stable content key of img. Images with identical
// pixels, bounds and encoding parameters yield identical keys; any
// difference in alpha, bounds or encoding yields a different key.
func keyFor(img image.Image) (imageKey, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return imageKey{}, errEmptyImage
	}
	key := imageKey{
		minX: int32(b.Min.X),
		minY: int32(b.Min.Y),
		w:    int32(w),
		h:    int32(h),
	}
	h256 := sha256.New()
	h64 := fnv.New64a()
	// Encoding parameters are part of the key, so a future change of
	// image type or encoder options can never merge with older entries.
	var header [16]byte
	binary.LittleEndian.PutUint32(header[0:4], uint32(key.minX))
	binary.LittleEndian.PutUint32(header[4:8], uint32(key.minY))
	binary.LittleEndian.PutUint32(header[8:12], uint32(key.w))
	binary.LittleEndian.PutUint32(header[12:16], uint32(key.h))
	h256.Write([]byte(imageTypePNG))
	h256.Write(header[:])
	h64.Write([]byte(imageTypePNG))
	h64.Write(header[:])
	// Canonical premultiplied 16-bit RGBA per pixel, so the key depends
	// only on the visible pixels, not on the concrete image type.
	var px [8]byte
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			binary.LittleEndian.PutUint16(px[0:2], uint16(r))
			binary.LittleEndian.PutUint16(px[2:4], uint16(g))
			binary.LittleEndian.PutUint16(px[4:6], uint16(bl))
			binary.LittleEndian.PutUint16(px[6:8], uint16(a))
			h256.Write(px[:])
			h64.Write(px[:])
		}
	}
	copy(key.sum[:], h256.Sum(nil))
	key.tag = h64.Sum64()
	return key, nil
}

// register returns the pdf image resource name for img. If the image (or
// one with identical content) was registered before, the cached name is
// returned and data is nil. Otherwise data holds the PNG encoding of img
// which the caller must register with the pdf under the returned name.
//
// Images with empty bounds return errEmptyImage. Oversized images, a full
// cache and a closed cache fall back to unique, uncached names so drawing
// still works and memory stays bounded. A PNG encoding failure is
// returned as an error.
func (c *imageCache) register(img image.Image) (name string, data []byte, err error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return "", nil, errEmptyImage
	}
	// Oversized images bypass the cache entirely: they are not hashed
	// and are registered under unique names.
	cached := int64(w)*int64(h) <= maxCachedPixels
	var key imageKey
	if cached {
		if key, err = keyFor(img); err != nil {
			return "", nil, err
		}
	}
	c.mu.Lock()
	if cached && !c.closed {
		if name, ok := c.entries[key]; ok {
			c.mu.Unlock()
			return name, nil, nil
		}
		if len(c.entries) >= c.limit {
			cached = false
		}
	} else {
		cached = false
	}
	if cached {
		name = "img-" + hex.EncodeToString(key.sum[:])
	} else {
		c.fallback++
		name = fmt.Sprintf("img-u%d", c.fallback)
	}
	c.mu.Unlock()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", nil, fmt.Errorf("draw2dpdf: encoding image as PNG: %w", err)
	}
	if cached {
		// Store the entry only after a successful encoding, so a
		// failed registration never poisons the cache. Concurrent
		// misses for the same key harmlessly encode twice; gofpdf
		// deduplicates registrations by name.
		c.mu.Lock()
		c.entries[key] = name
		c.mu.Unlock()
	}
	return name, buf.Bytes(), nil
}

// close releases the cached resource names. It is idempotent: calling it
// more than once has no further effect. The cache keeps working after
// close, but registers images under unique, uncached names.
func (c *imageCache) close() {
	c.mu.Lock()
	c.entries = make(map[imageKey]string)
	c.closed = true
	c.mu.Unlock()
}
