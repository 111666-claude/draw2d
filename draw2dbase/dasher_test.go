// Copyright 2010 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"math"
	"testing"
)

// recordingFlattener records the emitted path commands.
type recordingFlattener struct {
	moves []int
	lines []int
	ends  int
}

func (r *recordingFlattener) MoveTo(x, y float64) { r.moves = append(r.moves, len(r.lines)) }
func (r *recordingFlattener) LineTo(x, y float64) { r.lines = append(r.lines, 1) }
func (r *recordingFlattener) LineJoin()           {}
func (r *recordingFlattener) Close()              {}
func (r *recordingFlattener) End()                { r.ends++ }

// dashLine runs one horizontal segment of the given length through a
// freshly initialized dasher and returns the number of emitted line
// pieces (dashes) and subpath restarts (gaps).
func dashLine(t *testing.T, dash []float64, offset float64, length float64) (linePieces, gapMoves int) {
	t.Helper()
	rec := &recordingFlattener{}
	d := NewDashConverter(dash, offset, rec)
	d.MoveTo(0, 0)
	d.LineTo(length, 0)
	d.End()
	// the first move opens the subpath, every gap adds another MoveTo
	return len(rec.lines), len(rec.moves) - 1
}

func TestNormalizeDashPattern(t *testing.T) {
	// solid cases
	for _, dash := range [][]float64{nil, {}, {0}, {0, 0}} {
		got, off, dashed, err := NormalizeDashPattern(dash, 2)
		if err != nil || dashed || off != 0 || len(got) != 0 {
			t.Errorf("NormalizeDashPattern(%v) = %v,%v,%v,%v, want solid", dash, got, off, dashed, err)
		}
	}
	// odd length duplicated
	got, off, dashed, err := NormalizeDashPattern([]float64{3, 5, 7}, -3)
	if err != nil || !dashed {
		t.Fatalf("unexpected err=%v dashed=%v", err, dashed)
	}
	// period becomes 30, offset -3 is reduced to 27
	if len(got) != 6 || off != 27 {
		t.Errorf("got pattern=%v offset=%v, want 6 entries offset 27", got, off)
	}
	// offset reduction
	_, off, _, _ = NormalizeDashPattern([]float64{4, 6}, 23)
	if off != 3 {
		t.Errorf("offset 23 reduced to %v, want 3", off)
	}
	// invalid input
	if _, _, _, err := NormalizeDashPattern([]float64{-1, 2}, 0); err == nil {
		t.Error("negative length must error")
	}
	if _, _, _, err := NormalizeDashPattern([]float64{1, 2}, math.NaN()); err == nil {
		t.Error("NaN offset must error")
	}
}

func TestDasherStraightLine(t *testing.T) {
	// pattern [20,10] over 300 units starting at 0: 10 dashes
	pieces, gaps := dashLine(t, []float64{20, 10}, 0, 300)
	if pieces != 10 || gaps != 9 {
		t.Errorf("got %d pieces, %d gaps; want 10 pieces, 9 gaps", pieces, gaps)
	}
	// solid fallback for invalid pattern
	pieces, gaps = dashLine(t, []float64{-1}, 0, 100)
	if pieces != 1 || gaps != 0 {
		t.Errorf("invalid pattern fallback got %d pieces, %d gaps", pieces, gaps)
	}
}

func TestDasherOddLength(t *testing.T) {
	// [15,8,5] is normalized to [15,8,5,15,8,5], period 56 with an
	// on/off schedule of 15 on, 8 off, 5 on, 15 on, 8 off, 5 off.
	// The two adjacent on entries (5 and 15) are emitted as separate
	// line pieces; the final 5 long gap ends exactly at the path end so
	// it does not restart the subpath.
	pieces, gaps := dashLine(t, []float64{15, 8, 5}, 0, 56)
	if pieces != 3 || gaps != 2 {
		t.Errorf("odd pattern over one period: got %d pieces, %d gaps; want 3/2", pieces, gaps)
	}
}

func TestDasherNegativeOffset(t *testing.T) {
	// [15,8,5] period 56, offset -11 -> phase 45 (entry 4, 2 units
	// consumed). The resulting schedule starts with a 6 long on piece,
	// then off/on/off/on/off/on; the last gap ends before the path end.
	rec := &recordingFlattener{}
	d := NewDashConverter([]float64{15, 8, 5}, -11, rec)
	d.MoveTo(0, 0)
	d.LineTo(56, 0)
	d.End()
	// internal gaps at 6-11 and 26-34; the gap at 39-54 is followed by
	// another dash, so it also restarts the subpath: 3 gap moves
	if got := len(rec.moves) - 1; got != 3 {
		t.Errorf("expected 3 internal gap restarts, got %d (moves=%v)", got, rec.moves)
	}
}

func TestDasherSubpathRestart(t *testing.T) {
	// each subpath restarts the pattern at the same offset; pattern
	// [20,10] at offset 7 over 40 units has one internal gap per subpath
	rec := &recordingFlattener{}
	d := NewDashConverter([]float64{20, 10}, 7, rec)
	d.MoveTo(0, 0)
	d.LineTo(40, 0)
	d.MoveTo(0, 50)
	d.LineTo(40, 50)
	d.End()
	// 2 subpath starts + one gap move in each subpath = 4 moves
	if len(rec.moves) != 4 {
		t.Errorf("expected 4 MoveTo (2 subpaths + 2 gaps), got %d", len(rec.moves))
	}
}

func TestDasherZeroLengthSegment(t *testing.T) {
	// zero length segments must not advance the pattern
	rec := &recordingFlattener{}
	d := NewDashConverter([]float64{10, 5}, 0, rec)
	d.MoveTo(0, 0)
	d.LineTo(0, 0)
	d.LineTo(20, 0)
	d.End()
	if len(rec.lines) != 2 {
		t.Errorf("expected 2 dash pieces, got %d", len(rec.lines))
	}
}

func TestDasherSolidFallback(t *testing.T) {
	// invalid and empty patterns must behave like a solid line: every
	// LineTo is forwarded verbatim with no gap restarts
	for _, dash := range [][]float64{nil, {}, {0, 0}, {-1, 2}} {
		rec := &recordingFlattener{}
		d := NewDashConverter(dash, 0, rec)
		d.MoveTo(0, 0)
		d.LineTo(10, 0)
		d.LineJoin()
		d.LineTo(10, 10)
		d.LineTo(0, 10)
		d.Close()
		d.End()
		if len(rec.lines) != 3 {
			t.Errorf("pattern %v: got %d line pieces, want 3 (solid)", dash, len(rec.lines))
		}
		if len(rec.moves) != 1 {
			t.Errorf("pattern %v: got %d moves, want 1 (no gap restarts)", dash, len(rec.moves))
		}
	}
}
