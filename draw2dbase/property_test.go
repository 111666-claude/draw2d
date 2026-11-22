// Copyright 2026 The draw2d Authors. All rights reserved.
// Fixed-seed property tests for the flattening pipeline and the graphic
// context state stack. The references (hand-computed golden outputs,
// algebraic invariants) do not depend on implementation details.

package draw2dbase

import (
	"math"
	"math/rand"
	"runtime"
	"testing"

	"github.com/llgcode/draw2d"
)

// genFiniteMatrix builds a random finite matrix, frequently singular.
func genFiniteMatrix(r *rand.Rand) draw2d.Matrix {
	finitePool := []float64{0, 1, -1, 0.5, -0.5, 2, 10, -10, 1e6, -1e6, 1e-6}
	v := func() float64 { return finitePool[r.Intn(len(finitePool))] }
	switch r.Intn(5) {
	case 0:
		return draw2d.NewIdentityMatrix()
	case 1: // singular: zero determinant
		a, b := v(), v()
		return draw2d.Matrix{a, b, a, b, v(), v()}
	case 2: // singular: zero matrix
		return draw2d.Matrix{}
	default:
		return draw2d.Matrix{v(), v(), v(), v(), v(), v()}
	}
}

// TestPropertyPipelineFixedSeed pushes hostile fixed-seed samples through the
// whole pipeline (Flatten -> Dasher -> LineStroker -> Transformer) and
// checks, per sample: no panic, bounded time, bounded vertex count, bounded
// temporary memory and no NaN/Inf vertex.
func TestPropertyPipelineFixedSeed(t *testing.T) {
	r := rand.New(rand.NewSource(660))
	const samples = 300
	const maxAllocPerSample = 128 << 20 // 128 MiB of temporary memory
	for sample := 0; sample < samples; sample++ {
		p := genPath(r, 1+r.Intn(24), sample%3 == 0)
		dash := genDash(r)
		offset := genFloat(r)
		halfWidth := genFloat(r)
		tr := genFiniteMatrix(r)
		scale := genFloat(r)

		rec := &vertexRecorder{}
		var memBefore runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		outcome, recovered := runSample(sampleTimeBudget, func() {
			var sink Flattener = Transformer{Tr: tr, Flattener: rec}
			stroker := NewLineStroker(draw2d.RoundCap, draw2d.RoundJoin, sink)
			stroker.HalfLineWidth = halfWidth
			var liner Flattener = stroker
			if dash != nil {
				liner = NewDashConverter(dash, offset, stroker)
			}
			Flatten(p, liner, scale)
		})
		checkOutcome(t, outcome, recovered, "pipeline")
		checkRecorder(t, rec, "pipeline")
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)
		if memAfter.TotalAlloc-memBefore.TotalAlloc > maxAllocPerSample {
			t.Fatalf("sample %d: temporary memory exceeded: %d bytes", sample, memAfter.TotalAlloc-memBefore.TotalAlloc)
		}
	}
}

// TestPropertyStrokerNoStateLeak checks that LineStroker.End resets its
// internal buffers: stroking a second path with the same stroker must give
// the same output as stroking it with a fresh stroker.
func TestPropertyStrokerNoStateLeak(t *testing.T) {
	r := rand.New(rand.NewSource(661))
	for sample := 0; sample < 50; sample++ {
		p1 := genPath(r, 1+r.Intn(8), false)
		p2 := genPath(r, 1+r.Intn(8), false)
		width := 0.5 + r.Float64()*10

		reused := &vertexRecorder{}
		stroker := NewLineStroker(draw2d.RoundCap, draw2d.RoundJoin, reused)
		stroker.HalfLineWidth = width
		Flatten(p1, stroker, 1)
		before := len(reused.points)
		Flatten(p2, stroker, 1)
		secondRun := reused.points[before:]

		fresh := &vertexRecorder{}
		freshStroker := NewLineStroker(draw2d.RoundCap, draw2d.RoundJoin, fresh)
		freshStroker.HalfLineWidth = width
		Flatten(p2, freshStroker, 1)

		if len(secondRun) != len(fresh.points) {
			t.Fatalf("sample %d: state leak: reused stroker emitted %d values, fresh one %d",
				sample, len(secondRun), len(fresh.points))
		}
		for i := range secondRun {
			if secondRun[i] != fresh.points[i] {
				t.Fatalf("sample %d: state leak: reused stroker output differs at %d: %v vs %v",
					sample, i, secondRun[i], fresh.points[i])
			}
		}
	}
}

