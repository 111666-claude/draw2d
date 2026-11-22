// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dimg_test

import (
	"errors"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"

	xdraw "golang.org/x/image/draw"
)

// refComposite computes the Porter-Duff reference formula on premultiplied
// channels in [0,1] and returns the premultiplied result.
func refComposite(sr, sg, sb, sa, dr, dg, db, da float64, op draw2d.CompositeOp) (r, g, b, a float64) {
	var fs, fd float64
	switch op {
	case draw2d.CompositeOpOver:
		fs, fd = 1, 1-sa
	case draw2d.CompositeOpSrc:
		fs, fd = 1, 0
	case draw2d.CompositeOpIn:
		fs, fd = da, 0
	case draw2d.CompositeOpOut:
		fs, fd = 1-da, 0
	case draw2d.CompositeOpAtop:
		fs, fd = da, 1-sa
	case draw2d.CompositeOpXor:
		fs, fd = 1-da, 1-sa
	}
	return sr*fs + dr*fd, sg*fs + dg*fd, sb*fs + db*fd, sa*fs + da*fd
}

func premul(c color.RGBA) (r, g, b, a float64) {
	return float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255
}

var allOps = []draw2d.CompositeOp{
	draw2d.CompositeOpOver,
	draw2d.CompositeOpSrc,
	draw2d.CompositeOpIn,
	draw2d.CompositeOpOut,
	draw2d.CompositeOpAtop,
	draw2d.CompositeOpXor,
}

// checkPixel compares a destination pixel against the reference formula
// with a tolerance of 1/255 per channel.
func checkPixel(t *testing.T, dst *image.RGBA, x, y int, sc, dc color.RGBA, op draw2d.CompositeOp) {
	t.Helper()
	sr, sg, sb, sa := premul(sc)
	dr, dg, db, da := premul(dc)
	er, eg, eb, ea := refComposite(sr, sg, sb, sa, dr, dg, db, da, op)
	got := dst.RGBAAt(x, y)
	want := []struct{ name string; got, want uint8 }{
		{"r", got.R, uint8(math.Round(er * 255))},
		{"g", got.G, uint8(math.Round(eg * 255))},
		{"b", got.B, uint8(math.Round(eb * 255))},
		{"a", got.A, uint8(math.Round(ea * 255))},
	}
	for _, w := range want {
		if d := int(w.got) - int(w.want); d < -1 || d > 1 {
			t.Errorf("op %s pixel (%d,%d) channel %s: got %d, want %d (src=%v dst=%v)",
				op, x, y, w.name, w.got, w.want, sc, dc)
		}
	}
}

var (
	semiRed   = color.RGBA{R: 128, A: 128} // premultiplied: opaque red at alpha ~0.5
	semiGreen = color.RGBA{G: 128, A: 128} // premultiplied: opaque green at alpha ~0.5
)

func fill(img *image.RGBA, c color.RGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// TestCompositeOpsPerPixel draws a semi-transparent green square with
// non-zero bounds over a semi-transparent red canvas and checks every pixel
// against the Porter-Duff reference formulas.
func TestCompositeOpsPerPixel(t *testing.T) {
	for _, op := range allOps {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		fill(dst, semiRed)
		// non-zero source bounds: the square lands at (2,2)-(6,6)
		src := image.NewRGBA(image.Rect(2, 2, 6, 6))
		fill(src, semiGreen)

		if err := draw2dimg.DrawImageComposite(src, dst, draw2d.NewIdentityMatrix(), op, draw2dimg.LinearFilter); err != nil {
			t.Fatalf("op %s: %v", op, err)
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if x >= 2 && x < 6 && y >= 2 && y < 6 {
					checkPixel(t, dst, x, y, semiGreen, semiRed, op)
				} else if got := dst.RGBAAt(x, y); got != semiRed {
					t.Errorf("op %s: pixel (%d,%d) outside source bounds changed to %v", op, x, y, got)
				}
			}
		}
	}
}

// TestCompositeTransparent checks the operators where source and/or
// destination are fully transparent.
func TestCompositeTransparent(t *testing.T) {
	transparent := color.RGBA{}
	cases := []struct {
		name   string
		s, d   color.RGBA
	}{
		{"src-over-transparent-dst", semiGreen, transparent},
		{"transparent-src-over-dst", transparent, semiRed},
		{"both-transparent", transparent, transparent},
	}
	for _, tc := range cases {
		for _, op := range allOps {
			dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
			fill(dst, tc.d)
			src := image.NewRGBA(image.Rect(0, 0, 2, 2))
			fill(src, tc.s)
			if err := draw2dimg.DrawImageComposite(src, dst, draw2d.NewIdentityMatrix(), op, draw2dimg.LinearFilter); err != nil {
				t.Fatalf("%s op %s: %v", tc.name, op, err)
			}
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					checkPixel(t, dst, x, y, tc.s, tc.d, op)
				}
			}
		}
	}
}

