package draw2ddiff

import (
	"errors"
	"flag"
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
)

var updateGolden = flag.Bool("update-golden", false,
	"rewrite the golden baselines in draw2ddiff/testdata/golden (review diffs before committing)")

func TestMain(m *testing.M) {
	// Pin the bundled fonts so text rendering is reproducible on
	// Linux and macOS instead of depending on platform fonts.
	draw2d.SetFontFolder(FontFolder())
	os.Exit(m.Run())
}

func gateUpdateMode() bool {
	return *updateGolden || os.Getenv("DRAW2D_UPDATE_GOLDEN") == "1"
}

// TestCrossBackendGate is the rendering diff gate: every scene is
// rendered on the PNG, SVG and PDF backends and compared against the
// golden baselines. A default run never modifies the golden directory.
func TestCrossBackendGate(t *testing.T) {
	report, err := RunGate(GateConfig{Update: gateUpdateMode()})
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range report.Results {
		res := res
		t.Run(fmt.Sprintf("%s/%s", res.Scene, res.Backend), func(t *testing.T) {
			if !res.Passed {
				t.Errorf("%s", res)
			} else {
				t.Logf("%s", res)
			}
		})
	}
	if gateUpdateMode() {
		t.Logf("golden baselines updated in %s", GoldenDir())
		return
	}
	if failed := report.Failures(); len(failed) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "\n--- cross-backend gate failures (%d) ---\n", len(failed))
		for _, res := range failed {
			fmt.Fprintf(&b, "%s\n", res)
		}
		if report.ArtifactDir != "" {
			fmt.Fprintf(&b, "diff images and reproduction inputs kept in %s\n", report.ArtifactDir)
		}
		t.Error(b.String())
	}
}

// TestDeterminism renders every scene twice per backend and requires
// identical snapshots, guarding cross-platform (Linux/macOS)
// reproducibility of the gate inputs.
func TestDeterminism(t *testing.T) {
	for i := range Scenes {
		scene := &Scenes[i]
		for _, backend := range Backends {
			a, errA := Render(scene, backend)
			b, errB := Render(scene, backend)
			if (errA == nil) != (errB == nil) {
				t.Errorf("%s/%s: non-deterministic error behavior: %v vs %v",
					scene.Name, backend, errA, errB)
				continue
			}
			if errA != nil {
				if errA.Error() != errB.Error() {
					t.Errorf("%s/%s: non-deterministic error: %v vs %v",
						scene.Name, backend, errA, errB)
				}
				continue
			}
			if a.Hash != b.Hash {
				t.Errorf("%s/%s: non-deterministic output: %s vs %s",
					scene.Name, backend, a.Hash, b.Hash)
			}
		}
	}
}

// mutatedScene builds a scene variant for mutation testing.
func mutatedScene(name string, draw func(gc draw2d.GraphicContext)) *Scene {
	return &Scene{Name: name, Kind: KindNormal, Draw: draw}
}

// requireGateFailure asserts that the mutated scene is detected as a
// difference on every backend where both renders succeed.
func requireGateFailure(t *testing.T, scene, mutated *Scene) {
	t.Helper()
	for _, backend := range Backends {
		base, err := Render(scene, backend)
		if err != nil {
			t.Fatalf("%s/%s: baseline render failed: %v", scene.Name, backend, err)
		}
		mut, err := Render(mutated, backend)
		if err != nil {
			t.Fatalf("%s/%s: mutated render failed: %v", mutated.Name, backend, err)
		}
		res := Compare(scene, base, mut, DefaultTolerances())
		if res.Passed {
			t.Errorf("%s/%s: mutation was NOT detected (category %s)", scene.Name, backend, res.Category)
		} else {
			t.Logf("%s/%s: mutation detected: %s", scene.Name, backend, res)
		}
	}
}

// TestGateDetectsColorMutation: changing a fill color must fail the gate.
func TestGateDetectsColorMutation(t *testing.T) {
	scene := FindScene("overlap")
	mutated := mutatedScene("overlap", func(gc draw2d.GraphicContext) {
		gc.SetFillColor(color.RGBA{0x30, 0xe0, 0x30, 0xb0}) // was 0xe0, 0x30, 0x30
		gc.Fill(rectPath(40, 40, 140, 120))
		gc.SetFillColor(color.RGBA{0x30, 0x30, 0xe0, 0xb0})
		gc.Fill(rectPath(90, 70, 190, 150))
	})
	requireGateFailure(t, scene, mutated)
}

