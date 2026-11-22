// Copyright 2018 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"math"
	"unicode/utf8"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
	"golang.org/x/image/math/fixed"
)

// selectableTextAnalysis collects everything SelectableTextMode has to know
// about a string before deciding whether to emit a <text> element or fall
// back to paths.
type selectableTextAnalysis struct {
	hasMissingGlyph    bool
	hasNewline         bool
	hasNonZeroKerning  bool
	hasIllegalRune     bool
	hasNonFiniteSize   bool
	hasInvalidFontName bool
	hasNegativeDPI     bool
	overlong           bool
	fontLoaded         bool
	lines              []string
}

// analyzeSelectableText inspects text against the current context without
// mutating it. Font loading errors are reported through fontLoaded.
func (gc *GraphicContext) analyzeSelectableText(text string, opts SelectableTextOptions) selectableTextAnalysis {
	var a selectableTextAnalysis

	// font size
	if math.IsNaN(gc.Current.FontSize) || math.IsInf(gc.Current.FontSize, 0) {
		a.hasNonFiniteSize = true
	}

	// dpi
	if gc.DPI < 0 {
		a.hasNegativeDPI = true
	}

	// font name legality
	if !isLegalFontName(gc.Current.FontData.Name) {
		a.hasInvalidFontName = true
	}

	// rune level checks
	runeCount := 0
	for _, r := range text {
		runeCount++
		switch r {
		case '\n', '\r':
			a.hasNewline = true
		case '\t':
			// legal XML whitespace, nothing to do
		default:
			if !isLegalXMLRune(r) {
				a.hasIllegalRune = true
			}
		}
	}
	if opts.MaxRunes > 0 && runeCount > opts.MaxRunes {
		a.overlong = true
	}

	// font dependent checks: glyph coverage and kerning
	f, err := gc.loadCurrentFont()
	if err != nil || f == nil {
		// loadCurrentFont already tried the default font as well
		a.fontLoaded = false
		a.hasMissingGlyph = true
		return a
	}
	a.fontLoaded = true

	scale := fixed.Int26_6(gc.Current.Scale)
	prev, hasPrev := truetype.Index(0), false
	for _, r := range text {
		index := f.Index(r)
		if index == 0 && r != '\n' && r != '\r' && r != '\t' {
			// index 0 means the cmap has no mapping for this rune
			a.hasMissingGlyph = true
		}
		if hasPrev {
			if k := f.Kern(scale, prev, index); k != 0 {
				a.hasNonZeroKerning = true
			}
		}
		prev, hasPrev = index, true
	}

	a.lines = splitLines(text)
	return a
}

// decide maps the analysis flags to policies and returns the action to take.
// The strictest requested action wins: SkipText > FallbackToPath >
// EmitSelectable, so a single condition that forbids selectable output can
// never be overridden by a more permissive one.
func (a selectableTextAnalysis) decide(opts SelectableTextOptions) FallbackPolicy {
	policy := EmitSelectable
	raise := func(flag bool, p FallbackPolicy) {
		if !flag {
			return
		}
		if p > policy {
			policy = p
		}
	}
	raise(a.hasMissingGlyph || !a.fontLoaded, pickFontPolicy(a, opts))
	raise(a.hasNewline, opts.Newline)
	raise(a.hasNonZeroKerning, opts.Kerning)
	raise(a.hasIllegalRune, opts.ControlChar)
	raise(a.hasNonFiniteSize, opts.NonFiniteFontSize)
	raise(a.hasInvalidFontName, opts.InvalidFontName)
	raise(a.hasNegativeDPI, opts.NegativeDPI)
	raise(a.overlong, opts.OverlongText)
	return policy
}

// pickFontPolicy distinguishes a completely unavailable font from a font
// that loads but lacks some glyphs.
func pickFontPolicy(a selectableTextAnalysis, opts SelectableTextOptions) FallbackPolicy {
	if !a.fontLoaded {
		return opts.FontUnavailable
	}
	return opts.MissingGlyph
}