// TestCompositeTransform checks that the current transformation matrix is
// honored: a 2x2 source scaled by 2 and translated by (3,1) covers (3,1)-(7,5).
func TestCompositeTransform(t *testing.T) {
	tr := draw2d.Matrix{2, 0, 0, 2, 3, 1}
	for _, op := range allOps {
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		fill(dst, semiRed)
		src := image.NewRGBA(image.Rect(0, 0, 2, 2))
		fill(src, semiGreen)
		if err := draw2dimg.DrawImageComposite(src, dst, tr, op, draw2dimg.LinearFilter); err != nil {
			t.Fatalf("op %s: %v", op, err)
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if x >= 3 && x < 7 && y >= 1 && y < 5 {
					checkPixel(t, dst, x, y, semiGreen, semiRed, op)
				} else if got := dst.RGBAAt(x, y); got != semiRed {
					t.Errorf("op %s: pixel (%d,%d) outside transformed bounds changed to %v", op, x, y, got)
				}
			}
		}
	}
}

// TestCompositeDefaultOverUnchanged ensures the default operation produces
// exactly the same pixels as the historical draw.Over behavior.
func TestCompositeDefaultOverUnchanged(t *testing.T) {
	src := image.NewRGBA(image.Rect(1, 1, 5, 5))
	fill(src, semiGreen)

	legacy := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fill(legacy, semiRed)
	gcLegacy := draw2dimg.NewGraphicContext(legacy)
	draw2dimg.DrawImage(src, legacy, gcLegacy.GetMatrixTransform(), xdraw.Over, draw2dimg.BilinearFilter)

	def := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fill(def, semiRed)
	gc := draw2dimg.NewGraphicContext(def)
	if got := gc.GetCompositeOp(); got != draw2d.CompositeOpOver {
		t.Fatalf("default composite op = %s, want over", got)
	}
	gc.DrawImage(src)

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if got, want := def.RGBAAt(x, y), legacy.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d): default op gives %v, legacy over gives %v", x, y, got, want)
			}
		}
	}
}

// TestCompositeSaveRestore checks that the compositing operation is part of
// the Save/Restore state and that invalid operations are rejected.
func TestCompositeSaveRestore(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	gc := draw2dimg.NewGraphicContext(dst)
	if gc.GetCompositeOp() != draw2d.CompositeOpOver {
		t.Fatal("initial op is not over")
	}
	gc.SetCompositeOp(draw2d.CompositeOp(99)) // invalid: ignored
	if gc.GetCompositeOp() != draw2d.CompositeOpOver {
		t.Fatal("invalid op changed the current operation")
	}
	gc.Save()
	gc.SetCompositeOp(draw2d.CompositeOpXor)
	if gc.GetCompositeOp() != draw2d.CompositeOpXor {
		t.Fatal("op not set inside Save")
	}
	gc.Restore()
	if gc.GetCompositeOp() != draw2d.CompositeOpOver {
		t.Fatal("op not restored to over after Restore")
	}
}

// TestCompositeGCState checks that GraphicContext.DrawImage honors the
// operation set with SetCompositeOp.
func TestCompositeGCState(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fill(dst, semiRed)
	gc := draw2dimg.NewGraphicContext(dst)
	gc.SetCompositeOp(draw2d.CompositeOpIn)
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fill(src, semiGreen)
	gc.DrawImage(src)
	checkPixel(t, dst, 0, 0, semiGreen, semiRed, draw2d.CompositeOpIn)
	if got := dst.RGBAAt(3, 3); got != semiRed {
		t.Errorf("pixel outside source changed to %v", got)
	}
}

type hugeImage struct{ r image.Rectangle }

func (h hugeImage) ColorModel() color.Model     { return color.RGBAModel }
func (h hugeImage) Bounds() image.Rectangle     { return h.r }
func (h hugeImage) At(x, y int) color.Color     { return color.RGBA{} }
func (h hugeImage) Set(x, y int, c color.Color) {}

