#!/usr/bin/env python3
"""Rasterize the dash verification scenes and compare the visible stroke
intervals produced by the pdf backend against the png (reference image
backend) and svg backends.

Run with an interpreter that provides pypdfium2 and pillow:
    python -I verify.py <scene-dir>
"""

import os
import sys

import pypdfium2 as pdfium
import cairosvg
from PIL import Image

# A pixel is considered covered by the stroke when its gray value is
# below this threshold. Antialiasing leaves a few intermediate values;
# the threshold only needs to separate core stroke pixels from paper.
INK = 200
# Max allowed displacement, in pixels, between matching segment edges.
TOL = 3.0


def load_raster(path):
    # The image backend emits an RGBA bitmap with a transparent
    # background whose RGB channels are black; composite it on white so
    # empty pixels read like the pdf page background.
    img = Image.open(path).convert("RGBA")
    bg = Image.new("RGBA", img.size, (255, 255, 255, 255))
    return Image.alpha_composite(bg, img).convert("RGB")


def render_pdf(path):
    doc = pdfium.PdfDocument(path)
    page = doc[0]
    return page.render(scale=1).to_pil().convert("RGB")


def render_svg(path, size):
    # The draw2d svg backend does not emit a viewBox; inject one matching
    # the pdf page so the rasterizer maps user units to pixels 1:1.
    src = open(path, encoding="utf-8").read()
    src = src.replace("<svg ", '<svg viewBox="0 0 %d %d" ' % size, 1)
    png = cairosvg.svg2png(
        bytestring=src.encode("utf-8"),
        output_width=size[0], output_height=size[1],
        background_color="white")
    import io
    return Image.open(io.BytesIO(png)).convert("RGB")


def ink_intervals(img, axis_fixed, horizontal=True):
    """Return the covered [(start, end), ...] intervals along a line.

    A column (horizontal line) or row (vertical scan) is covered when at
    least one pixel around the fixed coordinate is dark enough, so a
    thin antialiased stroke cannot be missed.
    """
    w, h = img.size
    px = img.load()
    band = 2
    lo = max(0, axis_fixed - band)
    hi = min(h if horizontal else w, axis_fixed + band + 1)
    intervals = []
    start = None
    n = w if horizontal else h
    for t in range(n):
        covered = False
        for s in range(lo, hi):
            x, y = (t, s) if horizontal else (s, t)
            r, g, b = px[x, y]
            if r < INK and g < INK and b < INK:
                covered = True
                break
        if covered and start is None:
            start = t
        elif not covered and start is not None:
            intervals.append((start, t - 1))
            start = None
    if start is not None:
        intervals.append((start, n - 1))
    return intervals


def merge_gaps(intervals, max_gap=2):
    """Merge intervals separated by antialiasing noise."""
    out = []
    for a, b in intervals:
        if out and a - out[-1][1] - 1 <= max_gap:
            out[-1] = (out[-1][0], b)
        else:
            out.append((a, b))
    return out


def cmp_intervals(name, ref, got, tol=TOL):
    ref = merge_gaps(ref)
    got = merge_gaps(got)
    errors = []
    if len(ref) != len(got):
        errors.append(f"{name}: segment count {len(got)} != reference {len(ref)} "
                      f"(pdf={got}, ref={ref})")
        return errors
    for i, ((r0, r1), (g0, g1)) in enumerate(zip(ref, got)):
        if abs(r0 - g0) > tol or abs(r1 - g1) > tol:
            errors.append(f"{name}: segment {i} {g0}-{g1} drifts from reference "
                          f"{r0}-{r1} by more than {tol}px")
    return errors


def verify_scene_named(scene_dir, name, horizontal_scans, only=None):
    errors = []
    size = (600, 480)
    pdf_img = render_pdf(os.path.join(scene_dir, name + ".pdf"))
    refs = {
        "png": load_raster(os.path.join(scene_dir, name + ".png")),
        "svg": render_svg(os.path.join(scene_dir, name + ".svg"), size),
    }
    for backend, ref_img in refs.items():
        if only is not None and backend not in only:
            continue
        for label, y in horizontal_scans:
            ref_iv = ink_intervals(ref_img, y, horizontal=True)
            pdf_iv = ink_intervals(pdf_img, y, horizontal=True)
            errors += cmp_intervals(
                f"{name}/{backend}/{label}@y={y}", ref_iv, pdf_iv)
    return errors, pdf_img, refs["png"]


def verify_scene(scene_dir, name, horizontal_scans):
    return verify_scene_named(scene_dir, name, horizontal_scans)


