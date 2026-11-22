// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dpdf"
)

// strokeAnglePdf renders a two-segment acute angle into an uncompressed pdf
// and returns its content as a string.
func strokeAnglePdf(t *testing.T, draw func(gc *draw2dpdf.GraphicContext)) string {
	t.Helper()
	dest := draw2dpdf.NewPdf("L", "mm", "A4")
	dest.SetCompression(false)
	gc := draw2dpdf.NewGraphicContext(dest)
	draw(gc)
	var buf bytes.Buffer
	if err := dest.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func acuteAngle(gc *draw2dpdf.GraphicContext) {
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.LineTo(10, 100)
	gc.Stroke()
}

func TestPdfMiterLimitOperator(t *testing.T) {
	content := strokeAnglePdf(t, func(gc *draw2dpdf.GraphicContext) {
		gc.SetLineJoin(draw2d.MiterJoin)
		gc.SetMiterLimit(2.5)
		acuteAngle(gc)
	})
	if !strings.Contains(content, "2.5 M\n") {
		t.Errorf("pdf content misses the miter limit operator '2.5 M'")
	}
	if !strings.Contains(content, "0 j\n") {
		t.Errorf("pdf content misses the miter join style operator '0 j'")
	}
}

func TestPdfMiterLimitDefaultNotEmitted(t *testing.T) {
	// without an explicit SetMiterLimit the pdf output must stay unchanged
	content := strokeAnglePdf(t, func(gc *draw2dpdf.GraphicContext) {
		gc.SetLineJoin(draw2d.MiterJoin)
		acuteAngle(gc)
	})
	if strings.Contains(content, " M\n") {
		t.Errorf("pdf content must not contain a miter limit operator by default")
	}
}

func TestPdfMiterLimitSanitizesValues(t *testing.T) {
	for _, limit := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		content := strokeAnglePdf(t, func(gc *draw2dpdf.GraphicContext) {
			gc.SetLineJoin(draw2d.MiterJoin)
			gc.SetMiterLimit(limit)
			acuteAngle(gc)
		})
		if !strings.Contains(content, "10 M\n") {
			t.Errorf("limit %v: pdf content misses the fallback operator '10 M'", limit)
		}
	}
	content := strokeAnglePdf(t, func(gc *draw2dpdf.GraphicContext) {
		gc.SetLineJoin(draw2d.MiterJoin)
		gc.SetMiterLimit(-2)
		acuteAngle(gc)
	})
	if !strings.Contains(content, "1 M\n") {
		t.Errorf("negative limit: pdf content misses the clamped operator '1 M'")
	}
}

func TestPdfMiterLimitSaveRestore(t *testing.T) {
	content := strokeAnglePdf(t, func(gc *draw2dpdf.GraphicContext) {
		gc.SetLineJoin(draw2d.MiterJoin)
		gc.SetMiterLimit(4)
		acuteAngle(gc)
		gc.Save()
		gc.SetMiterLimit(1.5)
		acuteAngle(gc)
		gc.Restore()
		// the restored miter limit is emitted again for a miter join
		acuteAngle(gc)
	})
	if !strings.Contains(content, "4 M\n") {
		t.Errorf("pdf content misses '4 M'")
	}
	if !strings.Contains(content, "1.5 M\n") {
		t.Errorf("pdf content misses '1.5 M'")
	}
	if got := strings.Count(content, "4 M\n"); got != 2 {
		t.Errorf("pdf content contains '4 M' %d times, want 2 (set + restore)", got)
	}
}
