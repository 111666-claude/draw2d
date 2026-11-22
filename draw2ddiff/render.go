package draw2ddiff

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dpdf"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// fixedPDFDate pins the PDF creation date so PDF output is byte-stable.
var fixedPDFDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// FontError marks a render failure caused by an unavailable pinned font.
// The gate classifies it as CategoryFontPlatform.
type FontError struct {
	Font draw2d.FontData
	Err  error
}

func (e *FontError) Error() string {
	return fmt.Sprintf("pinned font %+v unavailable: %v", e.Font, e.Err)
}

// FontFolder returns the absolute path of the bundled resource/font
// directory, resolved relative to this source file so tests work from
// any working directory.
func FontFolder() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "resource", "font")
}

// nonFiniteTimeout bounds how long a backend may take on non-finite
// input before the outcome is classified as "did not terminate". Some
// rasterizers loop (nearly) forever on NaN coordinates; the timeout
// turns that into a stable, comparable classification.
const nonFiniteTimeout = 3 * time.Second

// Render draws the scene on the given backend and returns the comparable
// snapshot. Panics from a backend are recovered and returned as errors so
// a crashing backend is classified, not fatal to the test run. Non-finite
// scenes are additionally bounded by nonFiniteTimeout.
func Render(scene *Scene, backend Backend) (snap *Snapshot, err error) {
	if scene.Kind == KindNonFinite {
		return renderNonFinite(scene, backend)
	}
	return render(scene, backend)
}

// renderNonFinite renders a non-finite scene with a timeout so a
// non-terminating backend gets a stable classification instead of
// hanging the gate.
func renderNonFinite(scene *Scene, backend Backend) (*Snapshot, error) {
	type outcome struct {
		snap *Snapshot
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		snap, err := render(scene, backend)
		ch <- outcome{snap, err}
	}()
	select {
	case o := <-ch:
		return o.snap, o.err
	case <-time.After(nonFiniteTimeout):
		return nil, fmt.Errorf("backend %s did not terminate on non-finite input within %s", backend, nonFiniteTimeout)
	}
}

func render(scene *Scene, backend Backend) (snap *Snapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			snap = nil
			err = fmt.Errorf("backend %s panicked on scene %s: %v", backend, scene.Name, r)
		}
	}()
	if scene.Font != nil {
		if _, ferr := draw2d.GetGlobalFontCache().Load(*scene.Font); ferr != nil {
			return nil, &FontError{Font: *scene.Font, Err: ferr}
		}
	}
	switch backend {
	case BackendPNG:
		return renderPNG(scene)
	case BackendSVG:
		return renderSVG(scene)
	case BackendPDF:
		return renderPDF(scene)
	default:
		return nil, fmt.Errorf("unknown backend %q", backend)
	}
}

func renderPNG(scene *Scene) (*Snapshot, error) {
	img := image.NewRGBA(image.Rect(0, 0, CanvasWidth, CanvasHeight))
	gc := draw2dimg.NewGraphicContext(img)
	scene.Draw(gc)
	return NewRasterSnapshot(scene.Name, img), nil
}

func renderSVG(scene *Scene) (*Snapshot, error) {
	svg := draw2dsvg.NewSvg()
	svg.Width = fmt.Sprintf("%d", CanvasWidth)
	svg.Height = fmt.Sprintf("%d", CanvasHeight)
	gc := draw2dsvg.NewGraphicContext(svg)
	scene.Draw(gc)
	canonical, err := canonicalSVG(svg)
	if err != nil {
		return nil, fmt.Errorf("encoding svg: %v", err)
	}
	return NewStructuralSnapshot(scene.Name, BackendSVG, svgCounts(svg), canonical), nil
}

func renderPDF(scene *Scene) (*Snapshot, error) {
	pdf := draw2dpdf.NewPdf("P", "pt", "A4")
	pdf.SetCompression(false)
	pdf.SetCreationDate(fixedPDFDate)
	gc := draw2dpdf.NewGraphicContext(pdf)
	scene.Draw(gc)
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("pdf backend error: %v", err)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %v", err)
	}
	streams, err := pdfContentStreams(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("pdf structure: %v", err)
	}
	canonical, counts := canonicalPDF(streams)
	return NewStructuralSnapshot(scene.Name, BackendPDF, counts, canonical), nil
}

