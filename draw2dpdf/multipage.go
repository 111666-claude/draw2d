// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf

import (
	"errors"
	"io"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
)

// Errors returned by the multi page GraphicContext lifecycle.
var (
	// ErrClosed is returned when a page is added or the document is
	// finished after the document has been closed.
	ErrClosed = errors.New("draw2dpdf: document is already closed")
	// ErrInvalidPageSize is returned when a page width or height is zero
	// or negative.
	ErrInvalidPageSize = errors.New("draw2dpdf: page width and height must be greater than zero")
	// ErrInvalidOrientation is returned when a page orientation is
	// neither portrait ("P") nor landscape ("L").
	ErrInvalidOrientation = errors.New("draw2dpdf: orientation must be \"P\" (portrait) or \"L\" (landscape)")
	// ErrPageTooLarge is returned when a page exceeds the maximum PDF
	// page size of 14400 points (200 inch) in either dimension.
	ErrPageTooLarge = errors.New("draw2dpdf: page dimensions must not exceed 14400 points (200 inch)")
	// ErrInvalidDPI is returned when a non positive DPI is configured.
	ErrInvalidDPI = errors.New("draw2dpdf: DPI must be greater than zero")
	// ErrUnbalancedSave is returned when a page ends with unclosed Save
	// calls.
	ErrUnbalancedSave = errors.New("draw2dpdf: Save without a matching Restore before ending the page")
)

// maxPageDimension is the PDF user space limit (14400 points = 200 inch).
const maxPageDimension = 14400.0

// PageSize configures the width and height of a page in the document unit
// (see NewMultiPagePdf). A zero value PageSize passed to AddPage means
// "use the document default size".
type PageSize struct {
	Width  float64
	Height float64
}

// PageDocument owns a gofpdf document and its MultiPageGraphicContext. It
// is the entry point of the multi page lifecycle.
type PageDocument struct {
	pdf            *gofpdf.Fpdf
	gc             *MultiPageGraphicContext
	defOrientation string
}

// MultiPageGraphicContext is a GraphicContext with an explicit multi page
// lifecycle. New pages are started with PageDocument.AddPage and the
// document is finalized with PageDocument.Close or Output.
//
// Font (font data and size), colors, line style (width, cap, join and
// dash pattern) and the DPI are retained when a new page is started. The
// current path and the page transformation are reset: every page starts
// with an identity transformation and an empty path, so Save/Restore,
// Translate/Rotate/Scale, Clear/ClearRect and drawn images only affect
// the page they are issued on.
//
// The embedded GraphicContext keeps supporting the single page API
// (NewPdf/NewGraphicContext) unchanged when used directly.
type MultiPageGraphicContext struct {
	*GraphicContext
	doc *PageDocument
	// pageCount is the number of pages added so far.
	pageCount int
	// saveDepth counts the Save calls on the current page that have not
	// been closed with Restore.
	saveDepth int
	closed    bool
}

// NewMultiPagePdf creates a new multi page pdf document. orientationStr
// is the default orientation ("P" or "L"), unitStr is the measurement
// unit ("pt", "mm", "cm" or "in") and sizeStr is the default page size
// ("A4", "Letter", ...). No page is added until AddPage is called.
//
// An error is returned for an unknown unit, size or orientation.
func NewMultiPagePdf(orientationStr, unitStr, sizeStr string) (*PageDocument, error) {
	if err := validateOrientation(orientationStr); err != nil {
		return nil, err
	}
	pdf := gofpdf.New(orientationStr, unitStr, sizeStr, draw2d.GetFontFolder())
	if pdf.Err() {
		return nil, pdf.Error()
	}
	return newPageDocument(pdf, orientationStr), nil
}

// NewMultiPagePdfWithSize creates a new multi page pdf document with a
// custom default page size (width and height in unitStr units). The other
// parameters behave as in NewMultiPagePdf.
func NewMultiPagePdfWithSize(orientationStr, unitStr string, width, height float64) (*PageDocument, error) {
	if err := validateOrientation(orientationStr); err != nil {
		return nil, err
	}
	if err := validatePageSize(width, height); err != nil {
		return nil, err
	}
	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		OrientationStr: orientationStr,
		UnitStr:        unitStr,
		FontDirStr:     draw2d.GetFontFolder(),
		Size:           gofpdf.SizeType{Wd: width, Ht: height},
	})
	if pdf.Err() {
		return nil, pdf.Error()
	}
	if err := validatePagePointLimit(pdf, width, height); err != nil {
		return nil, err
	}
	return newPageDocument(pdf, orientationStr), nil
}

