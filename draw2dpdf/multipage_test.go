// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"bytes"
	"image"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dpdf"
)

// newTestDocument builds a multi page document in points so that
// coordinates map 1:1 to PDF user space.
func newTestDocument(t *testing.T) *draw2dpdf.PageDocument {
	t.Helper()
	doc, err := draw2dpdf.NewMultiPagePdfWithSize("P", "pt", 200, 300)
	if err != nil {
		t.Fatalf("NewMultiPagePdfWithSize failed: %v", err)
	}
	// Select a PDF core font so text output does not depend on external
	// font definition files.
	doc.Pdf().SetFont("helvetica", "", 12)
	return doc
}

// imageRefs returns the image XObject names referenced in a content
// stream (the "/In Do" operators).
func imageRefs(content string) []string {
	re := regexp.MustCompile(`/I\d+ Do`)
	return re.FindAllString(content, -1)
}

func renderDoc(t *testing.T, doc *draw2dpdf.PageDocument) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatalf("Output failed: %v", err)
	}
	return buf.Bytes()
}

// TestMultiPageThreePageDocument builds a three page document using two
// page sizes and verifies page count, text/path positions, PDF content
// order and resource references.
func TestMultiPageThreePageDocument(t *testing.T) {
	doc := newTestDocument(t)
	gc := doc.GraphicContext()

	// Persistent state configured before any page exists must survive
	// every page transition.
	stroke := color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}
	fill := color.RGBA{R: 0x44, G: 0x55, B: 0x66, A: 0xff}
	gc.SetStrokeColor(stroke)
	gc.SetFillColor(fill)
	gc.SetLineWidth(3)
	if err := doc.SetDPI(144); err != nil {
		t.Fatalf("SetDPI failed: %v", err)
	}
	if gc.GetDPI() != 144 {
		t.Fatalf("DPI = %d, want 144", gc.GetDPI())
	}

	// Page 1: default size (200x300 pt), text and a path.
	if err := doc.AddPage(); err != nil {
		t.Fatalf("AddPage 1 failed: %v", err)
	}
	gc.FillStringAt("page1", 10, 40)
	gc.MoveTo(20, 60)
	gc.LineTo(120, 90)
	gc.Stroke()
	// A transformation scoped to page 1.
	gc.Save()
	gc.Translate(50, 50)
	gc.MoveTo(0, 0)
	gc.LineTo(10, 10)
	gc.Stroke()
	gc.Restore()
	// An image only referenced by page 1.
	gc.DrawImage(image.NewRGBA(image.Rect(0, 0, 4, 4)))

	// Page 2: a different size and landscape orientation.
	if err := doc.AddPageWithSize("L", draw2dpdf.PageSize{Width: 400, Height: 250}); err != nil {
		t.Fatalf("AddPage 2 failed: %v", err)
	}
	if w, h := doc.Pdf().GetPageSize(); w != 400 || h != 250 {
		t.Fatalf("page 2 active size = %v x %v, want 400 x 250", w, h)
	}
	if gc.GetDPI() != 144 {
		t.Fatalf("DPI lost across page: got %d, want 144", gc.GetDPI())
	}
	// The path left open on page 1 must not leak onto page 2.
	if !gc.IsEmpty() {
		t.Fatal("current path was not cleared when starting a new page")
	}
	// Identity transformation: the translate from page 1 must be gone,
	// so a line at (30, 30) lands at (30, 30) in PDF space.
	gc.MoveTo(30, 30)
	gc.LineTo(80, 30)
	gc.Stroke()
	gc.FillStringAt("page2", 70, 100)
	// State retained: same colors and line width.
	if got := gc.Current.StrokeColor; got != stroke {
		t.Fatalf("stroke color not retained: %v", got)
	}
	if got := gc.Current.FillColor; got != fill {
		t.Fatalf("fill color not retained: %v", got)
	}
	if got := gc.GetFontSize(); got != 10 {
		t.Fatalf("font size changed unexpectedly: %v", got)
	}

	// Page 3: default size again, with a second image and another text.
	if err := doc.AddPage(); err != nil {
		t.Fatalf("AddPage 3 failed: %v", err)
	}
	gc.FillStringAt("page3", 5, 20)
	gc.DrawImage(image.NewRGBA(image.Rect(0, 0, 6, 6)))
	gc.MoveTo(1, 2)
	gc.LineTo(9, 8)
	gc.Stroke()

	if doc.PageCount() != 3 {
		t.Fatalf("PageCount = %d, want 3", doc.PageCount())
	}

	pages, err := parsePdfPages(renderDoc(t, doc))
	if err != nil {
		t.Fatalf("parsePdfPages failed: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("parsed %d pages, want 3", len(pages))
	}

	// Page sizes: page 1 and 3 use the 200x300 default, page 2 the
	// landscape custom size with the visible 400x250 dimensions.
	// gofpdf only writes an explicit MediaBox for page 2; the others
	// inherit the Pages root.
	if !floatsEqual(pages[0].mediaBox, []float64{0, 0, 200, 300}) {
		t.Errorf("page 1 MediaBox = %v, want [0 0 200 300]", pages[0].mediaBox)
	}
	if !floatsEqual(pages[1].mediaBox, []float64{0, 0, 400, 250}) {
		t.Errorf("page 2 MediaBox = %v, want [0 0 400 250]", pages[1].mediaBox)
	}
	if !floatsEqual(pages[2].mediaBox, []float64{0, 0, 200, 300}) {
		t.Errorf("page 3 MediaBox = %v, want [0 0 200 300]", pages[2].mediaBox)
	}

	// Content order: the text of every page appears before the text of
	// the next page and each page contains only its own strings.
	for i, want := range []string{"page1", "page2", "page3"} {
		if !strings.Contains(pages[i].content, "("+want+") Tj") {
			t.Errorf("page %d content missing text %q: %q", i+1, want, pages[i].content)
		}
		for j, other := range []string{"page1", "page2", "page3"} {
			if j != i && strings.Contains(pages[i].content, "("+other+") Tj") {
				t.Errorf("page %d unexpectedly contains text of page %d", i+1, j+1)
			}
		}
	}
	// The parser follows the Pages Kids array: distinct streams confirm
	// the pages are serialized in insertion order.
	if pages[0].content == pages[1].content || pages[1].content == pages[2].content {
		t.Fatal("page content streams are not distinct")
	}

	// Path positions: page 1 contains the line starting at (20,60) and
	// the transformed line ("cm" operator from Save/Translate). Page 2
	// must start with an identity transformation (no "cm" operator) and
	// keep its line at x=30 in PDF space.
	if !strings.Contains(pages[0].content, "20.00 240.00 m") {
		t.Errorf("page 1 missing line at (20,60): %q", pages[0].content)
	}
	if !strings.Contains(pages[0].content, " cm") {
		t.Errorf("page 1 missing the scoped transformation: %q", pages[0].content)
	}
	if strings.Contains(pages[1].content, " cm") {
		t.Errorf("page 2 inherited a transformation from page 1: %q", pages[1].content)
	}
	// PDF y is measured from the bottom. On page 2 the active page is
	// 400 units wide and 250 high, so the line at user point (30, 30) is
	// emitted as (30, 220).
	if !strings.Contains(pages[1].content, "30.00 220.00 m") {
		t.Errorf("page 2 missing line at (30,30): %q", pages[1].content)
	}

	// Image resource references stay on the page that drew them. The
	// image names are document global counters, so capture the name each
	// DrawImage produced and assert per page scoping.
	page1Images := imageRefs(pages[0].content)
	page2Images := imageRefs(pages[1].content)
	page3Images := imageRefs(pages[2].content)
	if len(page1Images) != 1 {
		t.Fatalf("page 1 should reference one image, got %v: %q", page1Images, pages[0].content)
	}
	if len(page2Images) != 0 {
		t.Errorf("page 2 should not reference any image, got %v: %q", page2Images, pages[1].content)
	}
	if len(page3Images) != 1 {
		t.Fatalf("page 3 should reference one image, got %v: %q", page3Images, pages[2].content)
	}
	if page1Images[0] == page3Images[0] {
		t.Errorf("the two pages reference the same image resource %q", page1Images[0])
	}
	if strings.Contains(pages[2].content, page1Images[0]) {
		t.Errorf("page 3 should not reference the page 1 image: %q", pages[2].content)
	}

	// Save/Restore balance: no unclosed q operator remains anywhere.
	for i, p := range pages {
		if countOccurrences(p.content, "q") != countOccurrences(p.content, "Q") {
			t.Errorf("page %d has unbalanced q/Q: %q", i+1, p.content)
		}
	}
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		d := a[i] - b[i]
		if d < -0.5 || d > 0.5 {
			return false
		}
	}
	return true
}