// drawSelectableText either emits a selectable <text> element, falls back to
// the path representation or skips the string entirely. It never panics and
// never leaves partial state on the context, whatever the input.
func (gc *GraphicContext) drawSelectableText(text string, drawType drawType, x, y float64) float64 {
	opts := gc.svg.SelectableText
	if opts == (SelectableTextOptions{}) {
		opts = DefaultSelectableTextOptions()
	}

	a := gc.analyzeSelectableText(text, opts)
	switch a.decide(opts) {
	case SkipText:
		// Deterministic no-op: no group, no element, context untouched.
		return 0
	case FallbackToPath:
		return gc.drawStringAsPath(text, drawType, x, y)
	}

	// EmitSelectable: sanitize and build the element.
	svgText := gc.buildSelectableText(text, x, y, opts)
	if svgText == nil {
		// sanitization removed everything; emit nothing rather than an
		// empty element that would alter layout
		return 0
	}
	group := gc.newGroup(drawType)
	group.SelectableTexts = []*SelectableText{svgText}

	// Return the expected advance width, computed from truetype metrics so
	// callers keep a usable cursor even though the browser does the shaping.
	// Without a usable font or a positive scale (negative/zero DPI) the
	// truetype lookup is impossible; report zero rather than failing.
	if !a.fontLoaded || gc.Current.Scale <= 0 || !isFinite(gc.Current.Scale) {
		return 0
	}
	left, _, right, _ := gc.GetStringBounds(text)
	return right - left
}

