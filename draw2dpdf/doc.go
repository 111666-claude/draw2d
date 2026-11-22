// Copyright 2015 The draw2d Authors. All rights reserved.
// created: 26/06/2015 by Stani Michiels

// Package draw2dpdf provides a graphic context that can draw vector
// graphics and text on pdf file with the gofpdf package.
//
// Quick Start
//
// The following Go code generates a simple drawing and saves it to a
// pdf document:
//   // Initialize the graphic context on an RGBA image
//   dest := draw2dpdf.NewPdf("L", "mm", "A4")
//   gc := draw2dpdf.NewGraphicContext(dest)
//
//   // Set some properties
//   gc.SetFillColor(color.RGBA{0x44, 0xff, 0x44, 0xff})
//   gc.SetStrokeColor(color.RGBA{0x44, 0x44, 0x44, 0xff})
//   gc.SetLineWidth(5)
//
//   // Draw a closed shape
//   gc.MoveTo(10, 10) // should always be called first for a new path
//   gc.LineTo(100, 50)
//   gc.QuadCurveTo(100, 10, 10, 10)
//   gc.Close()
//   gc.FillStroke()
//
//   // Save to file
//   draw2dpdf.SaveToPdfFile("hello.pdf", dest)
//
// Multi page documents
//
// Reports that span several pages use the PageDocument lifecycle instead
// of NewPdf:
//   doc, _ := draw2dpdf.NewMultiPagePdf("P", "mm", "A4")
//   gc := doc.GraphicContext()
//   for page := 0; page < 3; page++ {
//       if err := doc.AddPage(); err != nil {
//           log.Fatal(err)
//       }
//       // draw on gc: font, colors, line style and DPI carry over,
//       // while the path and page transformation start fresh.
//   }
//   doc.SaveToFile("report.pdf")
//
// AddPageWithSize selects a per page size and orientation. Save/Restore,
// Translate/Rotate/Scale, Clear/ClearRect and images only affect the
// current page. Closing a document with an unclosed Save, an illegal page
// size, an oversized page or a non positive DPI returns a typed error.
//
// There are more examples here:
// https://github.com/llgcode/draw2d/tree/master/samples
//
// Alternative backends
//
// Drawing on images is provided by the draw2d package.
// Drawing on opengl is provided by the draw2dgl package.
//
// Acknowledgments
//
// The pdf backend uses https://github.com/jung-kurt/gofpdf
package draw2dpdf
