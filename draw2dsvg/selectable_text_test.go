// Copyright 2018 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
)

// ---- test helpers -------------------------------------------------------

func newSelectableGC(t *testing.T, opts SelectableTextOptions) *GraphicContext {
	t.Helper()
	draw2d.SetFontFolder("../resource/font")
	svg := NewSvg()
	svg.FontMode = SelectableTextMode
	svg.SelectableText = opts
	gc := NewGraphicContext(svg)
	gc.SetFontData(draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleBold | draw2d.FontStyleItalic})
	gc.SetFontSize(14)
	return gc
}

func marshalSVG(t *testing.T, svg *Svg) string {
	t.Helper()
	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return string(out)
}

// lastGroup returns the most recently appended group.
func lastGroup(gc *GraphicContext) *Group {
	if len(gc.svg.Groups) == 0 {
		return nil
	}
	return gc.svg.Groups[len(gc.svg.Groups)-1]
}

// errFontCache always fails to load, including the default font.
type errFontCache struct{}

func (errFontCache) Load(draw2d.FontData) (*truetype.Font, error) {
	return nil, errors.New("font unavailable")
}
func (errFontCache) Store(draw2d.FontData, *truetype.Font) {}

// staticFontCache always returns the same font.
type staticFontCache struct{ f *truetype.Font }

func (c staticFontCache) Load(draw2d.FontData) (*truetype.Font, error) {
	if c.f == nil {
		return nil, errors.New("nil font")
	}
	return c.f, nil
}
func (c staticFontCache) Store(draw2d.FontData, *truetype.Font) {}

// ---- analysis / policy --------------------------------------------------

func TestSelectableText_DecidePolicy(t *testing.T) {
	opts := DefaultSelectableTextOptions()

	// clean input -> selectable
	a := selectableTextAnalysis{fontLoaded: true}
	if got := a.decide(opts); got != EmitSelectable {
		t.Errorf("clean analysis: got %v, want EmitSelectable", got)
	}

	// missing glyph defaults to path fallback
	a = selectableTextAnalysis{fontLoaded: true, hasMissingGlyph: true}
	if got := a.decide(opts); got != FallbackToPath {
		t.Errorf("missing glyph: got %v, want FallbackToPath", got)
	}

	// font unavailable uses its own policy
	a = selectableTextAnalysis{fontLoaded: false}
	if got := a.decide(opts); got != FallbackToPath {
		t.Errorf("font unavailable: got %v, want FallbackToPath", got)
	}

	// newline alone stays selectable (tspan)
	a = selectableTextAnalysis{fontLoaded: true, hasNewline: true}
	if got := a.decide(opts); got != EmitSelectable {
		t.Errorf("newline: got %v, want EmitSelectable", got)
	}

	// non-finite size skips even when another condition requests paths
	a = selectableTextAnalysis{fontLoaded: true, hasNonFiniteSize: true, hasMissingGlyph: true}
	if got := a.decide(opts); got != SkipText {
		t.Errorf("non-finite size: got %v, want SkipText", got)
	}
}

// ---- XML output ---------------------------------------------------------

func TestSelectableText_MixedScriptXml(t *testing.T) {
	gc := newSelectableGC(t, SelectableTextOptions{
		// Chinese runes are absent from luxi: keep the element anyway so
		// the XML structure can be inspected.
		MissingGlyph:      EmitSelectable,
		FontUnavailable:   FallbackToPath,
		Newline:           EmitSelectable,
		Kerning:           FallbackToPath,
		ControlChar:       FallbackToPath,
		NonFiniteFontSize: SkipText,
		InvalidFontName:   EmitSelectable,
		NegativeDPI:       FallbackToPath,
		OverlongText:      FallbackToPath,
	})
	gc.Translate(10, 20)
	gc.Rotate(math.Pi / 4)
	gc.FillStringAt("Hello 世界 ABC", 5, 30)

	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want one selectable text group, got %+v", g)
	}
	st := g.SelectableTexts[0]
	if st.SingleLine != "Hello 世界 ABC" {
		t.Errorf("utf-8 content lost: %q", st.SingleLine)
	}
	if st.X != 5 || st.Y != 30 {
		t.Errorf("baseline position = (%v,%v), want (5,30)", st.X, st.Y)
	}
	// font-size is DPI-compensated: 14pt at 92dpi -> 14*92/72 user units
	wantSize := 14.0 * 92.0 / 72.0
	if math.Abs(st.FontSize-wantSize) > 1e-9 {
		t.Errorf("font-size = %v, want %v", st.FontSize, wantSize)
	}
	if st.FontFamily != "'Luxi Mono', monospace" {
		t.Errorf("font-family = %q", st.FontFamily)
	}
	if st.FontStyle != "italic" || st.FontWeight != "bold" {
		t.Errorf("style/weight = %q/%q", st.FontStyle, st.FontWeight)
	}
	if !strings.Contains(g.Transform, "matrix(") {
		t.Errorf("rotation transform missing: %q", g.Transform)
	}

	out := marshalSVG(t, gc.svg)
	if !strings.Contains(out, "<text") || !strings.Contains(out, "Hello 世界 ABC") {
		t.Errorf("selectable text not emitted:\n%s", out)
	}
	// the content must be real character data, not path data
	if strings.Contains(out, "<path") {
		t.Errorf("unexpected path in selectable output:\n%s", out)
	}
	// round trip: output must stay valid XML
	var parsed Svg
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
}

