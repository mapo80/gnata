package gnata_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/recolabs/gnata"
)

func TestAppendPreservesExplicitEmptyArray(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"top-level", `$append([],[])`, `[]`},
		{"object-member", `{"items":$append([],[])}`, `{"items":[]}`},
		{"existing-empty-arrays", `$append(a,b)`, `[]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			expression, err := gnata.Compile(test.source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := expression.Eval(context.Background(), map[string]any{"a": []any{}, "b": []any{}})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != test.want {
				t.Fatalf("%s = %s, want %s", test.source, encoded, test.want)
			}
		})
	}
}

func TestAppendPreservesUndefinedNullAndNestedValues(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"undefined-left", `$append(missing,[])`, `[]`},
		{"undefined-right", `$append([],missing)`, `[]`},
		{"null-left", `$append(null,[])`, `[null]`},
		{"null-right", `$append([],null)`, `[null]`},
		{"two-nulls", `$append(null,null)`, `[null,null]`},
		{"singleton", `$append([1],[2])`, `[1,2]`},
		{"scalar", `$append(1,[])`, `[1]`},
		{"nested", `$append([[1],[]],[[2]])`, `[[1],[],[2]]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			expression, err := gnata.Compile(test.source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := expression.Eval(context.Background(), map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != test.want {
				t.Fatalf("%s = %s, want %s", test.source, encoded, test.want)
			}
		})
	}
	expression, err := gnata.Compile(`$append(missing,alsoMissing)`)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := expression.Eval(context.Background(), map[string]any{}); err != nil || result != nil {
		t.Fatalf("undefined append changed: result=%#v err=%v", result, err)
	}
}

func TestAppendStillEnforcesSequenceBound(t *testing.T) {
	expression, err := gnata.Compile(`$append([1],[2])`, gnata.WithSequence(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expression.Eval(context.Background(), map[string]any{}); err == nil {
		t.Fatal("append exceeded its sequence limit")
	}
}
