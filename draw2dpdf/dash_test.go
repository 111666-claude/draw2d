// Copyright 2015 The draw2d Authors. All rights reserved.

package draw2dpdf

import (
	"math"
	"testing"
)

func TestNormalizeDashSolid(t *testing.T) {
	cases := [][]float64{
		nil,
		{},
		{0},
		{0, 0, 0},
	}
	for _, dash := range cases {
		got, offset, dashed, err := normalizeDash(dash, 3)
		if err != nil || dashed || offset != 0 || len(got) != 0 {
			t.Errorf("normalizeDash(%v, 3) = %v, %v, %v, %v; want solid line", dash, got, offset, dashed, err)
		}
	}
}

func TestNormalizeDashOddLength(t *testing.T) {
	got, offset, dashed, err := normalizeDash([]float64{3, 5, 7}, 0)
	if err != nil || !dashed {
		t.Fatalf("unexpected error: %v, dashed: %v", err, dashed)
	}
	want := []float64{3, 5, 7, 3, 5, 7}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if offset != 0 {
		t.Errorf("offset = %v, want 0", offset)
	}
}

func TestNormalizeDashZerosDropped(t *testing.T) {
	got, _, dashed, err := normalizeDash([]float64{4, 0, 2, 0}, 0)
	if err != nil || !dashed {
		t.Fatalf("unexpected error: %v, dashed: %v", err, dashed)
	}
	want := []float64{4, 2}
	if len(got) != len(want) || got[0] != 4 || got[1] != 2 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeDashOffset(t *testing.T) {
	// pattern length is 10
	tests := []struct {
		offset float64
		want   float64
	}{
		{0, 0},
		{3, 3},
		{10, 0},  // full cycle
		{23, 3},  // two cycles + 3
		{-3, 7},  // negative offset shifts forward
		{-10, 0}, // negative full cycle
		{-23, 7},
	}
	for _, tc := range tests {
		_, got, _, err := normalizeDash([]float64{4, 6}, tc.offset)
		if err != nil {
			t.Errorf("offset %v: unexpected error %v", tc.offset, err)
			continue
		}
		if got != tc.want {
			t.Errorf("offset %v normalized to %v, want %v", tc.offset, got, tc.want)
		}
	}
}

func TestNormalizeDashInvalid(t *testing.T) {
	badLengths := [][]float64{
		{math.NaN(), 2},
		{math.Inf(1), 2},
		{math.Inf(-1), 2},
		{-1, 2},
		{2, -0.5},
	}
	for _, dash := range badLengths {
		if _, _, _, err := normalizeDash(dash, 0); err == nil {
			t.Errorf("normalizeDash(%v, 0) expected error", dash)
		}
	}
	badOffsets := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, offset := range badOffsets {
		if _, _, _, err := normalizeDash([]float64{1, 2}, offset); err == nil {
			t.Errorf("offset %v expected error", offset)
		}
	}
}

func TestNormalizeDashTooManyEntries(t *testing.T) {
	dash := make([]float64, maxDashEntries+2)
	for i := range dash {
		dash[i] = 1
	}
	if _, _, _, err := normalizeDash(dash, 0); err == nil {
		t.Error("expected error for overlong dash array")
	}
}