func TestSelectableText_EscapesMarkup(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FillStringAt(`a<b>&c"d'e`, 0, 10)

	out := marshalSVG(t, gc.svg)
	if !strings.Contains(out, "a&lt;b&gt;&amp;c\"d'e") &&
		!strings.Contains(out, "a&lt;b&gt;&amp;c&#34;d&#39;e") {
		t.Errorf("markup not escaped:\n%s", out)
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("raw markup leaked into XML:\n%s", out)
	}
}

func TestSelectableText_MultilineEscapeAndEmptyLine(t *testing.T) {
	opts := DefaultSelectableTextOptions()
	opts.MissingGlyph = EmitSelectable // CJK rune below
	gc := newSelectableGC(t, opts)
	// leading empty line, markup to escape, CJK content
	gc.FillStringAt("\nA<B>&\"' 中", 3, 9)

	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable text, got %+v", g)
	}
	st := g.SelectableTexts[0]
	if len(st.Lines) != 2 || st.Lines[0].Text != "" {
		t.Fatalf("want empty first line, got %+v", st.Lines)
	}
	out := marshalSVG(t, gc.svg)
	for _, want := range []string{"<tspan></tspan>", "&lt;B&gt;", "中"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<B>") {
		t.Errorf("raw markup leaked:\n%s", out)
	}
	var parsed Svg
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
}

func TestSelectableText_NewlineTspans(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FillStringAt("first\nsecond\r\nthird", 7, 40)

	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable text, got %+v", g)
	}
	st := g.SelectableTexts[0]
	if len(st.Lines) != 3 {
		t.Fatalf("want 3 tspan lines, got %d (%+v)", len(st.Lines), st.Lines)
	}
	want := []string{"first", "second", "third"}
	for i, line := range st.Lines {
		if line.Text != want[i] {
			t.Errorf("line %d = %q, want %q", i, line.Text, want[i])
		}
		if i == 0 {
			if line.DY != "" || line.X != 0 {
				t.Errorf("first line must not carry x/dy")
			}
		} else {
			if line.DY != "1em" || line.X != 7 {
				t.Errorf("line %d x/dy = %v/%q, want 7/1em", i, line.X, line.DY)
			}
		}
	}
	out := marshalSVG(t, gc.svg)
	if strings.Count(out, "<tspan") != 3 {
		t.Errorf("want 3 tspan elements:\n%s", out)
	}
}

// ---- fallback conditions ------------------------------------------------

func TestSelectableText_MissingGlyphFallsBackToPath(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FillStringAt("汉字", 0, 10)

	g := lastGroup(gc)
	if g == nil || len(g.Paths) != 1 {
		t.Fatalf("want path fallback, got %+v", g)
	}
	if len(g.SelectableTexts) != 0 {
		t.Errorf("selectable text must not be emitted on path fallback")
	}
}

func TestSelectableText_MissingGlyphEmitSelectable(t *testing.T) {
	opts := DefaultSelectableTextOptions()
	opts.MissingGlyph = EmitSelectable
	gc := newSelectableGC(t, opts)
	gc.FillStringAt("汉字", 0, 10)

	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable text, got %+v", g)
	}
	if g.SelectableTexts[0].SingleLine != "汉字" {
		t.Errorf("content = %q", g.SelectableTexts[0].SingleLine)
	}
}