// TestPropertySaveRestore verifies the Close/Save/Restore invariants of the
// graphic context state stack.
func TestPropertySaveRestore(t *testing.T) {
	gc := NewStackGraphicContext()
	gc.SetLineWidth(3.5)
	gc.Translate(10, 20)
	gc.SetFillRule(draw2d.FillRuleWinding)
	gc.MoveTo(1, 2)
	gc.LineTo(3, 4)
	savedTr := gc.Current.Tr
	savedPath := gc.GetPath()

	gc.Save()
	// mutate everything that Save must protect
	gc.SetLineWidth(99)
	gc.Scale(5, 5)
	gc.SetFillRule(draw2d.FillRuleEvenOdd)
	gc.MoveTo(7, 8)
	gc.LineTo(9, 10)
	gc.Restore()

	if gc.Current.Tr != savedTr {
		t.Fatalf("Restore did not bring back the transform: %v != %v", gc.Current.Tr, savedTr)
	}
	if gc.Current.LineWidth != 3.5 {
		t.Fatalf("Restore did not bring back the line width: %v", gc.Current.LineWidth)
	}
	if gc.Current.FillRule != draw2d.FillRuleWinding {
		t.Fatalf("Restore did not bring back the fill rule: %v", gc.Current.FillRule)
	}
	restored := gc.GetPath()
	if len(restored.Components) != len(savedPath.Components) || len(restored.Points) != len(savedPath.Points) {
		t.Fatalf("Restore did not bring back the path: %v vs %v", restored, savedPath)
	}
	for i := range restored.Points {
		if restored.Points[i] != savedPath.Points[i] {
			t.Fatalf("Restore: path point %d differs: %v vs %v", i, restored.Points[i], savedPath.Points[i])
		}
	}
}

// TestPropertyRestoreAtRootIsNoOp: Restore without a matching Save must not
// panic nor corrupt the current state.
func TestPropertyRestoreAtRootIsNoOp(t *testing.T) {
	gc := NewStackGraphicContext()
	gc.SetLineWidth(2)
	gc.Restore()
	gc.Restore()
	if gc.Current.LineWidth != 2 {
		t.Fatalf("Restore at root corrupted state: %v", gc.Current.LineWidth)
	}
}

// TestPropertySaveRestoreBalanced: a fixed-seed random sequence of balanced
// Save/Restore pairs must return to the exact initial state.
func TestPropertySaveRestoreBalanced(t *testing.T) {
	r := rand.New(rand.NewSource(662))
	gc := NewStackGraphicContext()
	initialTr := gc.Current.Tr
	depth := 0
	for i := 0; i < 200; i++ {
		switch r.Intn(4) {
		case 0:
			gc.Save()
			depth++
		case 1:
			if depth > 0 {
				gc.Restore()
				depth--
			}
		case 2:
			// mutations are only undone by Restore when inside a
			// Save/Restore pair
			if depth > 0 {
				gc.Translate(genFloat(r), genFloat(r))
			}
		case 3:
			if depth > 0 {
				gc.SetLineWidth(genFloat(r))
			}
		}
	}
	for ; depth > 0; depth-- {
		gc.Restore()
	}
	if gc.Current.Tr != initialTr {
		t.Fatalf("balanced Save/Restore did not restore the initial transform: %v != %v", gc.Current.Tr, initialTr)
	}
}

