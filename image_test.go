// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d

import (
	"errors"
	"image"
	"math"
	"testing"
)

// TestImageRectMatrixMapping checks that the computed matrix maps the
// source rectangle exactly onto the destination rectangle.
func TestImageRectMatrixMapping(t *testing.T) {
	srcBounds := image.Rect(0, 0, 16, 16)
	srcRect := image.Rect(4, 8, 8, 12)
	dstRect := image.Rect(20, 30, 28, 38)

	m, sr, err := ImageRectMatrix(srcBounds, srcRect, dstRect, NewIdentityMatrix())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr != srcRect {
		t.Errorf("clipped source rect = %v, want %v", sr, srcRect)
	}
	for _, pt := range [][2]image.Point{
		{srcRect.Min, dstRect.Min},
		{srcRect.Max, dstRect.Max},
		{{srcRect.Min.X, srcRect.Max.Y}, {dstRect.Min.X, dstRect.Max.Y}},
		{{srcRect.Max.X, srcRect.Min.Y}, {dstRect.Max.X, dstRect.Min.Y}},
	} {
		x, y := m.TransformPoint(float64(pt[0].X), float64(pt[0].Y))
		if x != float64(pt[1].X) || y != float64(pt[1].Y) {
			t.Errorf("source point %v mapped to (%v, %v), want %v", pt[0], x, y, pt[1])
		}
	}
}

