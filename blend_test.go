// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2d_test

import (
	"errors"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

func TestBlendModeString(t *testing.T) {
	cases := []struct {
		mode draw2d.BlendMode
		want string
	}{
		{draw2d.BlendNormal, "normal"},
		{draw2d.BlendMultiply, "multiply"},
		{draw2d.BlendScreen, "screen"},
		{draw2d.BlendMode(42), "BlendMode(42)"},
		{draw2d.BlendMode(-1), "BlendMode(-1)"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("BlendMode(%d).String() = %q, want %q", int(c.mode), got, c.want)
		}
	}
}

func TestBlendModeIsValid(t *testing.T) {
	for _, m := range []draw2d.BlendMode{draw2d.BlendNormal, draw2d.BlendMultiply, draw2d.BlendScreen} {
		if !m.IsValid() {
			t.Errorf("%v.IsValid() = false, want true", m)
		}
	}
	for _, m := range []draw2d.BlendMode{draw2d.BlendMode(3), draw2d.BlendMode(-1), draw2d.BlendMode(1000)} {
		if m.IsValid() {
			t.Errorf("%v.IsValid() = true, want false", m)
		}
	}
}

func TestNormalizeOpacity(t *testing.T) {
	cases := []struct {
		in      float64
		want    float64
		wantErr error
	}{
		{0, 0, nil},
		{0.25, 0.25, nil},
		{1, 1, nil},
		{-0.5, 0, nil},
		{1.5, 1, nil},
		{2, 1, nil},
		{math.NaN(), 0, draw2d.ErrInvalidOpacity},
		{math.Inf(1), 0, draw2d.ErrInvalidOpacity},
		{math.Inf(-1), 0, draw2d.ErrInvalidOpacity},
	}
	for _, c := range cases {
		got, err := draw2d.NormalizeOpacity(c.in)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("NormalizeOpacity(%v) error = %v, want %v", c.in, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeOpacity(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestBlendErrorsDistinct(t *testing.T) {
	// Every documented error is distinct, so callers can tell them apart with
	// errors.Is instead of string matching.
	errs := []error{
		draw2d.ErrInvalidBlendMode,
		draw2d.ErrInvalidOpacity,
		draw2d.ErrLayerTooLarge,
		draw2d.ErrLayerNotOpen,
		draw2d.ErrLayerDepthExceeded,
		draw2d.ErrEmptyCanvas,
		draw2d.ErrBlendUnsupported,
	}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("error %v matches unrelated %v", a, b)
			}
		}
	}
}

func TestDefaultMaxLayerPixels(t *testing.T) {
	if draw2d.DefaultMaxLayerPixels != 64<<20 {
		t.Errorf("DefaultMaxLayerPixels = %d, want %d", draw2d.DefaultMaxLayerPixels, 64<<20)
	}
}