// TestPropertyCloseReturnsToSubpathStart: Close must emit a segment back to
// the most recent MoveTo point.
func TestPropertyCloseReturnsToSubpathStart(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(1, 1)
	p.LineTo(5, 1)
	p.LineTo(3, 4)
	p.Close()
	rec := &vertexRecorder{}
	Flatten(p, rec, 1)
	if rec.closes != 1 {
		t.Fatalf("expected 1 Close, got %d", rec.closes)
	}
	n := len(rec.points)
	if n < 4 {
		t.Fatalf("flattened closed triangle too short: %v", rec.points)
	}
	if rec.points[n-2] != 1 || rec.points[n-1] != 1 {
		t.Fatalf("Close did not return to the subpath start: last point (%v, %v)",
			rec.points[n-2], rec.points[n-1])
	}
}

// TestGoldenFlattenLinePath: flattening a pure line path is the identity on
// its vertices (reference computed by hand, not from the implementation).
func TestGoldenFlattenLinePath(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(1, 2)
	p.LineTo(3, 4)
	p.LineTo(5, 6)
	rec := &vertexRecorder{}
	Flatten(p, rec, 1)
	want := []float64{1, 2, 3, 4, 5, 6}
	if len(rec.points) != len(want) {
		t.Fatalf("got %v, want %v", rec.points, want)
	}
	for i := range want {
		if rec.points[i] != want[i] {
			t.Fatalf("got %v, want %v", rec.points, want)
		}
	}
	if rec.moves != 1 || rec.lines != 2 || rec.ends != 1 {
		t.Fatalf("unexpected event counts: moves=%d lines=%d ends=%d", rec.moves, rec.lines, rec.ends)
	}
}

// TestGoldenDasher: dash pattern [2 2] over the segment (0,0)->(10,0) must
// produce the "on" segments [0,2], [4,6] and [8,10] (hand-computed).
func TestGoldenDasher(t *testing.T) {
	rec := &vertexRecorder{}
	dasher := NewDashConverter([]float64{2, 2}, 0, rec)
	dasher.MoveTo(0, 0)
	dasher.LineTo(10, 0)
	dasher.End()
	want := []float64{0, 0, 2, 0, 4, 0, 6, 0, 8, 0, 10, 0, 10, 0}
	if len(rec.points) != len(want) {
		t.Fatalf("got %v, want %v", rec.points, want)
	}
	for i := range want {
		if rec.points[i] != want[i] {
			t.Fatalf("got %v, want %v", rec.points, want)
		}
	}
}

// TestGoldenStroker: stroking the segment (0,0)->(10,0) with line width 2
// must outline the rectangle (0,-1) (10,-1) (10,1) (0,1): the normal is
// (dy, -dx)/d * halfWidth = (0, -1) (hand-computed reference).
func TestGoldenStroker(t *testing.T) {
	rec := &vertexRecorder{}
	stroker := NewLineStroker(draw2d.ButtCap, draw2d.BevelJoin, rec)
	stroker.HalfLineWidth = 1
	stroker.MoveTo(0, 0)
	stroker.LineTo(10, 0)
	stroker.End()
	want := []float64{0, -1, 10, -1, 10, 1, 0, 1, 0, -1}
	if len(rec.points) != len(want) {
		t.Fatalf("got %v, want %v", rec.points, want)
	}
	for i := range want {
		if rec.points[i] != want[i] {
			t.Fatalf("got %v, want %v", rec.points, want)
		}
	}
}

// TestGoldenCurveEndpoints: whatever the subdivision, a flattened curve must
// end exactly on the curve end point and stay finite.
func TestGoldenCurveEndpoints(t *testing.T) {
	cubic := []float64{0, 0, 1, 2, 3, 2, 4, 0}
	rec := &vertexRecorder{}
	rec.MoveTo(cubic[0], cubic[1])
	if err := TraceCubic(rec, cubic, 0.5); err != nil {
		t.Fatal(err)
	}
	n := len(rec.points)
	if rec.points[n-2] != 4 || rec.points[n-1] != 0 {
		t.Fatalf("cubic does not end on its end point: %v", rec.points)
	}
	if rec.nonFinite {
		t.Fatal("cubic emitted a non-finite vertex")
	}

	quad := []float64{0, 0, 2, 3, 4, 0}
	rec2 := &vertexRecorder{}
	rec2.MoveTo(quad[0], quad[1])
	if err := TraceQuad(rec2, quad, 0.5); err != nil {
		t.Fatal(err)
	}
	n = len(rec2.points)
	if rec2.points[n-2] != 4 || rec2.points[n-1] != 0 {
		t.Fatalf("quad does not end on its end point: %v", rec2.points)
	}
}

