package draw2ddiff

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

// BaselineVersion is the golden baseline format version understood by
// this gate. Bump it whenever the snapshot or baseline format changes;
// existing baselines then fail with CategoryVersionMismatch until
// regenerated with -update-golden.
const BaselineVersion = 1

// GoldenDir is the directory holding the golden baselines, resolved
// relative to this source file.
func GoldenDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "golden")
}

// Baseline is the versioned golden baseline manifest.
type Baseline struct {
	Version int `json:"version"`
	// Entries maps "scene/backend" to its golden expectation.
	Entries map[string]*GoldenEntry `json:"entries"`
}

// GoldenEntry is the golden expectation for one scene/backend pair.
type GoldenEntry struct {
	// Error, when non-empty, records that the backend deterministically
	// fails on this scene (the stable outcome for e.g. non-finite input).
	Error string `json:"error,omitempty"`
	// Hash of the golden snapshot payload.
	Hash string `json:"hash,omitempty"`
	// Counts are the golden element/operator counts (structural backends).
	Counts map[string]int `json:"counts,omitempty"`
	// Width/Height of the golden raster (raster backend).
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

func entryKey(scene string, backend Backend) string {
	return fmt.Sprintf("%s/%s", scene, backend)
}

// LoadBaseline reads the golden baseline manifest from dir.
func LoadBaseline(dir string) (*Baseline, error) {
	data, err := os.ReadFile(filepath.Join(dir, "baseline.json"))
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing baseline.json: %v", err)
	}
	if b.Entries == nil {
		b.Entries = map[string]*GoldenEntry{}
	}
	return &b, nil
}

// Save writes the baseline manifest and the per-scene golden artifacts
// into dir. It is only called in update mode.
func (b *Baseline) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "baseline.json"), append(data, '\n'), 0o644)
}

// artifactName is the golden file name for a scene/backend pair.
func artifactName(scene string, backend Backend) string {
	return fmt.Sprintf("%s.%s", scene, backend)
}

// LoadGoldenSnapshot reconstructs the golden snapshot for a scene/backend
// pair from the baseline directory.
func LoadGoldenSnapshot(dir string, scene *Scene, backend Backend, entry *GoldenEntry) (*Snapshot, error) {
	path := filepath.Join(dir, artifactName(scene.Name, backend))
	switch backend {
	case BackendPNG:
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, err
		}
		snap := NewRasterSnapshot(scene.Name, img)
		if snap.Hash != entry.Hash {
			return nil, fmt.Errorf("golden artifact %s does not match baseline.json hash", artifactName(scene.Name, backend))
		}
		return snap, nil
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		snap := NewStructuralSnapshot(scene.Name, backend, entry.Counts, data)
		if snap.Hash != entry.Hash {
			return nil, fmt.Errorf("golden artifact %s does not match baseline.json hash", artifactName(scene.Name, backend))
		}
		return snap, nil
	}
}

// SaveGoldenSnapshot writes a snapshot as a golden artifact (update mode).
func SaveGoldenSnapshot(dir string, snap *Snapshot) error {
	path := filepath.Join(dir, artifactName(snap.Scene, snap.Backend))
	switch snap.Backend {
	case BackendPNG:
		img := image.NewNRGBA(image.Rect(0, 0, snap.Width, snap.Height))
		img.Pix = snap.Pixels
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	default:
		return os.WriteFile(path, snap.Canonical, 0o644)
	}
}
