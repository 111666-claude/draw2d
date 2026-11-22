package draw2ddiff

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
)

// GateConfig configures a gate run.
type GateConfig struct {
	// GoldenDir holds the golden baselines. Defaults to GoldenDir().
	GoldenDir string
	// Tolerances for raster comparison. Zero value means defaults.
	Tolerances *Tolerances
	// Update rewrites the golden baselines instead of comparing.
	// Never set this from a default test run.
	Update bool
	// ArtifactDir receives failure artifacts (diff images, current and
	// golden outputs, minimal reproduction inputs). Defaults to a fresh
	// directory under os.TempDir(); override with DRAW2D_DIFF_OUT.
	ArtifactDir string
}

// GateReport is the outcome of a full gate run.
type GateReport struct {
	Results     []*Result
	ArtifactDir string // non-empty when failure artifacts were written
}

// Failures returns the results that did not pass.
func (r *GateReport) Failures() []*Result {
	var failed []*Result
	for _, res := range r.Results {
		if !res.Passed {
			failed = append(failed, res)
		}
	}
	return failed
}

// RunGate renders every registered scene on every backend and compares
// the snapshots against the golden baselines. In update mode it rewrites
// the baselines instead. A default (non-update) run never modifies the
// golden directory.
func RunGate(cfg GateConfig) (*GateReport, error) {
	dir := cfg.GoldenDir
	if dir == "" {
		dir = GoldenDir()
	}
	tol := DefaultTolerances()
	if cfg.Tolerances != nil {
		tol = *cfg.Tolerances
	}
	if cfg.Update {
		return updateGoldens(dir)
	}

	baseline, err := LoadBaseline(dir)
	if err != nil {
		return nil, fmt.Errorf("no usable golden baseline in %s (%v); run `go test ./draw2ddiff -update-golden` to create it", dir, err)
	}
	if baseline.Version != BaselineVersion {
		return nil, fmt.Errorf("golden baseline version %d, gate understands %d; regenerate with `go test ./draw2ddiff -update-golden`", baseline.Version, BaselineVersion)
	}

	report := &GateReport{}
	for i := range Scenes {
		scene := &Scenes[i]
		for _, backend := range Backends {
			res := checkOne(baseline, dir, scene, backend, tol)
			report.Results = append(report.Results, res)
		}
	}
	// Keep failure artifacts outside the golden directory.
	for _, res := range report.Failures() {
		artDir, err := artifactDir(cfg.ArtifactDir, &report.ArtifactDir)
		if err != nil {
			return report, err
		}
		writeFailureArtifacts(artDir, baseline, dir, res, tol)
	}
	return report, nil
}

// checkOne renders one scene on one backend and compares it against the
// golden expectation.
func checkOne(baseline *Baseline, dir string, scene *Scene, backend Backend, tol Tolerances) *Result {
	res := &Result{Scene: scene.Name, Backend: backend}
	entry := baseline.Entries[entryKey(scene.Name, backend)]

	snap, renderErr := Render(scene, backend)

	if renderErr != nil {
		res.Category = CategoryOutputFailure
		var ferr *FontError
		if errors.As(renderErr, &ferr) {
			res.Category = CategoryFontPlatform
		}
		res.Detail = renderErr.Error()
		// A baseline-recorded deterministic failure is a stable,
		// comparable outcome for edge-case scenes.
		if entry != nil && entry.Error != "" {
			res.Passed = true
			res.Category = scene.Kind.category()
			res.Detail = "backend failed as recorded in baseline: " + renderErr.Error()
		}
		return res
	}

	if entry == nil {
		res.Category = CategoryMissingGolden
		res.Detail = fmt.Sprintf("no golden entry for %s/%s; regenerate baselines", scene.Name, backend)
		return res
	}
	if entry.Error != "" {
		res.Category = CategoryOutputFailure
		res.Detail = fmt.Sprintf("baseline records failure %q but rendering succeeded", entry.Error)
		return res
	}

	// Edge-classified scenes must behave as classified: an empty scene
	// that produces output is a failure, not a silent pass.
	if scene.Kind == KindEmpty && !snap.Blank() {
		res.Category = CategoryEmptyScene
		res.Detail = "empty scene produced non-blank output"
		return res
	}

	golden, err := LoadGoldenSnapshot(dir, scene, backend, entry)
	if err != nil {
		res.Category = CategoryMissingGolden
		res.Detail = err.Error()
		return res
	}
	return Compare(scene, golden, snap, tol)
}

