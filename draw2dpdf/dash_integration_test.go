// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf

import (
	"bytes"
	"image"
	"math"
	"strings"
	"testing"
)

// newTestContext creates a graphic context whose content stream is emitted
// uncompressed so the pdf operators can be inspected directly.
func newTestContext(t *testing.T) *GraphicContext {
	t.Helper()
	pdf := NewPdf("P", "pt", "A4")
	pdf.SetCompression(false)
	return NewGraphicContext(pdf)
}

// content renders the document and returns the (uncompressed) content
// stream of the first page.
func (gc *GraphicContext) content(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := gc.pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output failed: %v", err)
	}
	raw := buf.String()
	start := strings.Index(raw, "stream\n")
	if start < 0 {
		start = strings.Index(raw, "stream\r\n")
	}
	if start < 0 {
		t.Fatal("no content stream found in pdf")
	}
	start = strings.Index(raw[start:], "\n") + start + 1
	end := strings.Index(raw[start:], "endstream")
	if end < 0 {
		t.Fatal("no endstream found in pdf")
	}
	return raw[start : start+end]
}

func TestPdfDashOperator(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{6, 4}, 0)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	content := gc.content(t)
	if !strings.Contains(content, "[6.00 4.00] 0.00 d") {
		t.Errorf("dash operator not emitted, content:\n%s", content)
	}
}

func TestPdfDashOddLengthAndNegativeOffset(t *testing.T) {
	gc := newTestContext(t)
	// odd length -> duplicated (pattern length becomes 36);
	// offset -3 is reduced into [0,36) -> 33
	gc.SetLineDash([]float64{3, 7, 8}, -3)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	content := gc.content(t)
	if !strings.Contains(content, "[3.00 7.00 8.00 3.00 7.00 8.00] 33.00 d") {
		t.Errorf("normalized dash operator not emitted, content:\n%s", content)
	}
}

func TestPdfDashEmptyResetsSolid(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{6, 4}, 0)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	gc.SetLineDash(nil, 0)
	gc.MoveTo(10, 20)
	gc.LineTo(100, 20)
	gc.Stroke()
	content := gc.content(t)
	if !strings.Contains(content, "[] 0.00 d") {
		t.Errorf("solid line reset operator not emitted, content:\n%s", content)
	}
}

func TestPdfDashAllZeroIsSolid(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{0, 0, 0}, 5)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	content := gc.content(t)
	if strings.Contains(content, " d\n") || strings.Contains(content, "d ") {
		t.Errorf("no dash operator expected for all-zero pattern, content:\n%s", content)
	}
}

func TestPdfDashInvalidReportsError(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{-1, 2}, 0)
	if !gc.pdf.Err() {
		t.Error("expected pdf error state for negative dash length")
	}
	gc.pdf.ClearError()
	gc.SetLineDash([]float64{1, 2}, math.NaN())
	if !gc.pdf.Err() {
		t.Error("expected pdf error state for NaN offset")
	}
	// the document must still render instead of looping
	gc.pdf.ClearError()
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	gc.content(t)
}

func TestPdfDashSaveRestore(t *testing.T) {
	gc := newTestContext(t)
	// outer dash
	gc.SetLineDash([]float64{10, 5}, 0)
	gc.Save()
	// inner dash overrides it
	gc.SetLineDash([]float64{2, 3}, 1)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	gc.Restore()
	// outer dash must be active again
	gc.MoveTo(10, 30)
	gc.LineTo(100, 30)
	gc.Stroke()
	content := gc.content(t)
	inner := "[2.00 3.00] 1.00 d"
	outer := "[10.00 5.00] 0.00 d"
	if !strings.Contains(content, inner) {
		t.Errorf("inner dash missing, content:\n%s", content)
	}
	if !strings.Contains(content, outer) {
		t.Errorf("outer dash not restored, content:\n%s", content)
	}
	if strings.LastIndex(content, inner) > strings.LastIndex(content, outer) {
		t.Errorf("outer dash should be re-emitted after restore, content:\n%s", content)
	}
}

func TestPdfDashFillStroke(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{5, 3}, 2)
	gc.SetFillColor(image.White)
	gc.MoveTo(10, 10)
	gc.LineTo(110, 10)
	gc.LineTo(110, 60)
	gc.Close()
	gc.FillStroke()
	content := gc.content(t)
	if !strings.Contains(content, "[5.00 3.00] 2.00 d") {
		t.Errorf("dash must apply to FillStroke, content:\n%s", content)
	}
}

func TestPdfSolidLineUnchanged(t *testing.T) {
	gc := newTestContext(t)
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	content := gc.content(t)
	if strings.Contains(content, " d") {
		t.Errorf("solid line must not emit a dash operator, content:\n%s", content)
	}
}

func TestPdfDashMultiSubpath(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{8, 4}, 0)
	// two subpaths in a single path
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.MoveTo(10, 40)
	gc.CubicCurveTo(40, 80, 70, 0, 100, 40)
	gc.Stroke()
	content := gc.content(t)
	if !strings.Contains(content, "[8.00 4.00] 0.00 d") {
		t.Errorf("dash operator missing for multi-subpath path, content:\n%s", content)
	}
}

func TestPdfDashEdgeCasesDoNotHang(t *testing.T) {
	// Every pathological input must terminate and still produce a
	// renderable document instead of looping inside gofpdf or the
	// content stream writer.
	cases := []struct {
		name   string
		dash   []float64
		offset float64
	}{
		{"zero lengths", []float64{0, 0, 1}, 0},
		{"all zero", []float64{0, 0}, 0},
		{"negative length", []float64{-5, 2}, 0},
		{"nan length", []float64{math.NaN(), 2}, 0},
		{"inf length", []float64{math.Inf(1), 2}, 0},
		{"nan offset", []float64{1, 2}, math.NaN()},
		{"inf offset", []float64{1, 2}, math.Inf(1)},
		{"neg inf offset", []float64{1, 2}, math.Inf(-1)},
		{"huge negative offset", []float64{1, 1}, -1e12},
		{"huge positive offset", []float64{1, 1}, 1e12},
		{"single entry", []float64{7}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gc := newTestContext(t)
			gc.SetLineDash(tc.dash, tc.offset)
			// long path with many subpaths
			for i := 0; i < 50; i++ {
				gc.MoveTo(10, float64(10+i*8))
				gc.LineTo(1e5, float64(10+i*8))
			}
			gc.Stroke()
			gc.pdf.ClearError()
			content := gc.content(t)
			if strings.Contains(content, "Inf") || strings.Contains(content, "NaN") {
				t.Errorf("non-finite value emitted in content:\n%s", content)
			}
		})
	}
}

func TestPdfDashInvalidInsideSaveRestoresOuter(t *testing.T) {
	gc := newTestContext(t)
	gc.SetLineDash([]float64{9, 4}, 0) // outer valid dash
	gc.Save()
	gc.SetLineDash([]float64{-1, 2}, 0) // invalid -> solid fallback + error
	if !gc.pdf.Err() {
		t.Fatal("expected pdf error state for invalid inner dash")
	}
	gc.pdf.ClearError()
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.Stroke()
	gc.Restore() // must bring back the outer dash
	gc.MoveTo(10, 30)
	gc.LineTo(100, 30)
	gc.Stroke()
	content := gc.content(t)
	if !strings.Contains(content, "[9.00 4.00] 0.00 d") {
		t.Errorf("outer dash not restored after invalid inner dash:\n%s", content)
	}
}
