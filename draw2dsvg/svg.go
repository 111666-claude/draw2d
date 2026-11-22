// Copyright 2015 The draw2d Authors. All rights reserved.
// created: 16/12/2017 by Drahoslav Bednář

package draw2dsvg

import (
	"encoding/xml"
)

/* svg elements */

type FontMode int

// Modes of font handling in svg
const (
	// Does nothing special
	// Makes sense only for common system fonts
	SysFontMode FontMode = 1 << iota

	// Links font files in css def
	// Requires distribution of font files with outputed svg
	LinkFontMode // TODO implement

	// Embeds glyphs definition in svg file itself in svg font format
	// Has poor browser support
	SvgFontMode

	// Embeds font definiton in svg file itself in woff format as part of css def
	CssFontMode // TODO implement

	// Converts texts to paths
	PathFontMode

	// Outputs text as selectable <text>/<tspan> elements carrying the
	// UTF-8 content, position, baseline, font-family, size, fill/stroke
	// and transform. The browser (or any other SVG renderer) performs the
	// actual shaping, so the text stays selectable and searchable.
	// Cases that cannot be represented faithfully (missing glyphs,
	// non-finite font size, ...) are handled according to
	// Svg.SelectableText options and can fall back to PathFontMode.
	SelectableTextMode
)

// FallbackPolicy decides what SelectableTextMode does when a piece of text
// cannot be represented faithfully by a <text> element.
type FallbackPolicy int

const (
	// EmitSelectable keeps the <text>/<tspan> element and sanitizes the
	// content instead (e.g. drops control characters, keeps the browser
	// fallback font for missing glyphs).
	EmitSelectable FallbackPolicy = iota

	// FallbackToPath renders the whole string as vector paths, exactly as
	// PathFontMode would. The output stays visually exact but the text is
	// no longer selectable.
	FallbackToPath

	// SkipText emits nothing for the string.
	SkipText
)

// SelectableTextOptions configures SelectableTextMode. The zero value is
// usable: every condition keeps the <text> element (EmitSelectable). Use
// DefaultSelectableTextOptions for strict output that falls back to paths
// whenever selectable rendering could diverge from PathFontMode.
type SelectableTextOptions struct {
	// MissingGlyph is used when the current font has no glyph for at least
	// one rune of the string (truetype index 0).
	MissingGlyph FallbackPolicy

	// FontUnavailable is used when the current FontData cannot be loaded
	// and the default font cannot be loaded either.
	FontUnavailable FallbackPolicy

	// Newline is used when the string contains a line break ('\n' or
	// '\r'). With EmitSelectable the lines are emitted as successive
	// <tspan> elements with a reset x and a dy of one font size.
	Newline FallbackPolicy

	// Kerning is used when the font defines a non-zero kerning pair inside
	// the string. Browsers apply their own kerning, which may differ from
	// the truetype tables used by the path backend.
	Kerning FallbackPolicy

	// ControlChar is used when the string contains characters that are not
	// legal in XML 1.0 text content (C0 controls other than tab/newline/CR,
	// surrogates, non-characters). With EmitSelectable they are removed.
	ControlChar FallbackPolicy

	// NonFiniteFontSize is used when the current font size is NaN, +Inf or
	// -Inf (a font-size attribute cannot represent any of those).
	NonFiniteFontSize FallbackPolicy

	// InvalidFontName is used when the current font name cannot be used in
	// a font-family attribute (empty or containing illegal characters).
	// With EmitSelectable the generic family of FontData is used instead.
	InvalidFontName FallbackPolicy

	// NegativeDPI is used when the graphic context DPI is negative, which
	// makes the internal truetype scale non-positive.
	NegativeDPI FallbackPolicy

	// MaxRunes caps the number of runes emitted inside a single <text>
	// element. Zero means no limit. Longer strings use OverlongText.
	MaxRunes int

	// OverlongText is used when utf8.RuneCountInString exceeds MaxRunes.
	OverlongText FallbackPolicy
}

// DefaultSelectableTextOptions returns strict options: everything that could
// make the selectable output diverge from the path output falls back to
// paths, except line breaks which are represented by <tspan> elements.
func DefaultSelectableTextOptions() SelectableTextOptions {
	return SelectableTextOptions{
		MissingGlyph:      FallbackToPath,
		FontUnavailable:   FallbackToPath,
		Newline:           EmitSelectable,
		Kerning:           FallbackToPath,
		ControlChar:       FallbackToPath,
		NonFiniteFontSize: SkipText,
		InvalidFontName:   EmitSelectable,
		NegativeDPI:       FallbackToPath,
		MaxRunes:          0,
		OverlongText:      FallbackToPath,
	}
}

