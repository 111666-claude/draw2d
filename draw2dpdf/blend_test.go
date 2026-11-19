// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dpdf_test

import (
	"errors"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dpdf"
)

func TestPdfBlendModeUnsupported(t *testing.T) {
	gc := draw2dpdf.NewGraphicContext(draw2dpdf.NewPdf("L", "mm", "A4"))

	// Normal is the historical behaviour and is accepted.
	if err := gc.SetBlendMode(draw2d.BlendNormal); err != nil {
		t.Errorf("SetBlendMode(normal) = %v, want nil", err)
	}
	// Unknown modes share the cross-backend error.
	if err := gc.SetBlendMode(draw2d.BlendMode(7)); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("SetBlendMode(invalid) = %v, want ErrInvalidBlendMode", err)
	}
	// Blending itself is explicitly unsupported rather than silently ignored.
	if err := gc.SetBlendMode(draw2d.BlendMultiply); !errors.Is(err, draw2d.ErrBlendUnsupported) {
		t.Errorf("SetBlendMode(multiply) = %v, want ErrBlendUnsupported", err)
	}
	if got := gc.GetBlendMode(); got != draw2d.BlendNormal {
		t.Errorf("rejected mode changed state to %v", got)
	}
}

func TestPdfBeginLayerUnsupported(t *testing.T) {
	gc := draw2dpdf.NewGraphicContext(draw2dpdf.NewPdf("L", "mm", "A4"))

	if err := gc.BeginLayer(draw2d.BlendNormal, 1); !errors.Is(err, draw2d.ErrBlendUnsupported) {
		t.Errorf("BeginLayer = %v, want ErrBlendUnsupported", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMode(7), 1); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("BeginLayer invalid mode = %v, want ErrInvalidBlendMode", err)
	}
	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("EndLayer = %v, want ErrLayerNotOpen", err)
	}
}
