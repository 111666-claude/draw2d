// Copyright 2026 The draw2d Authors. All rights reserved.

package boolop_test

import (
	"fmt"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/boolop"
)

// ExampleBoolean cuts a circular hole out of a badge shape and fills the
// resulting path again.
func ExampleBoolean() {
	badge := new(draw2d.Path)
	badge.MoveTo(0, 0)
	badge.LineTo(100, 0)
	badge.LineTo(100, 100)
	badge.LineTo(0, 100)
	badge.Close()

	hole := new(draw2d.Path)
	hole.ArcTo(50, 50, 20, 20, 0, 2*3.141592653589793)
	hole.Close()

	outline, err := boolop.Boolean(boolop.Difference, badge, hole, nil)
	if err != nil {
		panic(err)
	}
	// The result is an ordinary path: an outer ring and a hole ring, ready
	// to be filled by any draw2d backend.
	fmt.Println("subpaths:", countSubpaths(outline))
	// Output:
	// subpaths: 2
}

func countSubpaths(p *draw2d.Path) int {
	n := 0
	for _, c := range p.Components {
		if c == draw2d.MoveToCmp {
			n++
		}
	}
	return n
}