type Svg struct {
	XMLName  xml.Name `xml:"svg"`
	Xmlns    string   `xml:"xmlns,attr"`
	Width    string   `xml:"width,attr,omitempty"`
	Height   string   `xml:"height,attr,omitempty"`
	ViewBox  string   `xml:"viewBox,attr,omitempty"`
	Fonts    []*Font  `xml:"defs>font"`
	Masks    []*Mask  `xml:"defs>mask"`
	Groups   []*Group `xml:"g"`
	FontMode FontMode `xml:"-"`

	// SelectableText configures SelectableTextMode. It is ignored by the
	// other font modes. Leave as zero value for DefaultSelectableTextOptions.
	SelectableText SelectableTextOptions `xml:"-"`

	FillStroke
}

func NewSvg() *Svg {
	return &Svg{
		Xmlns:      "http://www.w3.org/2000/svg",
		FillStroke: FillStroke{Fill: "none", Stroke: "none"},
		FontMode:   PathFontMode,
	}
}

type Group struct {
	FillStroke
	Transform       string            `xml:"-"`
	Groups          []*Group          `xml:"-"`
	Paths           []*Path           `xml:"-"`
	Texts           []*Text           `xml:"-"`
	SelectableTexts []*SelectableText `xml:"-"`
	Image           *Image            `xml:"-"`
	Mask            string            `xml:"-"`
}