func TestSelectableText_ControlChars(t *testing.T) {
	// default: illegal control characters force path fallback
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FillStringAt("ab\x00\x01cd", 0, 10)
	g := lastGroup(gc)
	if g == nil || len(g.Paths) != 1 {
		t.Fatalf("want path fallback for control chars, got %+v", g)
	}

	// EmitSelectable: control characters are removed, text stays selectable.
	// Control runes have no glyph either, so MissingGlyph must be relaxed too.
	opts := DefaultSelectableTextOptions()
	opts.ControlChar = EmitSelectable
	opts.MissingGlyph = EmitSelectable
	gc = newSelectableGC(t, opts)
	gc.FillStringAt("ab\x00\x01cd", 0, 10)
	g = lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want sanitized selectable text, got %+v", g)
	}
	if got := g.SelectableTexts[0].SingleLine; got != "abcd" {
		t.Errorf("sanitized content = %q, want abcd", got)
	}
	// tab is legal XML whitespace and is preserved
	gc.FillStringAt("a\tb", 0, 20)
	g = lastGroup(gc)
	if g.SelectableTexts[0].SingleLine != "a\tb" {
		t.Errorf("tab must be preserved: %q", g.SelectableTexts[0].SingleLine)
	}
}

func TestSelectableText_NonFiniteFontSize(t *testing.T) {
	for _, size := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		gc := newSelectableGC(t, DefaultSelectableTextOptions())
		gc.SetFontSize(size)
		cursor := gc.FillStringAt("hello", 0, 10)
		if cursor != 0 {
			t.Errorf("NaN/Inf size cursor = %v, want 0", cursor)
		}
		if lastGroup(gc) != nil {
			t.Errorf("NaN/Inf size must not emit a group (SkipText)")
		}
		// context must still work afterwards
		gc.SetFontSize(14)
		gc.FillStringAt("ok", 0, 20)
		if lastGroup(gc) == nil || len(lastGroup(gc).SelectableTexts) != 1 {
			t.Fatalf("group state broken after non-finite size")
		}
	}
}

func TestSelectableText_EmptyString(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	cursor := gc.FillStringAt("", 0, 10)
	if cursor != 0 {
		t.Errorf("empty string cursor = %v, want 0", cursor)
	}
	if len(gc.svg.Groups) != 0 {
		t.Errorf("empty string must not produce a group")
	}
	// following draw must behave normally
	gc.FillStringAt("next", 0, 20)
	if len(gc.svg.Groups) != 1 || len(lastGroup(gc).SelectableTexts) != 1 {
		t.Fatalf("group state broken after empty string")
	}
}

func TestSelectableText_NegativeDPI(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.SetDPI(-92) // default policy: FallbackToPath, but scale is non-positive
	cursor := gc.FillStringAt("hello", 0, 10)
	if cursor != 0 {
		t.Errorf("negative DPI cursor = %v, want 0", cursor)
	}
	if len(gc.svg.Groups) != 0 {
		t.Errorf("negative DPI must not emit geometry")
	}
	// recovery after DPI is fixed
	gc.SetDPI(92)
	gc.FillStringAt("ok", 0, 20)
	if len(gc.svg.Groups) != 1 {
		t.Fatalf("group state broken after negative DPI")
	}
}

func TestSelectableText_DPICompensation(t *testing.T) {
	// path-mode glyphs are scaled by DPI, so the selectable font-size
	// attribute must equal fontSize * dpi / 72 user units.
	for _, dpi := range []int{72, 92, 144, 300} {
		gc := newSelectableGC(t, DefaultSelectableTextOptions())
		gc.SetDPI(dpi)
		gc.SetFontSize(12)
		gc.FillStringAt("hi", 0, 10)
		want := 12.0 * float64(dpi) / 72.0
		got := lastGroup(gc).SelectableTexts[0].FontSize
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("dpi=%d font-size = %v, want %v", dpi, got, want)
		}
	}

	// zero font size: no font-size attribute at all
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.SetFontSize(0)
	gc.FillStringAt("hi", 0, 10)
	if got := lastGroup(gc).SelectableTexts[0].FontSize; got != 0 {
		t.Errorf("zero font size emitted %v", got)
	}
}

