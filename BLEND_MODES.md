# Layer blend modes

draw2d can combine drawing with the destination using the W3C blend modes
`normal` (default), `multiply` and `screen`. The default output is unchanged:
contexts start in `draw2d.BlendNormal` and only emit a blend when asked.

The feature is available on the image backend (`draw2dimg`) and the SVG backend
(`draw2dsvg`). `draw2dpdf` and `draw2dgl` accept `normal` and return
`draw2d.ErrBlendUnsupported` for any other mode or for a layer.

## API

```go
mode := gc.GetBlendMode()                 // draw2d.BlendNormal initially
err := gc.SetBlendMode(draw2d.BlendMultiply) // per drawing operation

err = gc.BeginLayer(draw2d.BlendScreen, 0.6) // isolated group
// ... draw several shapes; they combine with each other normally ...
err = gc.EndLayer()                       // composited with the mode/opacity
```

`SetBlendMode` affects every subsequent `Fill`, `FillStroke`, `Stroke`,
`DrawImage`, `FillString*` and `StrokeString*`. Blend modes are part of the
saved state and are restored by `Restore`.

`BeginLayer` behaves like `Save`: it snapshots the drawing state (colors,
transform, ...) and `EndLayer` restores it. Inside the layer the blend mode is
`normal`, so the layer forms one isolated group; the whole group is then
blended with the backdrop using the mode and opacity passed to `BeginLayer`.

## Compositing

All compositing is done on premultiplied 8-bit RGBA:

```
normal:   co = cs + cb*(1-as)
multiply: co = (1-ab)*cs + cs*cb + (1-as)*cb
screen:   co = cs + cb - cs*cb
ao = as + ab*(1-as)
```

The image backend blends each raster span pixel by pixel, so anti-aliased edges
carry the correct coverage, transparent destinations are handled correctly and
a translucent red over blue is not washed towards grey the way a naive
"over" of a pre-flattened group would be. Layers render into a transparent
offscreen surface the size of the destination; only the rectangle that was
actually painted is read back. Nested layers are composited inside-out.

The SVG backend writes an equivalent structure:

```xml
<g style="mix-blend-mode:multiply;opacity:0.6">
  <g ...><path .../></g>
</g>
```

A per-operation mode is written as `style="mix-blend-mode:screen"` on that
operation's group. A plain layer containing blended children is marked
`style="isolation:isolate"` so those children blend with the layer instead of
the page.

## Edge cases

| situation | result |
| --- | --- |
| unknown `BlendMode` | `draw2d.ErrInvalidBlendMode`, state unchanged |
| NaN/±Inf layer opacity | `draw2d.ErrInvalidOpacity`, state unchanged |
| opacity < 0 or > 1 | clamped to [0, 1] |
| empty layer (nothing painted) | no output; destination untouched |
| nesting deeper than `draw2d.MaxLayerDepth` (64) | `draw2d.ErrLayerDepthExceeded` |
| image canvas larger than `MaxLayerPixels` (default `draw2d.DefaultMaxLayerPixels`, 64 Mi px) | `draw2d.ErrLayerTooLarge` |
| image canvas with an empty area | `draw2d.ErrEmptyCanvas` |
| `EndLayer` without an open layer | `draw2d.ErrLayerNotOpen` |
| PDF / OpenGL blend or layer | `draw2d.ErrBlendUnsupported` |
| drawing outside the canvas | clipped like any other drawing |