// canonicalSVG marshals the SVG document in a deterministic form.
func canonicalSVG(svg *draw2dsvg.Svg) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")
	if err := enc.Encode(svg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// svgCounts counts SVG elements recursively.
func svgCounts(svg *draw2dsvg.Svg) map[string]int {
	counts := map[string]int{}
	counts["font"] = len(svg.Fonts)
	for _, f := range svg.Fonts {
		counts["glyph"] += len(f.Glyphs)
	}
	counts["mask"] = len(svg.Masks)
	var walk func(g *draw2dsvg.Group)
	walk = func(g *draw2dsvg.Group) {
		counts["g"]++
		counts["path"] += len(g.Paths)
		counts["text"] += len(g.Texts)
		if g.Image != nil {
			counts["image"]++
		}
		for _, sub := range g.Groups {
			walk(sub)
		}
	}
	for _, g := range svg.Groups {
		walk(g)
	}
	return counts
}

// pdfContentStreams extracts the page content streams from an
// uncompressed PDF document.
func pdfContentStreams(doc []byte) ([][]byte, error) {
	objects, err := pdfObjects(doc)
	if err != nil {
		return nil, err
	}
	var contents [][]byte
	for _, obj := range objects {
		header := objHeader(obj)
		if !strings.Contains(header, "/Type /Page") || strings.Contains(header, "/Type /Pages") {
			continue
		}
		refs := contentRefs(header)
		if len(refs) == 0 {
			return nil, fmt.Errorf("page object without /Contents")
		}
		for _, ref := range refs {
			stream, ok := objects[ref]
			if !ok {
				return nil, fmt.Errorf("/Contents references missing object %d", ref)
			}
			data, err := streamData(stream)
			if err != nil {
				return nil, err
			}
			contents = append(contents, data)
		}
	}
	if len(contents) == 0 {
		return nil, fmt.Errorf("no page content stream found")
	}
	return contents, nil
}

// pdfObjects splits a PDF document into its indirect objects, keyed by
// object number.
func pdfObjects(doc []byte) (map[int][]byte, error) {
	objects := map[int][]byte{}
	rest := doc
	for {
		i := bytes.Index(rest, []byte(" obj"))
		if i < 0 {
			break
		}
		// object number and generation are on the line before " obj"
		head := rest[:i]
		line := head
		if j := bytes.LastIndexByte(head, '\n'); j >= 0 {
			line = head[j+1:]
		}
		var num, gen int
		if n, _ := fmt.Sscanf(string(line), "%d %d", &num, &gen); n != 2 {
			return nil, fmt.Errorf("malformed object header near offset %d", len(doc)-len(rest))
		}
		end := bytes.Index(rest[i:], []byte("endobj"))
		if end < 0 {
			return nil, fmt.Errorf("object %d missing endobj", num)
		}
		objects[num] = rest[i+4 : i+end]
		rest = rest[i+end+6:]
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("no indirect objects found")
	}
	return objects, nil
}

// objHeader returns the dictionary part of an object (before any stream).
func objHeader(obj []byte) string {
	if i := bytes.Index(obj, []byte("stream")); i >= 0 {
		obj = obj[:i]
	}
	return string(obj)
}

// contentRefs parses /Contents references (single ref or array).
func contentRefs(header string) []int {
	i := strings.Index(header, "/Contents")
	if i < 0 {
		return nil
	}
	rest := header[i+len("/Contents"):]
	var refs []int
	for _, field := range strings.Fields(rest) {
		if field == "R" && len(refs) > 0 {
			// previous field was the object number
			continue
		}
		var num int
		if _, err := fmt.Sscanf(strings.Trim(field, "[]"), "%d", &num); err == nil && num > 0 {
			refs = append(refs, num)
			continue
		}
		if len(refs) > 0 {
			break
		}
	}
	return refs
}

// streamData extracts the bytes between "stream" and "endstream".
func streamData(obj []byte) ([]byte, error) {
	i := bytes.Index(obj, []byte("stream"))
	if i < 0 {
		return nil, fmt.Errorf("object without stream")
	}
	data := obj[i+6:]
	if len(data) > 0 && data[0] == '\r' {
		data = data[1:]
	}
	if len(data) > 0 && data[0] == '\n' {
		data = data[1:]
	}
	j := bytes.LastIndex(data, []byte("endstream"))
	if j < 0 {
		return nil, fmt.Errorf("stream without endstream")
	}
	return data[:j], nil
}

// pdfOperators are the content-stream operators the gate counts.
var pdfOperators = map[string]string{
	"re": "rect", "m": "moveto", "l": "lineto", "c": "curveto",
	"v": "curveto", "y": "curveto", "h": "closepath",
	"S": "stroke", "s": "stroke", "f": "fill", "F": "fill", "f*": "fill",
	"B": "fillstroke", "B*": "fillstroke", "b": "fillstroke", "b*": "fillstroke",
	"Do": "image", "Tj": "text", "TJ": "text",
	"q": "gsave", "Q": "grestore", "cm": "matrix",
}

// canonicalPDF tokenizes content streams into a canonical, line-oriented
// form and counts drawing operators.
func canonicalPDF(streams [][]byte) ([]byte, map[string]int) {
	counts := map[string]int{}
	var out bytes.Buffer
	for _, stream := range streams {
		for _, tok := range strings.Fields(string(stream)) {
			if kind, ok := pdfOperators[tok]; ok {
				counts[kind]++
			}
			out.WriteString(tok)
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), counts
}

// countsString renders counts in a stable, human-readable form.
func countsString(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, counts[k])
	}
	return strings.Join(parts, " ")
}

// encodePNG encodes an image as PNG, returning nil on failure.
func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
