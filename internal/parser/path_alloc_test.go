package parser_test

import (
	"strings"
	"testing"

	"github.com/recolabs/gnata/internal/parser"
)

// longPath is a dotted path of n steps, mixing names and quoted names.
func longPath(n int) string {
	steps := make([]string, n)
	for i := range steps {
		if i%4 == 3 {
			steps[i] = "`step " + strings.Repeat("x", i%3) + "`"
		} else {
			steps[i] = "step" + strings.Repeat("x", i%3)
		}
	}
	return strings.Join(steps, ".")
}

func processedPath(t testing.TB, src string) *parser.Node {
	node, err := parser.NewParser(src).Parse()
	if err != nil {
		t.Fatalf("parse(%q): %v", src, err)
	}
	node, err = parser.ProcessAST(node)
	if err != nil {
		t.Fatalf("processAST(%q): %v", src, err)
	}
	return node
}

// Flattening a dotted path collects its steps into one slice: the
// allocations of parsing and processing an n-step path grow by a bounded
// number per step, not with the slices built and copied at every level of
// the nested binary nodes.
func TestPathFlatteningAllocations(t *testing.T) {
	for _, n := range []int{8, 32, 128} {
		src := longPath(n)
		node := processedPath(t, src)
		if node.Type != parser.NodePath || len(node.Steps) != n {
			t.Fatalf("%d steps: got type %s with %d steps", n, node.Type, len(node.Steps))
		}
		allocs := testing.AllocsPerRun(20, func() { processedPath(t, src) })
		perStep := allocs / float64(n)
		t.Logf("%d steps: %.0f allocations (%.2f per step)", n, allocs, perStep)
		if perStep > 4 {
			t.Fatalf("%d steps: %.2f allocations per step, want at most 4", n, perStep)
		}
	}
}

func BenchmarkPathFlattening(b *testing.B) {
	src := longPath(64)
	b.ReportAllocs()
	for b.Loop() {
		processedPath(b, src)
	}
}
