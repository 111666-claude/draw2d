# draw2ddiff — cross-backend rendering diff gate

`draw2ddiff` turns "render the same scene to PNG, SVG and PDF and eyeball
the three outputs" into an automated gate. A fixed set of scenes is drawn
through all three draw2d backends (`draw2dimg`, `draw2dsvg`, `draw2dpdf`)
and the resulting snapshots are compared against versioned golden
baselines. Any unexpected difference fails `go test`.

## What is compared

For every scene × backend pair the gate reports:

- **PNG (raster)**: pixel error — number of differing pixels, maximum
  per-channel delta, and the first differing pixel position `(x,y)`.
- **SVG (structural)**: element counts (`g`, `path`, `text`, `image`,
  `font`, `glyph`, `mask`) and the first differing line of the canonical
  XML form.
- **PDF (structural)**: content-stream operator counts (`moveto`,
  `lineto`, `curveto`, `fill`, `stroke`, `text`, `image`, `matrix`, …)
  and the first differing token line of the canonical content stream.

## Scenes

Fixed programs in `scenes.go` cover paths (all segment types, dash,
caps/joins), overlapping translucent shapes (order-sensitive), text,
images, transforms, alpha, plus edge-case scenes: `empty`,
`nonzero-bounds`, `missing-glyph`, `nonfinite`.

## Running

```sh
go test ./draw2ddiff/                 # the gate (read-only)
go test ./draw2ddiff/ -update-golden  # regenerate baselines (review first!)
DRAW2D_UPDATE_GOLDEN=1 go test ./draw2ddiff/   # same as -update-golden
```

A default run **never modifies** `testdata/golden`
(`TestDefaultRunDoesNotTouchGoldens` guards this). When the gate fails,
diff images (`*.diff.png`), the current and golden outputs and a minimal
reproduction input (`*.repro.txt`) are written to a fresh temp directory
(or to `DRAW2D_DIFF_OUT` when set) and the path is printed in the test
output.

## Tolerances

Raster comparison uses explicit tolerances (`Tolerances`):

- `ChannelDelta` — per-channel anti-aliasing slack (default 1),
- `MaxDiffPixels` / `MaxDiffFraction` — differing-pixel budget (default 0).

Structural (SVG/PDF) comparison is always exact.

## Categories

Every result has a stable category; anything other than `ok` fails the
gate — non-comparable items are never silently passed:

| category | meaning |
|---|---|
| `ok` | matches the golden baseline |
| `pixel-diff` / `structure-diff` | differs beyond tolerance |
| `empty-scene` | scene draws nothing (verified blank) |
| `nonzero-bounds` | image bounds not starting at (0,0) |
| `missing-glyph` | font lacks requested glyphs |
| `non-finite-params` | NaN/Inf input (a backend that hangs is bounded by a timeout and recorded as "did not terminate") |
| `output-failure` | backend failed to render/save |
| `font-platform` | pinned font unavailable — output would depend on platform fonts |
| `missing-golden` / `baseline-version-mismatch` | baselines missing or stale |

Deterministic backend failures on edge-case scenes (e.g. the rasterizer
not terminating on NaN input) are recorded in `baseline.json` as the
expected outcome, so the classification itself is regression-tested.

## Reproducibility (Linux / macOS)

- Text uses only the bundled `resource/font/luximbi.ttf` (the one font
  that also has gofpdf `.json`/`.z` definitions), resolved relative to
  the source tree — never platform fonts.
- PDF output is deterministic: compression off, creation date pinned.
- Raster snapshots are normalized to NRGBA so PNG golden round-trips
  compare equal on any platform.
- `TestDeterminism` renders every scene twice and requires identical
  hashes.

## Baseline format

`testdata/golden/baseline.json` (format `version: 1`) maps each
`scene/backend` pair to a content hash, element counts, and — for
deterministically failing edge cases — the expected error. The artifacts
(`<scene>.png|.svg|.pdf`) sit next to it. Bump `BaselineVersion` in
`golden.go` when the snapshot format changes; old baselines then fail
loudly until regenerated.

## Mutation coverage

`TestGateDetectsColorMutation`, `TestGateDetectsMatrixMutation` and
`TestGateDetectsPathOrderMutation` prove the gate fails when a color, the
transform, or the drawing order changes — on all three backends.

## Known limitation

The PDF structural extractor scans uncompressed `stream`/`endstream`
blocks with a minimal parser. It is intentionally dependency-free and
works with gofpdf's uncompressed output as configured here; it is not a
general PDF parser.