// updateGoldens renders everything and rewrites the golden directory.
func updateGoldens(dir string) (*GateReport, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	baseline := &Baseline{Version: BaselineVersion, Entries: map[string]*GoldenEntry{}}
	report := &GateReport{}
	for i := range Scenes {
		scene := &Scenes[i]
		for _, backend := range Backends {
			key := entryKey(scene.Name, backend)
			snap, err := Render(scene, backend)
			res := &Result{Scene: scene.Name, Backend: backend}
			if err != nil {
				// Record deterministic failures as the stable outcome.
				baseline.Entries[key] = &GoldenEntry{Error: err.Error()}
				res.Passed = true
				res.Category = scene.Kind.category()
				res.Detail = "recorded baseline failure: " + err.Error()
				report.Results = append(report.Results, res)
				continue
			}
			if err := SaveGoldenSnapshot(dir, snap); err != nil {
				return nil, fmt.Errorf("writing golden %s: %v", key, err)
			}
			entry := &GoldenEntry{Hash: snap.Hash, Counts: snap.Counts}
			if backend == BackendPNG {
				entry.Width, entry.Height = snap.Width, snap.Height
			}
			baseline.Entries[key] = entry
			res.Passed = true
			res.Category = scene.Kind.category()
			res.GoldenHash = snap.Hash
			report.Results = append(report.Results, res)
		}
	}
	if err := baseline.Save(dir); err != nil {
		return nil, err
	}
	return report, nil
}

// artifactDir resolves the failure-artifact directory, creating it lazily.
func artifactDir(configured string, cached *string) (string, error) {
	if *cached != "" {
		return *cached, nil
	}
	dir := configured
	if dir == "" {
		dir = os.Getenv("DRAW2D_DIFF_OUT")
	}
	if dir == "" {
		d, err := os.MkdirTemp("", "draw2ddiff-")
		if err != nil {
			return "", err
		}
		dir = d
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	*cached = dir
	return dir, nil
}

// writeFailureArtifacts keeps the current output, the golden, the diff
// image and a minimal reproduction input for one failed result.
func writeFailureArtifacts(artDir string, baseline *Baseline, goldenDir string, res *Result, tol Tolerances) {
	base := fmt.Sprintf("%s.%s", res.Scene, res.Backend)
	scene := FindScene(res.Scene)
	if scene == nil {
		return
	}
	// Current output, re-rendered.
	if snap, err := Render(scene, res.Backend); err == nil {
		ext := string(res.Backend)
		data := snap.Canonical
		if res.Backend == BackendPNG {
			data = encodePNG(imageFromSnapshot(snap))
		}
		os.WriteFile(filepath.Join(artDir, base+".current."+ext), data, 0o644)
	}
	// Golden copy.
	entry := baseline.Entries[entryKey(res.Scene, res.Backend)]
	if entry != nil && entry.Error == "" {
		if golden, err := LoadGoldenSnapshot(goldenDir, scene, res.Backend, entry); err == nil {
			ext := string(res.Backend)
			data := golden.Canonical
			if res.Backend == BackendPNG {
				data = encodePNG(imageFromSnapshot(golden))
			}
			os.WriteFile(filepath.Join(artDir, base+".golden."+ext), data, 0o644)
		}
	}
	// Diff image.
	if res.DiffImage != nil {
		os.WriteFile(filepath.Join(artDir, base+".diff.png"), res.DiffImage, 0o644)
	}
	// Minimal reproduction input.
	repro := fmt.Sprintf(`scene:    %s
backend:  %s
category: %s
detail:   %s
golden:   %s
current:  %s

Reproduce with:
  go test ./draw2ddiff -run 'TestCrossBackendGate/%s' -v

Regenerate baselines (review the diff first) with:
  go test ./draw2ddiff -update-golden
`, res.Scene, res.Backend, res.Category, res.Detail, res.GoldenHash, res.CurrentHash, res.Scene)
	os.WriteFile(filepath.Join(artDir, base+".repro.txt"), []byte(repro), 0o644)
}

// imageFromSnapshot reconstructs an image from a raster snapshot.
func imageFromSnapshot(snap *Snapshot) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, snap.Width, snap.Height))
	img.Pix = append([]byte(nil), snap.Pixels...)
	return img
}
