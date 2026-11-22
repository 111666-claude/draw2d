// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"

	"github.com/llgcode/draw2d/draw2dpdf"
)

// testImage returns a deterministic opaque w x h PNG-friendly image.
// The seed perturbs a single pixel, so different seeds mean different
// content.
func testImage(w, h int, seed byte) *image.RGBA {
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

// buildPDF draws one image per page (pages are added automatically) and
// returns the serialized document.
func buildPDF(t *testing.T, images ...image.Image) []byte {
	t.Helper()
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)
	for i, img := range images {
		if i > 0 {
			pdf.AddPage()
		}
		gc.DrawImage(img)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("generating pdf failed: %v", err)
	}
	if err := pdf.Error(); err != nil {
		t.Fatalf("pdf error: %v", err)
	}
	return buf.Bytes()
}

// countImageResources counts the image XObjects in a serialized pdf.
func countImageResources(doc []byte) int {
	return bytes.Count(doc, []byte("/Subtype /Image"))
}

// extractImageStreams returns the stream data of every image XObject.
func extractImageStreams(t *testing.T, doc []byte) [][]byte {
	t.Helper()
	var streams [][]byte
	rest := doc
	for {
		i := bytes.Index(rest, []byte("/Subtype /Image"))
		if i < 0 {
			return streams
		}
		rest = rest[i:]
		s := bytes.Index(rest, []byte("stream\n"))
		if s < 0 {
			t.Fatal("image object without stream")
		}
		rest = rest[s+len("stream\n"):]
		e := bytes.Index(rest, []byte("\nendstream"))
		if e < 0 {
			t.Fatal("image stream without endstream")
		}
		streams = append(streams, rest[:e])
		rest = rest[e:]
	}
}

// pngIDAT returns the concatenated IDAT chunk data of the PNG encoding
// of img. gofpdf embeds exactly these bytes as the pdf image stream.
func pngIDAT(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("bad PNG signature")
	}
	var idat []byte
	data = data[8:]
	for len(data) >= 8 {
		n := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		typ := string(data[4:8])
		if len(data) < 12+n {
			t.Fatal("truncated PNG chunk")
		}
		if typ == "IDAT" {
			idat = append(idat, data[8:8+n]...)
		}
		data = data[12+n:]
	}
	if len(idat) == 0 {
		t.Fatal("no IDAT chunk found")
	}
	return idat
}

// pixelsEqual reports whether a and b have identical pixels over b's
// bounds.
func pixelsEqual(a, b image.Image) bool {
	bd := b.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			if ar, ag, ab, aa := a.At(x, y).RGBA(); true {
				br, bg, bb, ba := b.At(x, y).RGBA()
				if ar != br || ag != bg || ab != bb || aa != ba {
					return false
				}
			}
		}
	}
	return true
}

func TestRepeatedImageAcrossPagesSharesOneResource(t *testing.T) {
	src := testImage(64, 48, 1)
	const pages = 6
	imgs := make([]image.Image, pages)
	for i := range imgs {
		// a fresh copy per page: deduplication must be content based,
		// not pointer based
		imgs[i] = testImage(64, 48, 1)
	}
	repeated := buildPDF(t, imgs...)
	if n := countImageResources(repeated); n != 1 {
		t.Fatalf("expected 1 image resource for %d pages, got %d", pages, n)
	}

	// The embedded stream must be the encoding of the source image, and
	// decoding it must reproduce the source pixels.
	streams := extractImageStreams(t, repeated)
	if len(streams) != 1 {
		t.Fatalf("expected 1 image stream, got %d", len(streams))
	}
	if !bytes.Equal(streams[0], pngIDAT(t, src)) {
		t.Fatal("embedded image stream differs from source PNG data")
	}

	// File size must not grow linearly with the page count: each added
	// page must cost less than one full image embedding.
	one := buildPDF(t, testImage(64, 48, 1))
	idatLen := len(pngIDAT(t, src))
	if grew := len(repeated) - len(one); grew >= (pages-1)*idatLen {
		t.Errorf("pdf size grows with pages: 1 page = %d bytes, %d pages = %d bytes",
			len(one), pages, len(repeated))
	}

	// Distinct images on every page must not be merged.
	distinct := buildPDF(t,
		testImage(64, 48, 1), testImage(64, 48, 2), testImage(64, 48, 3),
		testImage(64, 48, 4), testImage(64, 48, 5), testImage(64, 48, 6))
	if n := countImageResources(distinct); n != pages {
		t.Fatalf("expected %d image resources for distinct images, got %d", pages, n)
	}
	if len(repeated) >= len(distinct) {
		t.Errorf("deduplicated pdf (%d bytes) not smaller than per-page embedding (%d bytes)",
			len(repeated), len(distinct))
	}
	// The size difference must account for the 5 image embeddings saved.
	if saved := len(distinct) - len(repeated); saved < (pages-1)*idatLen {
		t.Errorf("deduplication saved only %d bytes, expected at least %d",
			saved, (pages-1)*idatLen)
	}
}