// MarshalXML emits the group preserving field order: nested groups, paths,
// texts, image. Both Text and SelectableText serialize as <text> elements;
// a group only ever contains one of the two kinds.
func (g *Group) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "g"}
	if g.Transform != "" {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "transform"}, Value: g.Transform})
	}
	if g.Mask != "" {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "mask"}, Value: g.Mask})
	}
	g.FillStroke.appendAttrs(&start.Attr)

	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, sub := range g.Groups {
		if err := e.EncodeElement(sub, xml.StartElement{Name: xml.Name{Local: "g"}}); err != nil {
			return err
		}
	}
	for _, p := range g.Paths {
		if err := e.EncodeElement(p, xml.StartElement{Name: xml.Name{Local: "path"}}); err != nil {
			return err
		}
	}
	for _, t := range g.Texts {
		if err := e.EncodeElement(t, xml.StartElement{Name: xml.Name{Local: "text"}}); err != nil {
			return err
		}
	}
	for _, t := range g.SelectableTexts {
		if err := e.EncodeElement(t, xml.StartElement{Name: xml.Name{Local: "text"}}); err != nil {
			return err
		}
	}
	if g.Image != nil {
		if err := e.EncodeElement(g.Image, xml.StartElement{Name: xml.Name{Local: "image"}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

type Path struct {
	FillStroke
	Desc string `xml:"d,attr"`
}

type Text struct {
	FillStroke
	Position
	FontSize   float64 `xml:"font-size,attr,omitempty"`
	FontFamily string  `xml:"font-family,attr,omitempty"`
	Text       string  `xml:",innerxml"`
	Style      string  `xml:"style,attr,omitempty"`
}

// TSpan represents one line of a SelectableText.
type TSpan struct {
	// Text is the sanitized UTF-8 content of the line.
	Text string `xml:",chardata"`
	// X resets the horizontal cursor at the start of the line; it is only
	// emitted on lines after the first.
	X float64 `xml:"x,attr,omitempty"`
	// DY is the relative baseline shift of the line; it is only emitted on
	// lines after the first.
	DY string `xml:"dy,attr,omitempty"`
}

// SelectableText is a <text> element whose content is real UTF-8 text, so
// browsers can select and search it. Multi-line strings are emitted as
// <tspan> children. Unlike Text, the content is escaped on marshaling.
type SelectableText struct {
	FillStroke
	Position
	FontSize   float64  `xml:"font-size,attr,omitempty"`
	FontFamily string   `xml:"font-family,attr,omitempty"`
	FontStyle  string   `xml:"font-style,attr,omitempty"`
	FontWeight string   `xml:"font-weight,attr,omitempty"`
	Style      string   `xml:"style,attr,omitempty"`
	Lines      []*TSpan `xml:"tspan"`
	// SingleLine is emitted directly as escaped character data when the
	// string has no line break.
	SingleLine string `xml:",chardata"`
}

// MarshalXML emits either escaped character data (single line) or tspan
// children (multiple lines), never both.
func (t *SelectableText) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "text"}
	start.Attr = append(start.Attr,
		xml.Attr{Name: xml.Name{Local: "x"}, Value: formatSvgFloat(t.X)},
		xml.Attr{Name: xml.Name{Local: "y"}, Value: formatSvgFloat(t.Y)},
	)
	if t.FontSize > 0 {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "font-size"},
			Value: formatSvgFloat(t.FontSize),
		})
	}
	if t.FontFamily != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "font-family"},
			Value: t.FontFamily,
		})
	}
	if t.FontStyle != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "font-style"},
			Value: t.FontStyle,
		})
	}
	if t.FontWeight != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "font-weight"},
			Value: t.FontWeight,
		})
	}
	if t.Style != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "style"},
			Value: t.Style,
		})
	}
	// fill / stroke attributes from FillStroke
	t.FillStroke.appendAttrs(&start.Attr)

	if len(t.Lines) == 0 {
		if err := e.EncodeToken(start); err != nil {
			return err
		}
		if err := e.EncodeToken(xml.CharData([]byte(t.SingleLine))); err != nil {
			return err
		}
		return e.EncodeToken(start.End())
	}

	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for i, line := range t.Lines {
		lineStart := xml.StartElement{Name: xml.Name{Local: "tspan"}}
		if i > 0 {
			lineStart.Attr = append(lineStart.Attr,
				xml.Attr{Name: xml.Name{Local: "x"}, Value: formatSvgFloat(t.X)},
				xml.Attr{Name: xml.Name{Local: "dy"}, Value: line.DY},
			)
		}
		if err := e.EncodeToken(lineStart); err != nil {
			return err
		}
		if err := e.EncodeToken(xml.CharData([]byte(line.Text))); err != nil {
			return err
		}
		if err := e.EncodeToken(lineStart.End()); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// appendAttrs appends the non-empty fill/stroke attributes.
func (fs FillStroke) appendAttrs(attrs *[]xml.Attr) {
	add := func(name, value string) {
		if value != "" {
			*attrs = append(*attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	add("fill", fs.Fill)
	add("fill-rule", fs.FillRule)
	add("stroke", fs.Stroke)
	add("stroke-width", fs.StrokeWidth)
	add("stroke-linecap", fs.StrokeLinecap)
	add("stroke-linejoin", fs.StrokeLinejoin)
	add("stroke-dasharray", fs.StrokeDasharray)
	add("stroke-dashoffset", fs.StrokeDashoffset)
}

// formatSvgFloat formats a coordinate like the rest of the svg output.
func formatSvgFloat(v float64) string {
	return optiSprintf("%f", v)
}

type Image struct {
	Position
	Dimension
	Href string `xml:"href,attr"`
}

type Mask struct {
	Identity
	Position
	Dimension
}

type Rect struct {
	Position
	Dimension
	FillStroke
}

func (m Mask) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	bigRect := Rect{}
	bigRect.X, bigRect.Y = 0, 0
	bigRect.Width, bigRect.Height = "100%", "100%"
	bigRect.Fill = "#fff"
	rect := Rect{}
	rect.X, rect.Y = m.X, m.Y
	rect.Width, rect.Height = m.Width, m.Height
	rect.Fill = "#000"

	return e.EncodeElement(struct {
		XMLName xml.Name `xml:"mask"`
		Rects   [2]Rect  `xml:"rect"`
		Id      string   `xml:"id,attr"`
	}{
		Rects: [2]Rect{bigRect, rect},
		Id:    m.Id,
	}, start)
}

/* font related elements */

type Font struct {
	Identity
	Face   *Face    `xml:"font-face"`
	Glyphs []*Glyph `xml:"glyph"`
}

type Face struct {
	Family    string  `xml:"font-family,attr"`
	Units     int     `xml:"units-per-em,attr"`
	HorizAdvX float64 `xml:"horiz-adv-x,attr"`
	// TODO add other attrs, like style, variant, weight...
}

type Glyph struct {
	Rune      Rune    `xml:"unicode,attr"`
	Desc      string  `xml:"d,attr"`
	HorizAdvX float64 `xml:"horiz-adv-x,attr"`
}

type Rune rune

func (r Rune) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{
		Name:  name,
		Value: string(rune(r)),
	}, nil
}

/* shared attrs */

type Identity struct {
	Id   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

type Position struct {
	X float64 `xml:"x,attr,omitempty"`
	Y float64 `xml:"y,attr,omitempty"`
}

type Dimension struct {
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
}

type FillStroke struct {
	Fill     string `xml:"fill,attr,omitempty"`
	FillRule string `xml:"fill-rule,attr,omitempty"`

	Stroke           string `xml:"stroke,attr,omitempty"`
	StrokeWidth      string `xml:"stroke-width,attr,omitempty"`
	StrokeLinecap    string `xml:"stroke-linecap,attr,omitempty"`
	StrokeLinejoin   string `xml:"stroke-linejoin,attr,omitempty"`
	StrokeDasharray  string `xml:"stroke-dasharray,attr,omitempty"`
	StrokeDashoffset string `xml:"stroke-dashoffset,attr,omitempty"`
}