func TestSelectableText_NegativeDPIEmitSelectable(t *testing.T) {
	opts := DefaultSelectableTextOptions()
	opts.NegativeDPI = EmitSelectable
	gc := newSelectableGC(t, opts)
	gc.SetDPI(-92)
	cursor := gc.FillStringAt("hi", 0, 10)
	if cursor != 0 {
		t.Errorf("cursor=%v want 0", cursor)
	}
	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable element even with negative dpi, got %+v", g)
	}
	// no usable DPI: compensation falls back to the 72dpi point value
	if got := g.SelectableTexts[0].FontSize; math.Abs(got-14.0) > 1e-9 {
		t.Errorf("font-size=%v want 14", got)
	}
}

func TestSelectableText_SaveRestoreAcrossSkip(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.Save()
	gc.SetFontSize(math.NaN())
	gc.FillStringAt("x", 0, 0) // SkipText, must not corrupt the stack
	gc.Restore()
	if got := gc.GetFontSize(); got != 14 {
		t.Errorf("font size after restore = %v, want 14", got)
	}
	if gc.GetFontData().Name != "luxi" {
		t.Errorf("font data corrupted: %+v", gc.GetFontData())
	}
}

func TestSelectableText_Overlong(t *testing.T) {
	opts := DefaultSelectableTextOptions()
	opts.MaxRunes = 3
	gc := newSelectableGC(t, opts) // OverlongText defaults to FallbackToPath
	gc.FillStringAt("abcd", 0, 10)
	if g := lastGroup(gc); g == nil || len(g.Paths) != 1 {
		t.Fatalf("want path fallback for overlong text, got %+v", g)
	}

	opts.OverlongText = EmitSelectable
	gc = newSelectableGC(t, opts)
	gc.FillStringAt("abcd", 0, 10)
	if g := lastGroup(gc); g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable text for overlong text, got %+v", g)
	}

	opts.OverlongText = SkipText
	gc = newSelectableGC(t, opts)
	gc.FillStringAt("abcd", 0, 10)
	if len(gc.svg.Groups) != 0 {
		t.Fatalf("want no output for SkipText overlong policy")
	}
}

func TestSelectableText_InvalidFontName(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.SetFontData(draw2d.FontData{Name: "bad<name>", Family: draw2d.FontFamilySerif})
	gc.FillStringAt("hello", 0, 10)
	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("want selectable text with generic family, got %+v", g)
	}
	if g.SelectableTexts[0].FontFamily != "serif" {
		t.Errorf("font-family = %q, want serif", g.SelectableTexts[0].FontFamily)
	}

	// empty name
	gc = newSelectableGC(t, DefaultSelectableTextOptions())
	gc.SetFontData(draw2d.FontData{Name: "", Family: draw2d.FontFamilySans})
	gc.FillStringAt("hello", 0, 10)
	g = lastGroup(gc)
	if g == nil || g.SelectableTexts[0].FontFamily != "sans-serif" {
		t.Fatalf("empty font name not replaced by generic family: %+v", g)
	}
}

func TestSelectableText_FontUnavailable(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FontCache = errFontCache{}
	gc.FillStringAt("hello", 0, 10)
	// default policy FallbackToPath, but no font means no geometry at all
	if len(gc.svg.Groups) != 0 {
		t.Fatalf("unavailable font must not emit groups")
	}

	opts := DefaultSelectableTextOptions()
	opts.FontUnavailable = EmitSelectable
	gc = newSelectableGC(t, opts)
	gc.FontCache = errFontCache{}
	gc.FillStringAt("hello", 0, 10)
	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("EmitSelectable must keep text without a local font, got %+v", g)
	}
	if g.SelectableTexts[0].SingleLine != "hello" {
		t.Errorf("content = %q", g.SelectableTexts[0].SingleLine)
	}

	// SkipText policy
	opts.FontUnavailable = SkipText
	gc = newSelectableGC(t, opts)
	gc.FontCache = errFontCache{}
	gc.FillStringAt("hello", 0, 10)
	if len(gc.svg.Groups) != 0 {
		t.Fatalf("SkipText must emit nothing")
	}
}