// TestMultiPageStateRetention verifies font, colors, line style and DPI
// survive page transitions while path and transformation do not.
func TestMultiPageStateRetention(t *testing.T) {
	doc := newTestDocument(t)
	gc := doc.GraphicContext()
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc.SetStrokeColor(color.RGBA{10, 20, 30, 255})
	gc.SetFillColor(color.RGBA{40, 50, 60, 255})
	gc.SetLineWidth(7.5)
	gc.SetLineCap(draw2d.SquareCap)
	gc.SetLineJoin(draw2d.BevelJoin)
	gc.SetLineDash([]float64{2, 3}, 1)
	gc.SetFontSize(18)
	if err := doc.SetDPI(96); err != nil {
		t.Fatal(err)
	}
	gc.Save()
	gc.Translate(100, 100)
	gc.Rotate(0.5)
	gc.Scale(2, 3)
	gc.MoveTo(1, 1)
	gc.Restore()

	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	c := gc.Current
	if c.LineWidth != 7.5 {
		t.Errorf("line width lost: %v", c.LineWidth)
	}
	if c.Cap != draw2d.SquareCap {
		t.Errorf("line cap lost: %v", c.Cap)
	}
	if c.Join != draw2d.BevelJoin {
		t.Errorf("line join lost: %v", c.Join)
	}
	if c.FontSize != 18 {
		t.Errorf("font size lost: %v", c.FontSize)
	}
	if gc.GetDPI() != 96 {
		t.Errorf("DPI lost: %v", gc.GetDPI())
	}
	if !gc.IsEmpty() {
		t.Error("path was not reset on new page")
	}
	if !gc.GetMatrixTransform().IsIdentity() {
		t.Errorf("transformation was not reset: %v", gc.GetMatrixTransform())
	}
	if gc.Current.Previous != nil {
		t.Error("save stack was not reset on new page")
	}
}

