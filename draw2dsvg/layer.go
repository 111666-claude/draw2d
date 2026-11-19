// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"strings"

	"github.com/llgcode/draw2d"
)

// SetBlendMode sets the blend mode used by subsequent drawing operations.
//
// Every drawing operation is already emitted as its own SVG group, so a
// non-normal mode is written as a mix-blend-mode declaration on that group.
// The mode is part of the saved state and is restored by Restore.
//
// An unknown mode is rejected with draw2d.ErrInvalidBlendMode and leaves the
// current mode unchanged. Clear and ClearRect are destructive operations
// without a style of their own: a non-normal mode does not apply to them and
// they keep their documented reset/mask behaviour.
func (gc *GraphicContext) SetBlendMode(mode draw2d.BlendMode) error {
	if !mode.IsValid() {
		return draw2d.ErrInvalidBlendMode
	}
	return gc.StackGraphicContext.SetBlendMode(mode)
}

// BeginLayer starts an isolated SVG group. Elements drawn while the layer is
// open are accumulated as children of that group, which is appended to the
// output when EndLayer is called with mix-blend-mode and opacity declarations
// taken from the arguments.
//
// The layer behaves like Save: drawing state at the call is restored when the
// layer ends. Inside the layer the blend mode is normal, so its children
// combine with each other; the group as a whole is blended with the backdrop
// by the SVG renderer.
//
// An unknown mode, a NaN or infinite opacity, nesting deeper than
// draw2d.MaxLayerDepth, and a graphic context without an Svg are rejected
// (draw2d.ErrInvalidBlendMode, draw2d.ErrInvalidOpacity,
// draw2d.ErrLayerDepthExceeded and draw2d.ErrBlendUnsupported respectively)
// without changing the context. The SVG backend does not allocate offscreen
// buffers, so the image backend's layer size limit does not apply.
func (gc *GraphicContext) BeginLayer(mode draw2d.BlendMode, opacity float64) error {
	if !mode.IsValid() {
		return draw2d.ErrInvalidBlendMode
	}
	opacity, err := draw2d.NormalizeOpacity(opacity)
	if err != nil {
		return err
	}
	if gc.svg == nil {
		return draw2d.ErrBlendUnsupported
	}
	if len(gc.layers) >= draw2d.MaxLayerDepth {
		return draw2d.ErrLayerDepthExceeded
	}

	gc.Save()
	gc.Current.BlendMode = draw2d.BlendNormal
	gc.layers = append(gc.layers, &svgLayer{
		mode:    mode,
		opacity: opacity,
		group:   &Group{},
	})
	return nil
}

// EndLayer closes the innermost layer and appends its group to the parent
// container. An empty layer produces no element at all. The drawing state
// saved by the matching BeginLayer call is restored.
//
// EndLayer returns draw2d.ErrLayerNotOpen when no layer is open.
func (gc *GraphicContext) EndLayer() error {
	if len(gc.layers) == 0 {
		return draw2d.ErrLayerNotOpen
	}
	layer := gc.layers[len(gc.layers)-1]
	gc.layers = gc.layers[:len(gc.layers)-1]

	// Restore the drawing state before attaching the group, so the state
	// change is invisible to the caller.
	gc.Restore()

	if len(layer.group.Groups) == 0 {
		// Empty layer: document it as producing no output.
		return nil
	}
	layer.group.Style = layerStyle(layer.mode, layer.opacity)
	// A blended child must blend with the layer's own contents, not with what
	// lies behind the layer. That is only guaranteed if the layer is a
	// stacking context, which CSS gives to groups with mix-blend-mode or
	// opacity; a plain group needs isolation:isolate.
	if layer.mode == draw2d.BlendNormal && layer.opacity >= 1 && hasBlendedChild(layer.group) {
		layer.group.Style = "isolation:isolate"
	}
	gc.setCurrentGroups(append(gc.currentGroups(), layer.group))
	return nil
}

// hasBlendedChild reports whether g contains a group with a mix-blend-mode
// declaration, directly or nested.
func hasBlendedChild(g *Group) bool {
	for _, child := range g.Groups {
		if child.Style == "" {
			continue
		}
		if strings.Contains(child.Style, "mix-blend-mode") || hasBlendedChild(child) {
			return true
		}
	}
	return false
}

// currentGroups returns the group slice new elements are appended to: the
// innermost open layer, or the SVG root.
func (gc *GraphicContext) currentGroups() []*Group {
	if n := len(gc.layers); n > 0 {
		return gc.layers[n-1].group.Groups
	}
	return gc.svg.Groups
}

// setCurrentGroups replaces the group slice new elements are appended to: the
// innermost open layer, or the SVG root.
func (gc *GraphicContext) setCurrentGroups(groups []*Group) {
	if n := len(gc.layers); n > 0 {
		gc.layers[n-1].group.Groups = groups
		return
	}
	gc.svg.Groups = groups
}

// layerStyle renders the CSS declarations for a layer group. The normal mode
// is omitted because it is the CSS initial value; a fully opaque layer emits
// no declaration at all.
func layerStyle(mode draw2d.BlendMode, opacity float64) string {
	var parts []string
	if mode != draw2d.BlendNormal {
		parts = append(parts, "mix-blend-mode:"+mode.String())
	}
	if opacity < 1 {
		parts = append(parts, "opacity:"+optiSprintf("%f", opacity))
	}
	return strings.Join(parts, ";")
}

// drawStyle renders the CSS declarations for a single drawing operation.
func drawStyle(mode draw2d.BlendMode) string {
	if mode == draw2d.BlendNormal {
		return ""
	}
	return "mix-blend-mode:" + mode.String()
}
