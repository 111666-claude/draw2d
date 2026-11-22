// Copyright 2026 The draw2d Authors. All rights reserved.
// Fixed-seed fuzz targets for the flattening pipeline. The seed corpus
// doubles as the reproducible regression samples (zero dash pattern,
// infinite flattening scale, NaN curves, malformed component streams,
// huge dash offsets, negative line widths). Run an actual fuzzing session
// with: go test -fuzz=FuzzFlatten -fuzztime=30s ./draw2dbase

package draw2dbase

import (
	"math"
	"testing"

	"github.com/llgcode/draw2d"
)

// decodeFuzzFloat maps a byte to an interesting float64: small integers for
// benign geometry, plus NaN, ±Inf, huge and denormal values.
func decodeFuzzFloat(b byte) float64 {
	switch b {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return -1
	case 3:
		return math.NaN()
	case 4:
		return math.Inf(1)
	case 5:
		return math.Inf(-1)
	case 6:
		return 1e308
	case 7:
		return -1e308
	case 8:
		return 1e-300
	case 9:
		return math.MaxFloat64
	case 10:
		return math.SmallestNonzeroFloat64
	default:
		return float64(b) - 128
	}
}

// pathFromFuzzBytes deterministically decodes a byte slice into a path,
// including structurally malformed component streams (case 6) and truncated
// point streams, mimicking externally imported paths.
func pathFromFuzzBytes(data []byte) *draw2d.Path {
	p := new(draw2d.Path)
	i := 0
	next := func() float64 {
		if i >= len(data) {
			return 0
		}
		v := decodeFuzzFloat(data[i])
		i++
		return v
	}
	for i < len(data) {
		cmd := data[i] % 8
		i++
		switch cmd {
		case 0:
			p.MoveTo(next(), next())
		case 1:
			p.LineTo(next(), next())
		case 2:
			p.QuadCurveTo(next(), next(), next(), next())
		case 3:
			p.CubicCurveTo(next(), next(), next(), next(), next(), next())
		case 4:
			p.ArcTo(next(), next(), next(), next(), next(), next())
		case 5:
			p.Close()
		case 6:
			// raw component injection: desynchronizes Components and Points
			if i < len(data) {
				p.Components = append(p.Components, draw2d.PathCmp(data[i]%8))
				i++
			}
		case 7:
			// truncate the point stream
			if len(p.Points) > 0 && i < len(data) {
				p.Points = p.Points[:int(data[i])%len(p.Points)]
				i++
			}
		}
	}
	return p
}

func checkOutcome(t *testing.T, outcome runOutcome, recovered any, what string) {
	t.Helper()
	switch outcome {
	case runPanicked:
		t.Fatalf("%s: panicked: %v", what, recovered)
	case runTimedOut:
		t.Fatalf("%s: did not finish within %s (suspected infinite loop)", what, sampleTimeBudget)
	}
}

func FuzzFlatten(f *testing.F) {
	// Regression seeds (see TestRegressions):
	f.Add([]byte{1, 4, 1, 1, 1, 1, 1, 1})          // arc flattened with finite scale
	f.Add([]byte{4, 4, 1, 1, 1, 1, 1, 1})          // arc flattened with +Inf scale (used to hang in TraceArc)
	f.Add([]byte{1, 4, 1, 1, 1, 1, 1, 6})          // arc with a 1e308 sweep (used to hang in TraceArc)
	f.Add([]byte{1, 0, 1, 1, 3, 3, 3, 3, 3, 3, 3}) // cubic with NaN control points (used to emit NaN vertices)
	f.Add([]byte{1, 3, 1, 1, 1, 1, 1, 1})          // cubic as first component (used to panic, index -2)
	f.Add([]byte{1, 6, 3, 1, 1, 1, 1, 1, 1})       // raw cubic component without points (used to panic)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > 4096 {
			return
		}
		scale := decodeFuzzFloat(data[0])
		p := pathFromFuzzBytes(data[1:])
		rec := &vertexRecorder{}
		outcome, recovered := runSample(sampleTimeBudget, func() {
			Flatten(p, rec, scale)
		})
		checkOutcome(t, outcome, recovered, "Flatten")
		checkRecorder(t, rec, "Flatten")
	})
}

