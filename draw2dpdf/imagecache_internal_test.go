// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf

import (
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"
)

// rgbaImage returns a deterministic opaque w x h image at (0,0).
// The seed byte perturbs a single pixel, so different seeds yield
// different content.
func rgbaImage(w, h int, seed byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x*7 + y*3),
				G: uint8(x*2 + y*11),
				B: uint8(x*5 + y*13),
				A: 0xff,
			})
		}
	}
	img.Pix[0] ^= seed
	return img
}

func TestKeyForStableForSameContent(t *testing.T) {
	a := rgbaImage(16, 8, 1)
	b := rgbaImage(16, 8, 1)
	ka, err := keyFor(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := keyFor(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka != kb {
		t.Error("identical pixel content produced different keys")
	}
}

func TestKeyForDistinguishesContent(t *testing.T) {
	base := rgbaImage(16, 8, 1)
	kb, err := keyFor(base)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]image.Image{
		"different pixel": rgbaImage(16, 8, 2),
		"different size":  rgbaImage(16, 9, 1),
		"different alpha": func() image.Image {
			img := rgbaImage(16, 8, 1)
			img.Pix[3] = 0x7f // alpha of first pixel
			return img
		}(),
		"non-zero bounds": func() image.Image {
			src := rgbaImage(16, 8, 1)
			img := image.NewRGBA(image.Rect(3, 5, 19, 13))
			copy(img.Pix, src.Pix)
			return img
		}(),
	}
	for name, img := range cases {
		k, err := keyFor(img)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if k == kb {
			t.Errorf("%s: key must differ from base image", name)
		}
	}
}

func TestKeyForEmptyImage(t *testing.T) {
	for _, r := range []image.Rectangle{
		image.Rect(0, 0, 0, 0),
		image.Rect(2, 2, 2, 10),
		image.Rect(0, 0, 10, 0),
	} {
		if _, err := keyFor(image.NewRGBA(r)); !errors.Is(err, errEmptyImage) {
			t.Errorf("bounds %v: expected errEmptyImage, got %v", r, err)
		}
	}
}

func TestRegisterReusesSameImage(t *testing.T) {
	c := newImageCache(0)
	img := rgbaImage(16, 8, 1)
	name1, data1, err := c.register(img)
	if err != nil {
		t.Fatal(err)
	}
	if len(data1) == 0 {
		t.Fatal("first registration must return PNG data")
	}
	name2, data2, err := c.register(rgbaImage(16, 8, 1))
	if err != nil {
		t.Fatal(err)
	}
	if name1 != name2 {
		t.Errorf("same content registered under different names %q, %q", name1, name2)
	}
	if data2 != nil {
		t.Error("second registration must not re-encode")
	}
	if len(c.entries) != 1 {
		t.Errorf("expected 1 cache entry, got %d", len(c.entries))
	}
}

func TestRegisterDistinctImages(t *testing.T) {
	c := newImageCache(0)
	n1, _, _ := c.register(rgbaImage(16, 8, 1))
	n2, _, _ := c.register(rgbaImage(16, 8, 2))
	if n1 == n2 {
		t.Error("different images must get different resource names")
	}
	if len(c.entries) != 2 {
		t.Errorf("expected 2 cache entries, got %d", len(c.entries))
	}
}

func TestRegisterCacheLimitFallback(t *testing.T) {
	c := newImageCache(2)
	c.register(rgbaImage(16, 8, 1))
	c.register(rgbaImage(16, 8, 2))
	// cache is full: a third distinct image falls back to a unique,
	// uncached name and must not grow the table
	name, data, err := c.register(rgbaImage(16, 8, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("fallback registration must still return PNG data")
	}
	if len(c.entries) != 2 {
		t.Errorf("cache grew past its limit: %d entries", len(c.entries))
	}
	// re-registering the overflow image does not reuse the name
	name2, data2, _ := c.register(rgbaImage(16, 8, 3))
	if name2 == name || data2 == nil {
		t.Error("overflow image must not be cached")
	}
}

func TestRegisterOversizeFallback(t *testing.T) {
	defer func(p int64) { maxCachedPixels = p }(maxCachedPixels)
	maxCachedPixels = 100
	c := newImageCache(0)
	big := rgbaImage(20, 20, 1) // 400 pixels > 100
	name1, data1, err := c.register(big)
	if err != nil {
		t.Fatal(err)
	}
	if len(data1) == 0 {
		t.Fatal("oversized image must still be encoded")
	}
	name2, _, _ := c.register(rgbaImage(20, 20, 1))
	if name1 == name2 {
		t.Error("oversized image must not be cached")
	}
	if len(c.entries) != 0 {
		t.Errorf("oversized image entered the cache: %d entries", len(c.entries))
	}
}

// hugeImage reports bounds that exceed the PNG format limit, so
// png.Encode fails on it.
type hugeImage struct{}

func (hugeImage) ColorModel() color.Model { return color.RGBAModel }
func (hugeImage) Bounds() image.Rectangle { return image.Rect(0, 0, 1<<32, 1) }
func (hugeImage) At(x, y int) color.Color { return color.RGBA{A: 0xff} }

func TestRegisterEncodeFailure(t *testing.T) {
	c := newImageCache(0)
	_, _, err := c.register(hugeImage{})
	if err == nil {
		t.Fatal("expected encoding error for image beyond PNG size limit")
	}
	if len(c.entries) != 0 {
		t.Error("failed registration must not poison the cache")
	}
}

func TestRegisterEmptyImage(t *testing.T) {
	c := newImageCache(0)
	if _, _, err := c.register(image.NewRGBA(image.Rectangle{})); !errors.Is(err, errEmptyImage) {
		t.Fatalf("expected errEmptyImage, got %v", err)
	}
}

func TestCloseIdempotentAndFallback(t *testing.T) {
	c := newImageCache(0)
	c.register(rgbaImage(16, 8, 1))
	c.close()
	c.close() // repeated close must not panic or corrupt the table
	if len(c.entries) != 0 {
		t.Error("close must release the resource table")
	}
	// drawing still works after close, but without deduplication
	n1, d1, err := c.register(rgbaImage(16, 8, 1))
	if err != nil || len(d1) == 0 {
		t.Fatalf("register after close failed: %v", err)
	}
	n2, d2, _ := c.register(rgbaImage(16, 8, 1))
	if n1 == n2 || d2 == nil {
		t.Error("closed cache must fall back to unique, uncached names")
	}
	if len(c.entries) != 0 {
		t.Error("closed cache must not collect entries")
	}
}

func TestRegisterConcurrent(t *testing.T) {
	c := newImageCache(0)
	const goroutines = 8
	const draws = 20
	names := make([]string, goroutines*draws)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			img := rgbaImage(16, 8, 1) // same content in every goroutine
			for i := 0; i < draws; i++ {
				name, _, err := c.register(img)
				if err != nil {
					t.Error(err)
					return
				}
				names[g*draws+i] = name
			}
		}(g)
	}
	wg.Wait()
	for i, name := range names {
		if name != names[0] {
			t.Errorf("concurrent registration diverged: names[0]=%q names[%d]=%q", names[0], i, name)
		}
	}
	if len(c.entries) != 1 {
		t.Errorf("expected 1 cache entry, got %d", len(c.entries))
	}
}