// TestImageRectMatrixComposed checks that the computed matrix is the source
// to destination mapping composed with the current transformation.
func TestImageRectMatrixComposed(t *testing.T) {
	srcBounds := image.Rect(0, 0, 16, 16)
	srcRect := image.Rect(4, 8, 8, 12)
	dstRect := image.Rect(20, 30, 28, 38)

	tr := NewScaleMatrix(2, 2)
	tr.Translate(3, 5) // tr = scale(2) then translate by (6, 10) in device space

	m, _, err := ImageRectMatrix(srcBounds, srcRect, dstRect, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reference: apply the srcRect->dstRect mapping, then tr.
	tm := Matrix{
		2, 0, 0, 2,
		20 - 4*2, 30 - 8*2,
	}
	for _, sp := range []image.Point{srcRect.Min, srcRect.Max, {6, 10}} {
		ux, uy := tm.TransformPoint(float64(sp.X), float64(sp.Y))
		wantX, wantY := tr.TransformPoint(ux, uy)
		gotX, gotY := m.TransformPoint(float64(sp.X), float64(sp.Y))
		if gotX != wantX || gotY != wantY {
			t.Errorf("source point %v mapped to (%v, %v), want (%v, %v)", sp, gotX, gotY, wantX, wantY)
		}
	}
}

// TestImageRectMatrixClipping checks that a source rectangle extending
// beyond the image bounds is clipped, while the mapping still refers to the
// original source rectangle.
func TestImageRectMatrixClipping(t *testing.T) {
	srcBounds := image.Rect(0, 0, 16, 16)
	srcRect := image.Rect(12, 12, 20, 20)
	dstRect := image.Rect(0, 0, 16, 16)

	m, sr, err := ImageRectMatrix(srcBounds, srcRect, dstRect, NewIdentityMatrix())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := image.Rect(12, 12, 16, 16); sr != want {
		t.Errorf("clipped source rect = %v, want %v", sr, want)
	}
	// The mapping must still map the full (unclipped) srcRect onto dstRect.
	x, y := m.TransformPoint(12, 12)
	if x != 0 || y != 0 {
		t.Errorf("srcRect.Min mapped to (%v, %v), want (0, 0)", x, y)
	}
	x, y = m.TransformPoint(20, 20)
	if x != 16 || y != 16 {
		t.Errorf("srcRect.Max mapped to (%v, %v), want (16, 16)", x, y)
	}
}

// TestImageRectMatrixNonZeroBounds checks source images whose bounds do not
// start at the origin.
func TestImageRectMatrixNonZeroBounds(t *testing.T) {
	srcBounds := image.Rect(5, 7, 13, 15)
	srcRect := image.Rect(6, 8, 9, 12)
	dstRect := image.Rect(0, 0, 6, 8)

	m, sr, err := ImageRectMatrix(srcBounds, srcRect, dstRect, NewIdentityMatrix())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr != srcRect {
		t.Errorf("clipped source rect = %v, want %v", sr, srcRect)
	}
	x, y := m.TransformPoint(6, 8)
	if x != 0 || y != 0 {
		t.Errorf("srcRect.Min mapped to (%v, %v), want (0, 0)", x, y)
	}
	x, y = m.TransformPoint(9, 12)
	if x != 6 || y != 8 {
		t.Errorf("srcRect.Max mapped to (%v, %v), want (6, 8)", x, y)
	}
}

// TestImageRectMatrixErrors checks the validation of invalid geometry and
// transformations.
func TestImageRectMatrixErrors(t *testing.T) {
	bounds := image.Rect(0, 0, 16, 16)
	rect := image.Rect(0, 0, 4, 4)
	nan := NewIdentityMatrix()
	nan[0] = math.NaN()
	inf := NewIdentityMatrix()
	inf[5] = math.Inf(1)

	tests := []struct {
		name    string
		srcRect image.Rectangle
		dstRect image.Rectangle
		tr      Matrix
		wantErr error
	}{
		{"empty source", image.Rectangle{}, rect, NewIdentityMatrix(), ErrEmptySourceRect},
		{"degenerate source", image.Rect(4, 4, 4, 8), rect, NewIdentityMatrix(), ErrEmptySourceRect},
		{"source outside", image.Rect(100, 100, 104, 104), rect, NewIdentityMatrix(), ErrSourceRectOutside},
		{"negative dest width", rect, image.Rectangle{Min: image.Pt(10, 0), Max: image.Pt(0, 4)}, NewIdentityMatrix(), ErrNegativeDestRect},
		{"negative dest height", rect, image.Rectangle{Min: image.Pt(0, 10), Max: image.Pt(4, 0)}, NewIdentityMatrix(), ErrNegativeDestRect},
		{"NaN matrix", rect, rect, nan, ErrNonFiniteMatrix},
		{"infinite matrix", rect, rect, inf, ErrNonFiniteMatrix},
		{"huge scale dest", image.Rect(0, 0, 1, 1), image.Rect(0, 0, 100000, 4), NewIdentityMatrix(), ErrImageScaleTooLarge},
		{"huge scale transform", rect, rect, NewScaleMatrix(1e6, 1), ErrImageScaleTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ImageRectMatrix(bounds, tt.srcRect, tt.dstRect, tt.tr)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestImageRectMatrixNoOp checks the cases that must draw nothing without
// failing: zero-sized destination and singular transformation.
func TestImageRectMatrixNoOp(t *testing.T) {
	bounds := image.Rect(0, 0, 16, 16)
	srcRect := image.Rect(0, 0, 4, 4)

	for _, tt := range []struct {
		name    string
		dstRect image.Rectangle
		tr      Matrix
	}{
		{"zero width dest", image.Rect(4, 0, 4, 4), NewIdentityMatrix()},
		{"zero height dest", image.Rect(0, 4, 4, 4), NewIdentityMatrix()},
		{"zero dest", image.Rect(4, 4, 4, 4), NewIdentityMatrix()},
		{"singular matrix", image.Rect(0, 0, 4, 4), NewScaleMatrix(0, 0)},
		{"flat matrix", image.Rect(0, 0, 4, 4), NewScaleMatrix(1, 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, sr, err := ImageRectMatrix(bounds, srcRect, tt.dstRect, tt.tr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sr.Empty() {
				t.Errorf("expected empty (no-op) source rect, got %v", sr)
			}
		})
	}
}