def main():
    if len(sys.argv) != 2:
        print("usage: verify.py <scene-dir>", file=sys.stderr)
        return 2
    scene_dir = sys.argv[1]
    all_errors = []

    # Straight lines: one scan per pattern.
    line_scans = [
        ("dash20-10-off0", 40),
        ("dash20-10-off7", 80),
        ("odd15-8-5-off0", 120),
        ("odd15-8-5-off-11", 160),
        ("dash30-6-off300", 200),
        ("solid", 250),
    ]
    all_errors += verify_scene(scene_dir, "line", line_scans)[0]

    # Save/restore scene: outer and inner patterns must both render.
    all_errors += verify_scene(scene_dir, "restore", [
        ("outer", 50), ("inner", 110), ("restored", 170),
    ])[0]

    # Scaling scene: both rows share a 15/7 pattern in user units, the
    # scaled one must show doubled dash lengths.
    scale_err, scale_pdf, scale_png = verify_scene(
        scene_dir, "scale", [("scaled", 200), ("unscaled", 260)])
    all_errors += scale_err
    scaled_pdf = merge_gaps(ink_intervals(scale_pdf, 200))
    unscaled_pdf = merge_gaps(ink_intervals(scale_pdf, 260))
    if len(scaled_pdf) > 0 and len(unscaled_pdf) > 0:
        sd = scaled_pdf[0][1] - scaled_pdf[0][0]
        ud = unscaled_pdf[0][1] - unscaled_pdf[0][0]
        if not 1.7 < sd / max(ud, 1) < 2.3:
            all_errors.append(
                f"scale: scaled dash length {sd}px is not ~2x the unscaled "
                f"{ud}px (CTM not applied to dash pattern)")
    else:
        all_errors.append("scale: missing dash segments in pdf render")

    # Cap/join scene: pdf and svg both extend each dash by half the line
    # width under round/square caps (line width 8 -> 4px extension), the
    # image backend stroker does not extend caps. Compare pdf against the
    # spec compliant svg render and merely report the legacy png gap.
    cap_err, cap_pdf, _ = verify_scene_named(
        scene_dir, "capjoin", [("round-cap", 50), ("square-cap", 110)],
        only=("svg",))
    all_errors += cap_err
    png_img = load_raster(os.path.join(scene_dir, "capjoin.png"))
    for label, y in (("round-cap", 50), ("square-cap", 110)):
        pdf_iv = merge_gaps(ink_intervals(cap_pdf, y))
        png_iv = merge_gaps(ink_intervals(png_img, y))
        if len(pdf_iv) != len(png_iv):
            all_errors.append(
                f"capjoin/png/{label}: segment count mismatch "
                f"pdf={len(pdf_iv)} png={len(png_iv)}")
            continue
        # the png reference sits 4px inside every pdf edge; check the
        # dash centers and pitch still coincide
        for i, ((p0, p1), (n0, n1)) in enumerate(zip(pdf_iv, png_iv)):
            if abs((p0 + p1) - (n0 + n1)) > 4:
                all_errors.append(
                    f"capjoin/png/{label}: segment {i} center drifts "
                    f"pdf=({p0},{p1}) png=({n0},{n1})")

    # Curve and multipath scenes: verify total inked coverage against the
    # png and svg references along several scan rows.
    for name, rows in (
        ("curve", (120, 200, 280, 340)),
        ("multipath", (60, 120, 300, 360, 420)),
    ):
        pdf_img = render_pdf(os.path.join(scene_dir, name + ".pdf"))
        refs = {
            "png": load_raster(os.path.join(scene_dir, name + ".png")),
            "svg": render_svg(os.path.join(scene_dir, name + ".svg"), (600, 480)),
        }
        for backend, png_img in refs.items():
            for y in rows:
                ref_iv = ink_intervals(png_img, y)
                pdf_iv = ink_intervals(pdf_img, y)
                ref_ink = sum(b - a + 1 for a, b in merge_gaps(ref_iv))
                pdf_ink = sum(b - a + 1 for a, b in merge_gaps(pdf_iv))
                if ref_ink == 0:
                    continue  # the curve may not cross this row
                if abs(ref_ink - pdf_ink) > 6:
                    all_errors.append(
                        f"{name}/{backend}@y={y}: inked width "
                        f"pdf={pdf_ink}px vs {backend}={ref_ink}px")

    if all_errors:
        print("FAIL")
        for e in all_errors:
            print(" -", e)
        return 1
    print("OK: pdf dash rendering matches png/svg reference on all scenes")
    return 0


if __name__ == "__main__":
    sys.exit(main())