// drawStringAsPath renders text exactly as PathFontMode does. The current
// path is guaranteed to be cleared afterwards so subsequent groups are not
// affected. When the internal truetype scale is not positive (e.g. negative
// DPI) no glyph can be rasterized, so the call degrades to a deterministic
// no-op instead of producing garbage geometry.
func (gc *GraphicContext) drawStringAsPath(text string, drawType drawType, x, y float64) float64 {
	if gc.Current.Scale <= 0 || !isFinite(gc.Current.Scale) {
		gc.Current.Path.Clear()
		return 0
	}
	// When no usable font exists, CreateStringPath draws nothing; do not
	// emit an empty group, just keep the context clean.
	if f, err := gc.loadCurrentFont(); err != nil || f == nil {
		gc.Current.Path.Clear()
		return 0
	}
	w := gc.CreateStringPath(text, x, y)
	gc.drawPaths(drawType)
	gc.Current.Path.Clear()
	return w
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// buildSelectableText produces the sanitized element. It returns nil when
// nothing renderable remains.
func (gc *GraphicContext) buildSelectableText(text string, x, y float64, opts SelectableTextOptions) *SelectableText {
	lines := sanitizedLines(text)
	if len(lines) == 0 {
		return nil
	}
	hasContent := false
	for _, l := range lines {
		if l != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return nil
	}

	t := &SelectableText{
		Position:   Position{X: x, Y: y},
		FontFamily: gc.selectableFontFamily(opts),
		FontStyle:  fontStyleString(gc.Current.FontData.Style),
		FontWeight: fontWeightString(gc.Current.FontData.Style),
	}
	// Path-mode geometry is scaled by the DPI (1pt = DPI/72 user units),
	// so the font-size attribute must carry the same px-equivalent size or
	// the browser would render the text smaller than the path output.
	if size := gc.selectableFontSize(); size > 0 {
		t.FontSize = size
	}

	if len(lines) == 1 {
		t.SingleLine = lines[0]
		return t
	}
	// 'em' tracks the (DPI-compensated) font-size automatically.
	for i, line := range lines {
		span := &TSpan{Text: line}
		if i > 0 {
			span.DY = "1em"
			span.X = x
		}
		t.Lines = append(t.Lines, span)
	}
	return t
}

// selectableFontSize converts the point font size into SVG user units
// according to the current DPI. It returns 0 for non-finite or non-positive
// sizes, in which case no font-size attribute is emitted.
func (gc *GraphicContext) selectableFontSize() float64 {
	size := gc.Current.FontSize
	if !isFinite(size) || size <= 0 {
		return 0
	}
	dpi := gc.DPI
	if dpi <= 0 {
		// no usable DPI: keep the point value rather than emitting zero
		dpi = 72
	}
	return size * float64(dpi) / 72.0
}

// selectableFontFamily returns a quoted, legal font-family value with a
// generic fallback. The built-in "luxi" fonts are mapped to the family
// names fontconfig/browsers actually register ("Luxi Sans", "Luxi Mono",
// "Luxi Serif") because "luxi" itself never matches an installed font.
// When the configured name is otherwise illegal only the generic family
// is emitted.
func (gc *GraphicContext) selectableFontFamily(opts SelectableTextOptions) string {
	generic := genericFontFamily(gc.Current.FontData.Family)
	name := gc.Current.FontData.Name
	if name == "luxi" {
		return "'" + luxiCSSFamily(gc.Current.FontData.Family) + "', " + generic
	}
	if !isLegalFontName(name) {
		return generic
	}
	return "'" + sanitizeFontName(name) + "', " + generic
}

// luxiCSSFamily maps the FontData family of the bundled luxi fonts to the
// family name advertised by their font files.
func luxiCSSFamily(family draw2d.FontFamily) string {
	switch family {
	case draw2d.FontFamilySerif:
		return "Luxi Serif"
	case draw2d.FontFamilyMono:
		return "Luxi Mono"
	default:
		return "Luxi Sans"
	}
}

func genericFontFamily(f draw2d.FontFamily) string {
	switch f {
	case draw2d.FontFamilySerif:
		return "serif"
	case draw2d.FontFamilyMono:
		return "monospace"
	default:
		return "sans-serif"
	}
}

func fontStyleString(style draw2d.FontStyle) string {
	if style&draw2d.FontStyleItalic != 0 {
		return "italic"
	}
	return ""
}

func fontWeightString(style draw2d.FontStyle) string {
	if style&draw2d.FontStyleBold != 0 {
		return "bold"
	}
	return ""
}

// splitLines breaks text on \n and \r, handling the CRLF sequence once.
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '\n' || r == '\r' {
			lines = append(lines, text[start:i])
			i += size
			if r == '\r' && i < len(text) && text[i] == '\n' {
				i++
			}
			start = i
			continue
		}
		i += size
	}
	lines = append(lines, text[start:])
	return lines
}

// sanitizedLines splits text into lines and removes every rune that is not
// legal in XML 1.0 content.
func sanitizedLines(text string) []string {
	lines := splitLines(text)
	out := make([]string, len(lines))
	for i, line := range lines {
		var buf []rune
		for _, r := range line {
			if r == '\t' || isLegalXMLRune(r) {
				buf = append(buf, r)
			}
		}
		out[i] = string(buf)
	}
	return out
}

// isLegalXMLRune reports whether r may appear in an XML 1.0 document.
// See https://www.w3.org/TR/xml/#NT-Char.
func isLegalXMLRune(r rune) bool {
	switch {
	case r == 0x09 || r == 0x0A || r == 0x0D:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}

// isLegalFontName reports whether name can be used inside a font-family
// attribute after quoting. Empty names and names containing control
// characters, quotes or XML-significant markup are rejected.
func isLegalFontName(name string) bool {
	if name == "" {
		return false
	}
	if !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			return false
		}
		switch r {
		case '"', '\'', '<', '>', '&':
			return false
		}
	}
	return true
}

// sanitizeFontName strips characters that would break a quoted CSS token.
// It is only called after isLegalFontName succeeded.
func sanitizeFontName(name string) string {
	var b []rune
	for _, r := range name {
		if r == '\\' {
			continue
		}
		b = append(b, r)
	}
	return string(b)
}