func TestSimilarImagesAreNotMerged(t *testing.T) {
	base := testImage(32, 32, 1)

	onePixelOff := testImage(32, 32, 2)

	withAlpha := testImage(32, 32, 1)
	withAlpha.Pix[3] = 0x7f

	shifted := image.NewRGBA(image.Rect(10, 20, 42, 52))
	copy(shifted.Pix, base.Pix)

	doc := buildPDF(t, base, onePixelOff, withAlpha, shifted)
	// base and withAlpha/onePixelOff/shifted differ, so 4 resources;
	// the semi-transparent image adds an SMask image, hence 5.
	if n := countImageResources(doc); n != 5 {
		t.Errorf("expected 5 image resources (4 images + 1 smask), got %d", n)
	}
}

func TestImageResourcePixelsMatchSource(t *testing.T) {
	src := testImage(48, 40, 7)
	doc := buildPDF(t, src, testImage(48, 40, 7))
	streams := extractImageStreams(t, doc)
	if len(streams) != 1 {
		t.Fatalf("expected 1 image stream, got %d", len(streams))
	}
	// Rebuild a decodable PNG from the source IHDR and the embedded
	// stream, then compare pixels.
	var enc bytes.Buffer
	if err := png.Encode(&enc, src); err != nil {
		t.Fatal(err)
	}
	data := enc.Bytes()
	data = data[8:] // skip signature
	var rebuilt []byte
	rebuilt = append(rebuilt, "\x89PNG\r\n\x1a\n"...)
	for len(data) >= 8 {
		n := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		typ := string(data[4:8])
		chunk := data[:12+n]
		switch typ {
		case "IHDR", "PLTE":
			rebuilt = append(rebuilt, chunk...)
		case "IDAT":
			// replaced by the embedded stream below
		case "IEND":
			// appended below
		default:
			rebuilt = append(rebuilt, chunk...)
		}
		data = data[12+n:]
	}
	idat := streams[0]
	var clen [4]byte
	clen[0] = byte(len(idat) >> 24)
	clen[1] = byte(len(idat) >> 16)
	clen[2] = byte(len(idat) >> 8)
	clen[3] = byte(len(idat))
	rebuilt = append(rebuilt, clen[:]...)
	rebuilt = append(rebuilt, "IDAT"...)
	rebuilt = append(rebuilt, idat...)
	crc := pngCRC(append([]byte("IDAT"), idat...))
	rebuilt = append(rebuilt, crc[:]...)
	rebuilt = append(rebuilt, 0, 0, 0, 0)
	rebuilt = append(rebuilt, "IEND"...)
	crc = pngCRC([]byte("IEND"))
	rebuilt = append(rebuilt, crc[:]...)

	decoded, err := png.Decode(bytes.NewReader(rebuilt))
	if err != nil {
		t.Fatalf("decoding embedded image stream failed: %v", err)
	}
	if !pixelsEqual(decoded, src) {
		t.Error("pixels of embedded image resource differ from source image")
	}
}

// pngCRC computes the PNG chunk CRC (IEEE, over type+data).
func pngCRC(b []byte) [4]byte {
	var crc uint32 = 0xffffffff
	for _, c := range b {
		crc ^= uint32(c)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xedb88320
			} else {
				crc >>= 1
			}
		}
	}
	crc ^= 0xffffffff
	return [4]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)}
}

func TestDrawImageAfterCacheClose(t *testing.T) {
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)
	gc.DrawImage(testImage(32, 32, 1))
	// repeated close must be safe and must not corrupt the resource table
	gc.CloseImageCache()
	gc.CloseImageCache()
	pdf.AddPage()
	// drawing still works after close, but without deduplication
	gc.DrawImage(testImage(32, 32, 1))
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("generating pdf failed: %v", err)
	}
	if err := pdf.Error(); err != nil {
		t.Fatalf("pdf error: %v", err)
	}
	if n := countImageResources(buf.Bytes()); n != 2 {
		t.Errorf("expected 2 image resources after cache close, got %d", n)
	}
}

func TestDrawImageConcurrent(t *testing.T) {
	pdf := draw2dpdf.NewPdf("L", "mm", "A4")
	gc := draw2dpdf.NewGraphicContext(pdf)
	const goroutines = 8
	const draws = 20
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			img := testImage(16, 16, 1)
			for i := 0; i < draws; i++ {
				gc.DrawImage(img)
			}
		}()
	}
	wg.Wait()
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("generating pdf failed: %v", err)
	}
	if n := countImageResources(buf.Bytes()); n != 1 {
		t.Errorf("expected 1 image resource after concurrent draws, got %d", n)
	}
}