// TestGateDetectsMatrixMutation: changing the transform must fail the gate.
func TestGateDetectsMatrixMutation(t *testing.T) {
	scene := FindScene("transform")
	mutated := mutatedScene("transform", func(gc draw2d.GraphicContext) {
		gc.Save()
		gc.Translate(150, 100)
		gc.Rotate(math.Pi / 4) // was math.Pi / 6
		gc.Scale(1.5, 0.75)
		gc.SetFillColor(color.RGBA{0x2a, 0x9d, 0x8f, 0xff})
		gc.Fill(rectPath(-40, -25, 40, 25))
		gc.Restore()
		gc.SetFillColor(color.RGBA{0xe9, 0xc4, 0x6a, 0xff})
		gc.Fill(rectPath(10, 10, 30, 30))
	})
	requireGateFailure(t, scene, mutated)
}

// TestGateDetectsPathOrderMutation: swapping the drawing order of two
// overlapping translucent shapes must fail the gate.
func TestGateDetectsPathOrderMutation(t *testing.T) {
	scene := FindScene("overlap")
	mutated := mutatedScene("overlap", func(gc draw2d.GraphicContext) {
		// same shapes, reversed order
		gc.SetFillColor(color.RGBA{0x30, 0x30, 0xe0, 0xb0})
		gc.Fill(rectPath(90, 70, 190, 150))
		gc.SetFillColor(color.RGBA{0xe0, 0x30, 0x30, 0xb0})
		gc.Fill(rectPath(40, 40, 140, 120))
	})
	requireGateFailure(t, scene, mutated)
}

// TestMissingFontNotSilentlyPassed: a scene whose pinned font is
// unavailable must be classified font-platform and must fail the gate.
func TestMissingFontNotSilentlyPassed(t *testing.T) {
	missing := draw2d.FontData{Name: "font-that-does-not-exist", Family: draw2d.FontFamilySans}
	scene := &Scene{
		Name: "text-missing-font",
		Kind: KindNormal,
		Font: &missing,
		Draw: func(gc draw2d.GraphicContext) {
			gc.SetFontData(missing)
			gc.FillStringAt("hello", 10, 10)
		},
	}
	for _, backend := range Backends {
		_, err := Render(scene, backend)
		var ferr *FontError
		if err == nil {
			t.Errorf("%s: missing font silently passed render", backend)
			continue
		}
		if !errors.As(err, &ferr) {
			t.Errorf("%s: missing font classified as %T, want *FontError", backend, err)
		}
	}
}

// TestEdgeCaseClassifications: edge-case scenes get their stable
// categories and are compared, not silently passed.
func TestEdgeCaseClassifications(t *testing.T) {
	// Empty scene must produce blank/empty output on every backend.
	empty := FindScene("empty")
	for _, backend := range Backends {
		snap, err := Render(empty, backend)
		if err != nil {
			t.Fatalf("empty/%s: render failed: %v", backend, err)
		}
		if !snap.Blank() {
			t.Errorf("empty/%s: empty scene produced non-blank output", backend)
		}
	}
	// Edge-case scenes render (or fail) deterministically; the gate
	// records either outcome in the baseline.
	for _, name := range []string{"nonzero-bounds", "missing-glyph", "nonfinite"} {
		scene := FindScene(name)
		for _, backend := range Backends {
			snap, err := Render(scene, backend)
			if err != nil {
				t.Logf("%s/%s: classified %s (backend rejected input: %v)",
					name, backend, scene.Kind.category(), err)
				continue
			}
			if snap.Hash == "" {
				t.Errorf("%s/%s: snapshot without hash", name, backend)
			}
			t.Logf("%s/%s: classified %s, hash %s, counts %s",
				name, backend, scene.Kind.category(), snap.Hash, countsString(snap.Counts))
		}
	}
}

// TestMissingGoldenFails: without baselines the gate must fail loudly.
func TestMissingGoldenFails(t *testing.T) {
	dir := t.TempDir()
	_, err := RunGate(GateConfig{GoldenDir: dir})
	if err == nil {
		t.Fatal("gate passed without golden baselines")
	}
	if !strings.Contains(err.Error(), "update-golden") {
		t.Errorf("error should explain how to create baselines, got: %v", err)
	}
}

// TestDefaultRunDoesNotTouchGoldens: a default gate run must not modify
// the golden directory.
func TestDefaultRunDoesNotTouchGoldens(t *testing.T) {
	if gateUpdateMode() {
		t.Skip("running in update mode")
	}
	dir := GoldenDir()
	before := dirState(t, dir)
	if _, err := RunGate(GateConfig{}); err != nil {
		t.Fatal(err)
	}
	after := dirState(t, dir)
	for name, sum := range before {
		if after[name] != sum {
			t.Errorf("default gate run modified golden file %s", name)
		}
	}
	if len(after) != len(before) {
		t.Errorf("default gate run changed golden directory entries: %d -> %d", len(before), len(after))
	}
}

func dirState(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading golden dir: %v", err)
	}
	state := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		state[e.Name()] = hashBytes(data)
	}
	return state
}