func FuzzDasher(f *testing.F) {
	// Regression seeds:
	f.Add([]byte{1, 0, 1, 1, 1, 138, 138})    // dash [0] (used to hang)
	f.Add([]byte{2, 1, 2, 1, 1, 1, 138, 138}) // dash [1,-1] (used to hang)
	f.Add([]byte{1, 1, 6, 1, 1, 138, 138})    // dash offset 1e308 (used to hang)
	f.Add([]byte{0, 1, 1, 1, 138, 138})       // no dash: solid line
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 || len(data) > 4096 {
			return
		}
		i := 0
		nDash := int(data[i]) % 6
		i++
		dash := make([]float64, nDash)
		for k := range dash {
			if i >= len(data) {
				return
			}
			dash[k] = decodeFuzzFloat(data[i])
			i++
		}
		offset := float64(0)
		if i < len(data) {
			offset = decodeFuzzFloat(data[i])
			i++
		}
		rec := &vertexRecorder{}
		outcome, recovered := runSample(sampleTimeBudget, func() {
			dasher := NewDashConverter(dash, offset, rec)
			dasher.MoveTo(0, 0)
			for ; i+1 < len(data); i += 2 {
				dasher.LineTo(decodeFuzzFloat(data[i]), decodeFuzzFloat(data[i+1]))
			}
			dasher.End()
		})
		checkOutcome(t, outcome, recovered, "Dasher")
		checkRecorder(t, rec, "Dasher")
	})
}

func FuzzLineStroker(f *testing.F) {
	// Regression seeds:
	f.Add([]byte{2, 1, 1, 138, 138}) // negative line width
	f.Add([]byte{3, 1, 1, 138, 138}) // NaN line width (used to emit NaN vertices)
	f.Add([]byte{6, 1, 1, 138, 138}) // 1e308 line width
	f.Add([]byte{1, 3, 3, 138, 138}) // NaN coordinates (used to emit NaN vertices)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 4096 {
			return
		}
		halfWidth := decodeFuzzFloat(data[0])
		rec := &vertexRecorder{}
		outcome, recovered := runSample(sampleTimeBudget, func() {
			stroker := NewLineStroker(draw2d.RoundCap, draw2d.RoundJoin, rec)
			stroker.HalfLineWidth = halfWidth
			stroker.MoveTo(decodeFuzzFloat(data[1]), decodeFuzzFloat(data[2]))
			for i := 3; i+1 < len(data); i += 2 {
				stroker.LineTo(decodeFuzzFloat(data[i]), decodeFuzzFloat(data[i+1]))
			}
			stroker.Close()
			stroker.End()
		})
		checkOutcome(t, outcome, recovered, "LineStroker")
		checkRecorder(t, rec, "LineStroker")
	})
}

func FuzzTraceCurves(f *testing.F) {
	f.Add([]byte{1, 1, 1, 1, 1, 1, 1, 1})       // benign cubic
	f.Add([]byte{6, 7, 6, 7, 6, 7, 6, 7})       // ±1e308 cubic (deep subdivision)
	f.Add([]byte{3, 3, 3, 3, 3, 3, 3, 3})       // NaN cubic
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0})       // degenerate cubic
	f.Add([]byte{4, 1, 1, 1, 1, 1, 1, 1, 1, 1}) // +Inf scale arc
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) != 8 && len(data) != 10 {
			return
		}
		vals := make([]float64, len(data))
		for i, b := range data {
			vals[i] = decodeFuzzFloat(b)
		}
		rec := &vertexRecorder{}
		outcome, recovered := runSample(sampleTimeBudget, func() {
			if len(vals) == 8 {
				// cubic curve: recursion depth is bounded by
				// CurveRecursionLimit, segment count by maxTraceSegments
				TraceCubic(rec, vals, 0.5)
				rec = &vertexRecorder{}
				TraceQuad(rec, vals, 0.5)
			} else {
				// arc: x, y, rx, ry, start, angle, scale
				TraceArc(rec, vals[2], vals[3], vals[4], vals[5], vals[6], vals[7], vals[8])
			}
		})
		checkOutcome(t, outcome, recovered, "TraceCurves")
		if rec.overflow {
			t.Fatalf("TraceCurves: emitted more than %d vertices", maxRecordedVertices)
		}
	})
}

// TestFuzzSeedCorpusIsReproducible re-runs every seed corpus entry of every
// fuzz target through the same harness to document that the regression
// samples are deterministic: two runs must produce identical vertex streams.
func TestFuzzSeedCorpusIsReproducible(t *testing.T) {
	seeds := [][]byte{
		{4, 4, 1, 1, 1, 1, 1, 1},
		{1, 4, 1, 1, 1, 1, 1, 6},
		{1, 0, 1, 1, 3, 3, 3, 3, 3, 3, 3},
		{1, 3, 1, 1, 1, 1, 1, 1},
	}
	for _, seed := range seeds {
		run := func() []float64 {
			rec := &vertexRecorder{}
			Flatten(pathFromFuzzBytes(seed[1:]), rec, decodeFuzzFloat(seed[0]))
			return rec.points
		}
		first, second := run(), run()
		if len(first) != len(second) {
			t.Fatalf("seed %v: non-deterministic output: %d vs %d points", seed, len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("seed %v: non-deterministic output at %d: %v vs %v", seed, i, first[i], second[i])
			}
		}
	}
}
