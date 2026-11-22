// Copyright 2026 The draw2d Authors. All rights reserved.
// Fault-injection tests: they re-introduce the three classic defects
// (out-of-range indexing, infinite loop, state leak) into copies of the
// pipeline stages and prove the QA harness detects each of them. If any of
// these tests fail, the harness itself is blind and none of its other
// results can be trusted.

package draw2dbase

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/llgcode/draw2d"
)

// flattenOutOfRange is a copy of the pre-hardening Flatten: it indexes the
// point stream without bounds checks.
func flattenOutOfRange(path *draw2d.Path, flattener Flattener) {
	i := 0
	for _, cmp := range path.Components {
		switch cmp {
		case draw2d.MoveToCmp:
			flattener.MoveTo(path.Points[i], path.Points[i+1])
			i += 2
		case draw2d.CubicCurveToCmp:
			// panics with index out of range when the path starts with a
			// curve (i-2 == -2) or the point stream is truncated
			TraceCubic(flattener, path.Points[i-2:], 0.5)
			i += 6
		}
	}
}

func TestFaultInjectionDetectsOutOfRange(t *testing.T) {
	p := &draw2d.Path{
		Components: []draw2d.PathCmp{draw2d.CubicCurveToCmp},
		Points:     []float64{1, 2, 3, 4, 5, 6},
	}
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		flattenOutOfRange(p, rec)
	})
	if outcome != runPanicked {
		t.Fatalf("harness failed to detect the injected out-of-range panic (outcome=%v, recovered=%v)", outcome, recovered)
	}
	// and the hardened Flatten must survive the same input
	outcome, recovered = runSample(sampleTimeBudget, func() {
		Flatten(p, rec, 1)
	})
	if outcome != runOK {
		t.Fatalf("hardened Flatten did not survive the malformed path (outcome=%v, recovered=%v)", outcome, recovered)
	}
}

// dashLineToDeadLoop is a copy of the pre-hardening dasher inner loop: with
// a zero dash length it never advances. The abort flag only exists so the
// leaked goroutine can be cleaned up after the harness reported the timeout.
func dashLineToDeadLoop(next Flattener, dash []float64, x, y float64, abort *int32) {
	distance := 0.0
	currentDash := 0
	rest := dash[currentDash] - distance
	for rest < 0 {
		distance = distance - dash[currentDash]
		currentDash = (currentDash + 1) % len(dash)
		rest = dash[currentDash] - distance
	}
	d := distance2(0, 0, x, y)
	for d >= rest {
		if atomic.LoadInt32(abort) != 0 {
			return
		}
		k := rest / d
		lx := k * x
		ly := k * y
		next.LineTo(lx, ly)
		d = d - rest // rest == 0 with a zero dash: d never decreases
		currentDash = (currentDash + 1) % len(dash)
		rest = dash[currentDash]
	}
}

func distance2(x1, y1, x2, y2 float64) float64 {
	return vectorDistance(x2-x1, y2-y1)
}

func TestFaultInjectionDetectsDeadLoop(t *testing.T) {
	var abort int32
	faultyRec := &vertexRecorder{}
	outcome, _ := runSample(200*time.Millisecond, func() {
		dashLineToDeadLoop(faultyRec, []float64{0}, 10, 10, &abort)
	})
	atomic.StoreInt32(&abort, 1) // let the leaked goroutine exit
	if outcome != runTimedOut {
		t.Fatalf("harness failed to detect the injected infinite loop (outcome=%v)", outcome)
	}
	// and the hardened dasher must survive the same input
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		dasher := NewDashConverter([]float64{0}, 0, rec)
		dasher.MoveTo(0, 0)
		dasher.LineTo(10, 10)
		dasher.End()
	})
	if outcome != runOK {
		t.Fatalf("hardened dasher did not survive the zero dash (outcome=%v, recovered=%v)", outcome, recovered)
	}
}

// leakingStroker is a LineStroker variant whose End "forgets" to reset its
// vertex buffers, leaking state from one subpath into the next.
type leakingStroker struct {
	LineStroker
}

func (l *leakingStroker) End() {
	if len(l.vertices) > 1 {
		l.Flattener.MoveTo(l.vertices[0], l.vertices[1])
		for i, j := 2, 3; j < len(l.vertices); i, j = i+2, j+2 {
			l.Flattener.LineTo(l.vertices[i], l.vertices[j])
		}
	}
	l.Flattener.End()
	// injected defect: no reset of l.vertices / l.rewind
}

// verticesOfSecondRun is the invariant check used by
// TestPropertyStrokerNoStateLeak: stroke two paths, then verify the output
// of the second path matches a fresh stroker. It returns false when a state
// leak is detected.
func verticesOfSecondRun(t *testing.T, newStroker func(Flattener) Flattener) bool {
	t.Helper()
	mk := func() *draw2d.Path {
		p := new(draw2d.Path)
		p.MoveTo(1, 1)
		p.LineTo(5, 1)
		p.LineTo(5, 5)
		return p
	}
	p1, p2 := mk(), mk()
	p2.MoveTo(7, 7) // make p2 different from p1
	p2.LineTo(9, 7)

	reused := &vertexRecorder{}
	stroker := newStroker(reused)
	Flatten(p1, stroker, 1)
	before := len(reused.points)
	Flatten(p2, stroker, 1)
	secondRun := reused.points[before:]

	fresh := &vertexRecorder{}
	Flatten(p2, newStroker(fresh), 1)

	if len(secondRun) != len(fresh.points) {
		return false
	}
	for i := range secondRun {
		if secondRun[i] != fresh.points[i] {
			return false
		}
	}
	return true
}

func TestFaultInjectionDetectsStateLeak(t *testing.T) {
	leaking := verticesOfSecondRun(t, func(f Flattener) Flattener {
		return &leakingStroker{LineStroker: *NewLineStroker(draw2d.ButtCap, draw2d.BevelJoin, f)}
	})
	if leaking {
		t.Fatal("harness failed to detect the injected state leak")
	}
	// and the real stroker must pass the same invariant
	sane := verticesOfSecondRun(t, func(f Flattener) Flattener {
		return NewLineStroker(draw2d.ButtCap, draw2d.BevelJoin, f)
	})
	if !sane {
		t.Fatal("the real LineStroker leaks state between subpaths")
	}
}