// TestMultiPageEmptyPages covers an entirely empty document and empty
// pages between drawn pages.
func TestMultiPageEmptyPages(t *testing.T) {
	// No page at all: Close produces a valid single empty page.
	doc := newTestDocument(t)
	if err := doc.Close(); err != nil {
		t.Fatalf("Close without pages failed: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount = %d, want 1", doc.PageCount())
	}
	pages, err := parsePdfPages(renderDoc(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("empty document rendered %d pages", len(pages))
	}

	// Empty pages between drawn pages are kept and ordered.
	doc2 := newTestDocument(t)
	gc2 := doc2.GraphicContext()
	if err := doc2.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc2.FillStringAt("first", 5, 5)
	if err := doc2.AddPage(); err != nil { // empty page
		t.Fatal(err)
	}
	if err := doc2.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc2.FillStringAt("third", 5, 5)
	pages, err = parsePdfPages(renderDoc(t, doc2))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	if strings.Contains(pages[1].content, "Tj") {
		t.Errorf("middle page should be empty, got %q", pages[1].content)
	}
	if !strings.Contains(pages[0].content, "(first) Tj") ||
		!strings.Contains(pages[2].content, "(third) Tj") {
		t.Error("drawn pages lost around empty middle page")
	}
}

// TestMultiPageFontRetention verifies that a custom font selected on one
// page keeps working on the next page without calling SetFontData again.
func TestMultiPageFontRetention(t *testing.T) {
	draw2d.SetFontFolder("../resource/font")
	doc := newTestDocument(t)
	gc := doc.GraphicContext()
	fontData := draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleBold | draw2d.FontStyleItalic}
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc.SetFontData(fontData)
	gc.SetFontSize(14)
	gc.FillStringAt("custom-font", 10, 20)
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	if got := gc.GetFontData(); got != fontData {
		t.Errorf("font data not retained: got %+v, want %+v", got, fontData)
	}
	if got := gc.GetFontSize(); got != 14 {
		t.Errorf("font size not retained: got %v, want 14", got)
	}
	// Drawing text must succeed on the second page with the same font.
	gc.FillStringAt("second-page", 10, 20)
	pages, err := parsePdfPages(renderDoc(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pages[0].content, "(custom-font) Tj") {
		t.Errorf("page 1 missing custom font text: %q", pages[0].content)
	}
	if !strings.Contains(pages[1].content, "(second-page) Tj") {
		t.Errorf("page 2 missing text drawn with retained font: %q", pages[1].content)
	}
}

// TestMultiPageDpiRetention verifies the configured DPI and its derived
// font scale survive page transitions.
func TestMultiPageDpiRetention(t *testing.T) {
	doc := newTestDocument(t)
	gc := doc.GraphicContext()
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetDPI(150); err != nil {
		t.Fatal(err)
	}
	scaleBefore := gc.Current.Scale
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	if gc.GetDPI() != 150 {
		t.Errorf("DPI not retained: %d", gc.GetDPI())
	}
	if gc.Current.Scale != scaleBefore {
		t.Errorf("font scale not retained across pages: %v vs %v", gc.Current.Scale, scaleBefore)
	}
}

// TestMultiPageUnits verifies page sizes and the oversized page limit
// work in a non point unit (mm).
func TestMultiPageUnits(t *testing.T) {
	// 210 x 297 mm is A4 in points (595.28 x 841.89).
	doc, err := draw2dpdf.NewMultiPagePdfWithSize("P", "mm", 210, 297)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc := doc.GraphicContext()
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	pages, err := parsePdfPages(renderDoc(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if !floatsEqual(pages[0].mediaBox, []float64{0, 0, 595.28, 841.89}) {
		t.Errorf("A4 mm MediaBox = %v", pages[0].mediaBox)
	}
	// A 10 mm line starts at x = 10mm = 28.35 pt.
	if !strings.Contains(pages[0].content, "28.35 ") {
		t.Errorf("mm to point conversion missing: %q", pages[0].content)
	}

	// 600 mm is about 1700 points, well within the limit; 600 cm is not.
	big, err := draw2dpdf.NewMultiPagePdfWithSize("P", "mm", 600, 100)
	if err != nil {
		t.Errorf("600mm page should be valid, got %v", err)
	}
	if err := big.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestMultiPageCloseAndErrors covers repeated Close, illegal sizes,
// negative DPI, unbalanced Save and oversized pages.
func TestMultiPageCloseAndErrors(t *testing.T) {
	// Repeated Close and AddPage after Close.
	doc := newTestDocument(t)
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != draw2dpdf.ErrClosed {
		t.Errorf("second Close = %v, want ErrClosed", err)
	}
	if err := doc.AddPage(); err != draw2dpdf.ErrClosed {
		t.Errorf("AddPage after Close = %v, want ErrClosed", err)
	}

	// Illegal custom default size.
	if _, err := draw2dpdf.NewMultiPagePdfWithSize("P", "pt", 0, 100); err != draw2dpdf.ErrInvalidPageSize {
		t.Errorf("zero width = %v, want ErrInvalidPageSize", err)
	}
	if _, err := draw2dpdf.NewMultiPagePdfWithSize("P", "pt", 100, -5); err != draw2dpdf.ErrInvalidPageSize {
		t.Errorf("negative height = %v, want ErrInvalidPageSize", err)
	}
	// Illegal per page size and orientation.
	doc2 := newTestDocument(t)
	if err := doc2.AddPage(); err != nil {
		t.Fatal(err)
	}
	if err := doc2.AddPageWithSize("P", draw2dpdf.PageSize{Width: -1, Height: 10}); err != draw2dpdf.ErrInvalidPageSize {
		t.Errorf("invalid AddPage size = %v", err)
	}
	if err := doc2.AddPageWithSize("P", draw2dpdf.PageSize{Width: 100, Height: 0}); err != draw2dpdf.ErrInvalidPageSize {
		t.Errorf("partial AddPage size = %v, want ErrInvalidPageSize", err)
	}
	if doc2.PageCount() != 1 {
		t.Errorf("failed AddPage must not create a page, got count %d", doc2.PageCount())
	}
	if err := doc2.AddPageWithSize("X", draw2dpdf.PageSize{}); err != draw2dpdf.ErrInvalidOrientation {
		t.Errorf("invalid orientation = %v", err)
	}
	if _, err := draw2dpdf.NewMultiPagePdf("diagonal", "pt", "A4"); err != draw2dpdf.ErrInvalidOrientation {
		t.Errorf("invalid default orientation = %v", err)
	}
	// Unknown gofpdf size string.
	if _, err := draw2dpdf.NewMultiPagePdf("P", "pt", "nope"); err == nil {
		t.Error("unknown size string should fail")
	}

	// Negative / zero DPI.
	doc3 := newTestDocument(t)
	if err := doc3.SetDPI(0); err != draw2dpdf.ErrInvalidDPI {
		t.Errorf("zero DPI = %v, want ErrInvalidDPI", err)
	}
	if err := doc3.SetDPI(-72); err != draw2dpdf.ErrInvalidDPI {
		t.Errorf("negative DPI = %v, want ErrInvalidDPI", err)
	}
	if err := doc3.AddPage(); err != nil {
		t.Fatal(err)
	}
	if doc3.GraphicContext().GetDPI() != draw2dpdf.DPI {
		t.Error("rejected SetDPI must leave the default DPI untouched")
	}

	// Unbalanced Save across a page transition.
	doc4 := newTestDocument(t)
	gc4 := doc4.GraphicContext()
	if err := doc4.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc4.Save()
	if err := doc4.AddPage(); err != draw2dpdf.ErrUnbalancedSave {
		t.Errorf("AddPage with open Save = %v, want ErrUnbalancedSave", err)
	}
	if doc4.PageCount() != 1 {
		t.Errorf("failed AddPage must not create a page, got count %d", doc4.PageCount())
	}
	// Closing with an open Save fails as well.
	if err := doc4.Close(); err != draw2dpdf.ErrUnbalancedSave {
		t.Errorf("Close with open Save = %v, want ErrUnbalancedSave", err)
	}
	// After balancing, the document closes cleanly.
	gc4.Restore()
	if err := doc4.Close(); err != nil {
		t.Errorf("Close after Restore failed: %v", err)
	}

	// Restore without Save surfaces an error on the document.
	doc5 := newTestDocument(t)
	gc5 := doc5.GraphicContext()
	if err := doc5.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc5.Restore()
	if !doc5.Pdf().Err() {
		t.Error("Restore without Save should set the underlying pdf error")
	}

	// Oversized page: 15000 pt exceeds the 14400 point PDF limit.
	if _, err := draw2dpdf.NewMultiPagePdfWithSize("P", "pt", 15000, 100); err != draw2dpdf.ErrPageTooLarge {
		t.Errorf("oversized default page = %v, want ErrPageTooLarge", err)
	}
	doc6 := newTestDocument(t)
	if err := doc6.AddPage(); err != nil {
		t.Fatal(err)
	}
	if err := doc6.AddPageWithSize("P", draw2dpdf.PageSize{Width: 200, Height: 15000}); err != draw2dpdf.ErrPageTooLarge {
		t.Errorf("oversized AddPage = %v, want ErrPageTooLarge", err)
	}
	// The same limit applies regardless of the unit: 600 cm exceeds it.
	if _, err := draw2dpdf.NewMultiPagePdfWithSize("P", "cm", 600, 10); err != draw2dpdf.ErrPageTooLarge {
		t.Errorf("oversized cm page = %v, want ErrPageTooLarge", err)
	}
}

// TestMultiPageClearIsPageScoped verifies Clear/ClearRect only paint on
// the current page and never leak into another page's content stream.
func TestMultiPageClearIsPageScoped(t *testing.T) {
	doc := newTestDocument(t)
	gc := doc.GraphicContext()
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc.Clear()
	gc.ClearRect(1, 2, 3, 4)
	if err := doc.AddPage(); err != nil {
		t.Fatal(err)
	}
	gc.FillStringAt("second", 1, 1)
	pages, err := parsePdfPages(renderDoc(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	// ClearRect/Clear are emitted as filled path rectangles ("f*" fill
	// operator); page 1 has two (Clear + ClearRect), page 2 has none.
	if countOccurrences(pages[0].content, "f*") < 2 {
		t.Errorf("page 1 should contain the clear rectangles: %q", pages[0].content)
	}
	if strings.Contains(pages[1].content, "f*") {
		t.Errorf("page 2 inherited clear rectangle: %q", pages[1].content)
	}
}
