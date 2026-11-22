// Copyright 2026 The draw2d Authors. All rights reserved.
// Shared harness for the geometry QA suite: bounded vertex recording,
// per-sample time/memory limits and deterministic generators of hostile
// geometry (NaN/Inf, huge coordinates, degenerate curves, bad dashes,
// singular matrices).

package draw2dbase

import (
	"math"
	"math/rand"
	"time"

	"github.com/llgcode/draw2d"
)

// maxRecordedVertices bounds how many vertices a single sample may emit.
// It caps the temporary memory a sample can use: a recorder never holds
// more than 2*maxRecordedVertices float64 (32 MiB). The bound covers the
// worst case of the pipeline: ~48 components x maxTraceSegments curve
// segments + maxDashTransitions dash transitions, amplified 2x by the
// LineStroker outline.
const maxRecordedVertices = 1 << 21

// sampleTimeBudget is the maximum wall-clock time one sample may take.
// Anything slower is reported as a suspected infinite loop.
const sampleTimeBudget = 2 * time.Second

// vertexRecorder is a Flattener that records emitted points, enforces a
// vertex cap and flags any non-finite coordinate it is handed.
type vertexRecorder struct {
	points    []float64
	overflow  bool // more than maxRecordedVertices vertices were emitted
	nonFinite bool // a NaN or ±Inf coordinate was emitted
	moves     int
	lines     int
	closes    int
	ends      int
}

func (r *vertexRecorder) add(x, y float64) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		r.nonFinite = true
	}
	if len(r.points)/2 >= maxRecordedVertices {
		r.overflow = true
		return
	}
	r.points = append(r.points, x, y)
}

func (r *vertexRecorder) MoveTo(x, y float64) { r.moves++; r.add(x, y) }
func (r *vertexRecorder) LineTo(x, y float64) { r.lines++; r.add(x, y) }
func (r *vertexRecorder) LineJoin()           {}
func (r *vertexRecorder) Close()              { r.closes++ }
func (r *vertexRecorder) End()                { r.ends++ }

// runOutcome describes how a sample run terminated.
type runOutcome int

const (
	runOK runOutcome = iota
	runPanicked
	runTimedOut
)

// runSample executes f with a wall-clock budget. It returns runPanicked
// (recovered) if f panics and runTimedOut if f does not finish within the
// budget — the two failure modes the QA harness must always detect.
func runSample(budget time.Duration, f func()) (outcome runOutcome, recovered any) {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case r := <-done:
		if r != nil {
			return runPanicked, r
		}
		return runOK, nil
	case <-time.After(budget):
		return runTimedOut, nil
	}
}

// interestingFloats is the pool of hostile values every generator draws
// from: NaN, ±Inf, huge and denormal coordinates mixed with benign ones.
var interestingFloats = []float64{
	0, 1, -1, 0.5, -0.5, 10, -10, 100,
	math.NaN(), math.Inf(1), math.Inf(-1),
	1e308, -1e308, 1e-300, -1e-300,
	math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64,
}

// genFloat returns a hostile float64 drawn from r.
func genFloat(r *rand.Rand) float64 {
	if r.Intn(2) == 0 {
		return interestingFloats[r.Intn(len(interestingFloats))]
	}
	// uniform in [-1e6, 1e6], occasionally scaled up
	v := (r.Float64()*2 - 1) * 1e6
	if r.Intn(8) == 0 {
		v *= 1e300
	}
	return v
}

// genDash builds a possibly-invalid dash pattern (bad lengths included).
func genDash(r *rand.Rand) []float64 {
	n := r.Intn(6)
	if n == 0 {
		return nil
	}
	dash := make([]float64, n)
	for i := range dash {
		dash[i] = genFloat(r)
	}
	return dash
}

// genPath builds a random path of at most n components. With malformed=true
// the component/point streams may be structurally inconsistent, as happens
// with externally imported paths.
func genPath(r *rand.Rand, n int, malformed bool) *draw2d.Path {
	p := new(draw2d.Path)
	for i := 0; i < n; i++ {
		switch r.Intn(7) {
		case 0:
			p.MoveTo(genFloat(r), genFloat(r))
		case 1:
			p.LineTo(genFloat(r), genFloat(r))
		case 2:
			p.QuadCurveTo(genFloat(r), genFloat(r), genFloat(r), genFloat(r))
		case 3:
			p.CubicCurveTo(genFloat(r), genFloat(r), genFloat(r), genFloat(r), genFloat(r), genFloat(r))
		case 4:
			p.ArcTo(genFloat(r), genFloat(r), genFloat(r), genFloat(r), genFloat(r), genFloat(r))
		case 5:
			p.Close()
		case 6:
			// structural noise: only in malformed mode
			if malformed {
				p.Components = append(p.Components, draw2d.PathCmp(r.Intn(8)))
			}
		}
	}
	if malformed && r.Intn(4) == 0 && len(p.Points) > 0 {
		// truncate or extend the point stream so it no longer matches the
		// component stream
		p.Points = p.Points[:r.Intn(len(p.Points)+1)]
	}
	return p
}

// checkRecorder verifies the post-conditions every sample must satisfy:
// no panic (checked by the caller via runSample), no NaN/Inf vertex and no
// vertex-count overflow.
func checkRecorder(t interface{ Fatalf(string, ...any) }, r *vertexRecorder, what string) {
	if r.nonFinite {
		t.Fatalf("%s: non-finite (NaN/Inf) vertex emitted", what)
	}
	if r.overflow {
		t.Fatalf("%s: emitted more than %d vertices", what, maxRecordedVertices)
	}
}
