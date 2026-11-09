package gnata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/recolabs/gnata"
	"github.com/recolabs/gnata/internal/parser"
)

// suiteExpressions returns the expressions of the conformance suite that
// compile, in a fixed order.
func suiteExpressions(t *testing.T) []string {
	t.Helper()
	var out []string
	groups, err := os.ReadDir(filepath.Join("testdata", "groups"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		dir := filepath.Join("testdata", "groups", g.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var cases []struct {
				Expr     string `json:"expr"`
				ExprFile string `json:"expr-file"`
			}
			if raw[0] != '[' {
				raw = append(append([]byte{'['}, raw...), ']')
			}
			if err := json.Unmarshal(raw, &cases); err != nil {
				t.Fatal(err)
			}
			for _, c := range cases {
				expr := c.Expr
				if c.ExprFile != "" {
					b, err := os.ReadFile(filepath.Join(dir, c.ExprFile))
					if err != nil {
						continue
					}
					expr = string(b)
				}
				if _, err := gnata.Compile(expr); err == nil {
					out = append(out, expr)
				}
			}
		}
	}
	return out
}

// TestASTNodeSize keeps a tree node in the 144-byte size class: the fields
// most nodes use are inline, the rest behind one pointer.
func TestASTNodeSize(t *testing.T) {
	if size := unsafe.Sizeof(parser.Node{}); size > 144 {
		t.Errorf("parser.Node is %d bytes, want at most 144", size)
	}
}

// TestCompileAllocatesTheTreeOnce bounds the bytes Compile allocates for the
// expressions of the conformance suite. Almost all of them are the tree,
// which the compiled expression keeps for its lifetime.
func TestCompileAllocatesTheTreeOnce(t *testing.T) {
	exprs := suiteExpressions(t)
	if len(exprs) < 1500 {
		t.Fatalf("%d expressions compile, want at least 1500", len(exprs))
	}
	keep := make([]*gnata.Expression, len(exprs))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i, expr := range exprs {
		e, err := gnata.Compile(expr)
		if err != nil {
			t.Fatal(err)
		}
		keep[i] = e
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(keep)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("%d expressions: %d B, %d objects", len(exprs), allocated, after.Mallocs-before.Mallocs)
	if limit := uint64(2_700_000); allocated > limit {
		t.Errorf("compiling %d expressions allocated %d B, want at most %d", len(exprs), allocated, limit)
	}
}