func newPageDocument(pdf *gofpdf.Fpdf, orientation string) *PageDocument {
	// Same draw2d compatible defaults as NewPdf.
	pdf.SetMargins(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetLineCapStyle("round")
	pdf.SetLineJoinStyle("round")
	pdf.SetLineWidth(1)
	// Disable automatic page breaks: pages are managed explicitly through
	// AddPage, otherwise gofpdf could insert pages behind the back of the
	// lifecycle and invalidate the page bookkeeping.
	pdf.SetAutoPageBreak(false, 0)
	// Uncompressed content streams keep the per page drawing order
	// inspectable.
	pdf.SetCompression(false)
	doc := &PageDocument{pdf: pdf, defOrientation: normalizeOrientation(orientation)}
	doc.gc = &MultiPageGraphicContext{
		GraphicContext: NewGraphicContext(pdf),
		doc:            doc,
	}
	return doc
}

// normalizeOrientation reduces the accepted orientation spellings to the
// single letter form gofpdf stores internally.
func normalizeOrientation(orientation string) string {
	if orientation == "" {
		return "P"
	}
	return strings.ToUpper(orientation[:1])
}

// GraphicContext returns the draw2d GraphicContext bound to the document.
// It must not be used to draw before the first successful AddPage call.
func (doc *PageDocument) GraphicContext() *MultiPageGraphicContext {
	return doc.gc
}

// Pdf exposes the underlying gofpdf document. It is intended for read
// only inspection (PageNo, Error, ...); driving pages directly bypasses
// the lifecycle bookkeeping.
func (doc *PageDocument) Pdf() *gofpdf.Fpdf {
	return doc.pdf
}

// PageCount returns the number of pages added so far.
func (doc *PageDocument) PageCount() int {
	return doc.gc.pageCount
}

// AddPage starts a new page using the document default size and
// orientation. See AddPageWithSize for the lifecycle semantics.
func (doc *PageDocument) AddPage() error {
	return doc.AddPageWithSize("", PageSize{})
}

// AddPageWithSize starts a new page with an explicit orientation ("P" or
// "L") and size. An empty orientation or a zero value size falls back to
// the document defaults.
//
// Font, font size, colors, line style and DPI are carried over from the
// previous page, while the current path, transformation stack and Save
// depth are reset. Ending a page with unclosed Save calls returns
// ErrUnbalancedSave and no new page is started. Adding a page after Close
// returns ErrClosed.
func (doc *PageDocument) AddPageWithSize(orientation string, size PageSize) error {
	gc := doc.gc
	if gc.closed {
		return ErrClosed
	}
	if err := gc.pdf.Error(); err != nil {
		return err
	}
	if gc.pageCount > 0 && gc.saveDepth != 0 {
		return ErrUnbalancedSave
	}
	if orientation == "" {
		orientation = doc.defOrientation
	}
	if orientation != "" {
		if err := validateOrientation(orientation); err != nil {
			return err
		}
	}
	if size.Width != 0 || size.Height != 0 {
		// Both dimensions must be provided together: a partial size is
		// rejected rather than silently producing a zero sized page.
		if err := validatePageSize(size.Width, size.Height); err != nil {
			return err
		}
		if err := validatePagePointLimit(gc.pdf, size.Width, size.Height); err != nil {
			return err
		}
	}
	pdfSize := gofpdf.SizeType{Wd: size.Width, Ht: size.Height}
	if size.Width != 0 && size.Height != 0 {
		// gofpdf stores page sizes in portrait orientation and swaps
		// the dimensions for landscape pages; accept Width/Height as the
		// visible dimensions and convert accordingly.
		if orientation == "L" {
			pdfSize.Wd, pdfSize.Ht = size.Height, size.Width
		}
	}
	if orientation == doc.defOrientation && size.Width == 0 && size.Height == 0 {
		gc.pdf.AddPage()
	} else {
		gc.pdf.AddPageFormat(orientation, pdfSize)
	}
	if err := gc.pdf.Error(); err != nil {
		return err
	}
	gc.pageCount++
	gc.saveDepth = 0
	// Reset the draw2d side of the page state. The gofpdf side (font,
	// colors, line width, cap, join, dash) is restored by AddPageFormat.
	gc.resetPageState()
	return nil
}

// resetPageState clears the per page draw2d state: path, transformation
// and Save stack. Persistent state (font, colors, line style, DPI) is
// taken from the currently active context and carried onto the new page.
func (gc *MultiPageGraphicContext) resetPageState() {
	// Every page must start outside a transformation (gofpdf ends the
	// previous page and opens the new one without any q/Q context), so
	// collapse the whole Save stack onto a single fresh context which
	// keeps only the persistent graphic state.
	prev := gc.StackGraphicContext.Current
	next := &draw2dbase.ContextStack{}
	next.LineWidth = prev.LineWidth
	next.Dash = prev.Dash
	next.DashOffset = prev.DashOffset
	next.StrokeColor = prev.StrokeColor
	next.FillColor = prev.FillColor
	next.FillRule = prev.FillRule
	next.Cap = prev.Cap
	next.Join = prev.Join
	next.FontSize = prev.FontSize
	next.FontData = prev.FontData
	next.Font = prev.Font
	next.Scale = prev.Scale
	next.Tr = draw2d.NewIdentityMatrix()
	next.Path = new(draw2d.Path)
	gc.StackGraphicContext.Current = next
}

// SetDPI sets the DPI which influences the font size. The DPI is retained
// across pages. A non positive value returns ErrInvalidDPI.
func (doc *PageDocument) SetDPI(dpi int) error {
	if dpi <= 0 {
		return ErrInvalidDPI
	}
	doc.gc.SetDPI(dpi)
	return nil
}

// Save wraps GraphicContext.Save and tracks the nesting depth so that
// unclosed transformations can be reported when the page ends.
func (gc *MultiPageGraphicContext) Save() {
	gc.GraphicContext.Save()
	gc.saveDepth++
}

// Restore wraps GraphicContext.Restore and tracks the nesting depth.
func (gc *MultiPageGraphicContext) Restore() {
	if gc.saveDepth == 0 {
		// Mirror gofpdf behavior and surface the error on the document.
		gc.pdf.SetError(errors.New("draw2dpdf: Restore without a matching Save"))
		return
	}
	gc.GraphicContext.Restore()
	gc.saveDepth--
}

// Close finalizes the document. Empty pages are valid and kept as is.
// Closing twice returns ErrClosed without rewriting anything. A document
// on which no page was ever added gets a single empty page so that the
// resulting PDF is valid.
func (doc *PageDocument) Close() error {
	gc := doc.gc
	if gc.closed {
		return ErrClosed
	}
	if gc.saveDepth != 0 {
		return ErrUnbalancedSave
	}
	gc.closed = true
	if gc.pageCount == 0 {
		// gofpdf itself appends an empty page on Close when none exists;
		// keep the bookkeeping in sync.
		gc.pageCount = 1
	}
	return gc.pdf.Error()
}

// Output writes the finalized pdf document to w. Close is called first
// when it has not been called yet, so an explicit Close followed by
// Output (or repeated Output calls) is supported. See Close for empty
// page and error semantics.
func (doc *PageDocument) Output(w io.Writer) error {
	if !doc.gc.closed {
		if err := doc.Close(); err != nil {
			return err
		}
	} else if doc.gc.saveDepth != 0 {
		return ErrUnbalancedSave
	}
	return doc.pdf.Output(w)
}

// SaveToFile writes the finalized pdf document to filePath.
func (doc *PageDocument) SaveToFile(filePath string) error {
	if !doc.gc.closed {
		if err := doc.Close(); err != nil {
			return err
		}
	}
	return doc.pdf.OutputFileAndClose(filePath)
}

func validateOrientation(orientation string) error {
	switch orientation {
	case "P", "L":
		return nil
	default:
		return ErrInvalidOrientation
	}
}

// validatePageSize checks the dimensions expressed in document units,
// before any point conversion.
func validatePageSize(width, height float64) error {
	if width <= 0 || height <= 0 {
		return ErrInvalidPageSize
	}
	return nil
}

// validatePagePointLimit converts the requested size (in document units)
// to points and applies the PDF 14400 point user space limit.
func validatePagePointLimit(pdf *gofpdf.Fpdf, width, height float64) error {
	if pdf.UnitToPointConvert(width) > maxPageDimension ||
		pdf.UnitToPointConvert(height) > maxPageDimension {
		return ErrPageTooLarge
	}
	return nil
}