// TestCompositeErrors checks the explicit error paths and that neither the
// destination nor the source is modified on failure.
func TestCompositeErrors(t *testing.T) {
	newDst := func() *image.RGBA {
		dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
		fill(dst, semiRed)
		return dst
	}
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fill(src, semiGreen)
	id := draw2d.NewIdentityMatrix()

	checkDstUnchanged := func(t *testing.T, dst *image.RGBA) {
		t.Helper()
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				if got := dst.RGBAAt(x, y); got != semiRed {
					t.Fatalf("destination modified at (%d,%d): %v", x, y, got)
				}
			}
		}
	}

	t.Run("unknown-op", func(t *testing.T) {
		dst := newDst()
		err := draw2dimg.DrawImageComposite(src, dst, id, draw2d.CompositeOp(42), draw2dimg.LinearFilter)
		if !errors.Is(err, draw2d.ErrUnsupportedCompositeOp) {
			t.Fatalf("got %v, want ErrUnsupportedCompositeOp", err)
		}
		checkDstUnchanged(t, dst)
	})
	t.Run("nil-source", func(t *testing.T) {
		dst := newDst()
		if err := draw2dimg.DrawImageComposite(nil, dst, id, draw2d.CompositeOpOver, draw2dimg.LinearFilter); !errors.Is(err, draw2dimg.ErrEmptyImage) {
			t.Fatalf("got %v, want ErrEmptyImage", err)
		}
		checkDstUnchanged(t, dst)
	})
	t.Run("empty-source", func(t *testing.T) {
		dst := newDst()
		empty := image.NewRGBA(image.Rectangle{})
		if err := draw2dimg.DrawImageComposite(empty, dst, id, draw2d.CompositeOpXor, draw2dimg.LinearFilter); !errors.Is(err, draw2dimg.ErrEmptyImage) {
			t.Fatalf("got %v, want ErrEmptyImage", err)
		}
		checkDstUnchanged(t, dst)
	})
	t.Run("non-finite-transform", func(t *testing.T) {
		for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			dst := newDst()
			tr := draw2d.NewIdentityMatrix()
			tr[4] = v
			if err := draw2dimg.DrawImageComposite(src, dst, tr, draw2d.CompositeOpOver, draw2dimg.LinearFilter); !errors.Is(err, draw2dimg.ErrNonFiniteTransform) {
				t.Fatalf("tr=%v: got %v, want ErrNonFiniteTransform", v, err)
			}
			checkDstUnchanged(t, dst)
		}
	})
	t.Run("huge-destination", func(t *testing.T) {
		huge := hugeImage{image.Rect(0, 0, 20000, 20000)}
		if err := draw2dimg.DrawImageComposite(src, huge, id, draw2d.CompositeOpXor, draw2dimg.LinearFilter); !errors.Is(err, draw2dimg.ErrImageTooLarge) {
			t.Fatalf("got %v, want ErrImageTooLarge", err)
		}
	})
	t.Run("gc-draw-image-with-op", func(t *testing.T) {
		dst := newDst()
		gc := draw2dimg.NewGraphicContext(dst)
		if err := gc.DrawImageWithOp(src, draw2d.CompositeOp(42)); !errors.Is(err, draw2d.ErrUnsupportedCompositeOp) {
			t.Fatalf("got %v, want ErrUnsupportedCompositeOp", err)
		}
		checkDstUnchanged(t, dst)
		if err := gc.DrawImageWithOp(src, draw2d.CompositeOpXor); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		checkPixel(t, dst, 0, 0, semiGreen, semiRed, draw2d.CompositeOpXor)
	})
}

// TestCompositeSourceNotModified ensures compositing never writes to the
// source image.
func TestCompositeSourceNotModified(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fill(src, semiGreen)
	before := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copy(before.Pix, src.Pix)
	for _, op := range allOps {
		dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
		fill(dst, semiRed)
		if err := draw2dimg.DrawImageComposite(src, dst, draw2d.NewIdentityMatrix(), op, draw2dimg.LinearFilter); err != nil {
			t.Fatal(err)
		}
	}
	for i, v := range src.Pix {
		if v != before.Pix[i] {
			t.Fatalf("source modified at byte %d", i)
		}
	}
}

// TestCompositeConcurrentReadOnlySource draws the same source image from
// many goroutines (run with -race) and checks each result against the
// single-threaded reference.
func TestCompositeConcurrentReadOnlySource(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fill(src, semiGreen)

	reference := image.NewRGBA(image.Rect(0, 0, 8, 8))
	fill(reference, semiRed)
	if err := draw2dimg.DrawImageComposite(src, reference, draw2d.NewIdentityMatrix(), draw2d.CompositeOpXor, draw2dimg.LinearFilter); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
			fill(dst, semiRed)
			gc := draw2dimg.NewGraphicContext(dst)
			gc.SetCompositeOp(draw2d.CompositeOpXor)
			gc.DrawImage(src)
			for j, v := range dst.Pix {
				if v != reference.Pix[j] {
					errs <- errors.New("concurrent result differs from reference")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