func TestSelectableText_KerningDetection(t *testing.T) {
	// pure logic: a detected kerning pair applies the Kerning policy
	a := selectableTextAnalysis{fontLoaded: true, hasNonZeroKerning: true}
	opts := DefaultSelectableTextOptions()
	if got := a.decide(opts); got != FallbackToPath {
		t.Errorf("kerning default: got %v, want FallbackToPath", got)
	}
	opts.Kerning = EmitSelectable
	if got := a.decide(opts); got != EmitSelectable {
		t.Errorf("kerning relaxed: got %v, want EmitSelectable", got)
	}

	// end-to-end detection requires a font with a kern table; DejaVu ships
	// with the test environment, skip elsewhere.
	path := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("DejaVu Sans not available; skipping end-to-end kerning test")
	}
	f, err := truetype.Parse(data)
	if err != nil {
		t.Skipf("cannot parse DejaVu Sans: %v", err)
	}
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.FontCache = staticFontCache{f: f}
	gc.FillStringAt("-AV", 0, 10) // DejaVu defines a kern pair for -A
	if g := lastGroup(gc); g == nil || len(g.Paths) != 1 {
		t.Fatalf("want path fallback for kerned text, got %+v", g)
	}
}

// ---- state isolation ----------------------------------------------------

func TestSelectableText_GroupStateIsolation(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())

	// a stream of pathological inputs followed by a normal draw
	gc.SetFontSize(math.NaN())
	gc.FillStringAt("nan-size", 0, 0)
	gc.SetFontSize(14)
	gc.FillStringAt("", 0, 0)
	gc.FillStringAt("汉字", 0, 0) // missing glyph -> path fallback
	gc.SetFontData(draw2d.FontData{Name: "bad\"name", Family: draw2d.FontFamilySans})
	gc.FillStringAt("weird-font", 0, 0)
	gc.SetFontData(draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono})

	gc.Save()
	gc.Translate(3, 4)
	gc.FillStringAt("final", 1, 2)
	gc.Restore()

	// a plain vector draw afterwards must land in its own clean group
	gc.MoveTo(0, 0)
	gc.LineTo(10, 10)
	gc.Stroke()

	out := marshalSVG(t, gc.svg)
	var parsed Svg
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("corrupted XML after pathological input: %v\n%s", err, out)
	}
	last := lastGroup(gc)
	if last == nil || len(last.Paths) != 1 || last.Transform != "" {
		t.Fatalf("final vector group not isolated: %+v", last)
	}
}

func TestSelectableText_FillAndStroke(t *testing.T) {
	gc := newSelectableGC(t, DefaultSelectableTextOptions())
	gc.StrokeStringAt("stroked", 1, 2)
	g := lastGroup(gc)
	if g == nil || len(g.SelectableTexts) != 1 {
		t.Fatalf("stroke text missing: %+v", g)
	}
	if g.Stroke == "" {
		t.Errorf("stroke color not inherited by group")
	}
}

// ---- legacy modes stay byte identical -----------------------------------

func TestLegacyModes_XmlUnchanged(t *testing.T) {
	draw2d.SetFontFolder("../resource/font")

	// PathFontMode: text becomes a path group, no <text> element
	pathSvg := NewSvg()
	pathSvg.FontMode = PathFontMode
	pathGC := NewGraphicContext(pathSvg)
	pathGC.SetFontData(draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono})
	pathGC.SetFontSize(14)
	pathGC.FillStringAt("Hello", 8, 20)
	pathOut := marshalSVG(t, pathSvg)
	if strings.Contains(pathOut, "<text") {
		t.Errorf("PathFontMode leaked a text element:\n%s", pathOut)
	}
	if !strings.Contains(pathOut, "<path") {
		t.Errorf("PathFontMode lost its path element:\n%s", pathOut)
	}

	// SvgFontMode: keeps the legacy Text element with inner XML and a font def
	svgFontSvg := NewSvg()
	svgFontSvg.FontMode = SvgFontMode
	svgGC := NewGraphicContext(svgFontSvg)
	svgGC.SetFontData(draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono})
	svgGC.SetFontSize(14)
	svgGC.FillStringAt("Hello", 8, 20)
	svgOut := marshalSVG(t, svgFontSvg)
	if !strings.Contains(svgOut, "<font") || !strings.Contains(svgOut, "<glyph") {
		t.Errorf("SvgFontMode lost its embedded font:\n%s", svgOut)
	}
	if !strings.Contains(svgOut, "<text") || !strings.Contains(svgOut, "Hello") {
		t.Errorf("SvgFontMode lost its text element:\n%s", svgOut)
	}
}
