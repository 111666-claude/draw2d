// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg_test

import (
	"encoding/xml"
	"math"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// strokeAngle renders a two-segment acute angle and returns the svg tree.
func strokeAngle(join draw2d.LineJoin, miterLimit float64, setLimit bool) *draw2dsvg.Svg {
	dest := draw2dsvg.NewSvg()
	gc := draw2dsvg.NewGraphicContext(dest)
	gc.SetLineJoin(join)
	if setLimit {
		gc.SetMiterLimit(miterLimit)
	}
	gc.MoveTo(10, 10)
	gc.LineTo(100, 10)
	gc.LineTo(10, 100)
	gc.Stroke()
	return dest
}

func TestSvgMiterLimitAttribute(t *testing.T) {
	dest := strokeAngle(draw2d.MiterJoin, 2.5, true)
	if len(dest.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(dest.Groups))
	}
	group := dest.Groups[0]
	if group.StrokeLinejoin != "miter" {
		t.Errorf("stroke-linejoin = %q, want %q", group.StrokeLinejoin, "miter")
	}
	if group.StrokeMiterlimit != "2.5" {
		t.Errorf("stroke-miterlimit = %q, want %q", group.StrokeMiterlimit, "2.5")
	}

	// the attribute must survive xml serialization
	data, err := xml.Marshal(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `stroke-miterlimit="2.5"`) {
		t.Errorf("serialized svg misses stroke-miterlimit: %s", data)
	}
}

func TestSvgMiterLimitDefault(t *testing.T) {
	dest := strokeAngle(draw2d.MiterJoin, 0, false)
	if got := dest.Groups[0].StrokeMiterlimit; got != "10" {
		t.Errorf("default stroke-miterlimit = %q, want %q", got, "10")
	}
}

func TestSvgMiterLimitSanitizesNonFinite(t *testing.T) {
	for _, limit := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		dest := strokeAngle(draw2d.MiterJoin, limit, true)
		if got := dest.Groups[0].StrokeMiterlimit; got != "10" {
			t.Errorf("limit %v: stroke-miterlimit = %q, want %q", limit, got, "10")
		}
	}
	// non positive limits are clamped to 1 (bevel everything)
	dest := strokeAngle(draw2d.MiterJoin, -2, true)
	if got := dest.Groups[0].StrokeMiterlimit; got != "1" {
		t.Errorf("negative limit: stroke-miterlimit = %q, want %q", got, "1")
	}
}

func TestSvgMiterLimitOmittedForOtherJoins(t *testing.T) {
	for _, join := range []draw2d.LineJoin{draw2d.BevelJoin, draw2d.RoundJoin} {
		dest := strokeAngle(join, 2.5, true)
		group := dest.Groups[0]
		if group.StrokeMiterlimit != "" {
			t.Errorf("join %v: unexpected stroke-miterlimit %q", join, group.StrokeMiterlimit)
		}
		data, err := xml.Marshal(dest)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "stroke-miterlimit") {
			t.Errorf("join %v: serialized svg must not contain stroke-miterlimit", join)
		}
	}
}