// --- Reproducible regression samples -------------------------------------
// Each of these used to crash, hang or emit NaN before the hardening. They
// are also encoded as fuzz seed corpus entries.

func TestRegressionZeroDashPattern(t *testing.T) {
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		dasher := NewDashConverter([]float64{0}, 0, rec)
		dasher.MoveTo(0, 0)
		dasher.LineTo(10, 10)
		dasher.End()
	})
	checkOutcome(t, outcome, recovered, "zero dash")
	checkRecorder(t, rec, "zero dash")
}

func TestRegressionNegativeDashPattern(t *testing.T) {
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		dasher := NewDashConverter([]float64{1, -1}, 0, rec)
		dasher.MoveTo(0, 0)
		dasher.LineTo(10, 10)
		dasher.End()
	})
	checkOutcome(t, outcome, recovered, "negative dash")
	checkRecorder(t, rec, "negative dash")
}

func TestRegressionHugeDashOffset(t *testing.T) {
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		dasher := NewDashConverter([]float64{1, 1}, 1e18, rec)
		dasher.MoveTo(0, 0)
		dasher.LineTo(10, 0)
		dasher.End()
	})
	checkOutcome(t, outcome, recovered, "huge dash offset")
	checkRecorder(t, rec, "huge dash offset")
}

func TestRegressionInfiniteFlatteningScale(t *testing.T) {
	p := new(draw2d.Path)
	p.ArcTo(5, 5, 3, 3, 0, math.Pi)
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		Flatten(p, rec, math.Inf(1))
	})
	checkOutcome(t, outcome, recovered, "infinite scale arc")
	checkRecorder(t, rec, "infinite scale arc")
}

func TestRegressionHugeArcSweep(t *testing.T) {
	p := new(draw2d.Path)
	p.ArcTo(0, 0, 1, 1, 0, 1e308)
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		Flatten(p, rec, 1)
	})
	checkOutcome(t, outcome, recovered, "huge arc sweep")
	checkRecorder(t, rec, "huge arc sweep")
}

func TestRegressionNaNCubicNoNaNVertices(t *testing.T) {
	p := new(draw2d.Path)
	p.MoveTo(0, 0)
	p.CubicCurveTo(math.NaN(), math.NaN(), 1, 1, 2, 2)
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		Flatten(p, rec, 1)
	})
	checkOutcome(t, outcome, recovered, "NaN cubic")
	checkRecorder(t, rec, "NaN cubic")
}

func TestRegressionMalformedComponents(t *testing.T) {
	// curve component without a current point, and a truncated point stream
	p := &draw2d.Path{
		Components: []draw2d.PathCmp{draw2d.CubicCurveToCmp, draw2d.LineToCmp},
		Points:     []float64{1, 2, 3},
	}
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		Flatten(p, rec, 1)
	})
	checkOutcome(t, outcome, recovered, "malformed components")
	checkRecorder(t, rec, "malformed components")
}

func TestRegressionNegativeLineWidth(t *testing.T) {
	rec := &vertexRecorder{}
	outcome, recovered := runSample(sampleTimeBudget, func() {
		stroker := NewLineStroker(draw2d.RoundCap, draw2d.RoundJoin, rec)
		stroker.HalfLineWidth = -5
		stroker.MoveTo(0, 0)
		stroker.LineTo(10, 0)
		stroker.End()
	})
	checkOutcome(t, outcome, recovered, "negative line width")
	checkRecorder(t, rec, "negative line width")
	// a negative width mirrors the outline but must still produce the
	// rectangle of a width-5 stroke (hand-computed reference)
	want := []float64{0, 5, 10, 5, 10, -5, 0, -5, 0, 5}
	if len(rec.points) != len(want) {
		t.Fatalf("got %v, want %v", rec.points, want)
	}
	for i := range want {
		if rec.points[i] != want[i] {
			t.Fatalf("got %v, want %v", rec.points, want)
		}
	}
}
