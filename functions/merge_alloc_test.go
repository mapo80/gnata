package functions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/recolabs/gnata/internal/evaluator"
)

// fnMergeBefore is $merge as it was before the result was sized and a
// first ordered object copied whole: the oracle of the members, their order
// and the refusals.
func fnMergeBefore(args []any, _ any) (any, error) {
	if len(args) == 0 || args[0] == nil {
		return nil, nil
	}
	arr, ok := args[0].([]any)
	if !ok {
		if evaluator.IsMap(args[0]) {
			return args[0], nil
		}
		return nil, &evaluator.JSONataError{Code: "T0410", Message: "$merge: argument must be an array of objects"}
	}
	result := evaluator.NewOrderedMap()
	for _, item := range arr {
		if !evaluator.IsMap(item) {
			return nil, &evaluator.JSONataError{Code: "T0412", Message: "$merge: array elements must be objects"}
		}
		evaluator.MapRange(item, func(k string, v any) bool {
			result.Set(k, v)
			return true
		})
	}
	return result, nil
}

func orderedOf(pairs ...any) *evaluator.OrderedMap {
	m := evaluator.NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1])
	}
	return m
}

func TestMergeMatchesTheMemberByMemberMerge(t *testing.T) {
	deleted := orderedOf("a", 1, "b", 2, "c", 3)
	deleted.Delete("b")
	cases := [][]any{
		nil,
		{nil},
		{orderedOf("a", 1)},
		{[]any{}},
		{[]any{orderedOf("z", 1, "a", 2), orderedOf("a", 3, "m", 4)}},
		{[]any{orderedOf("z", 1), map[string]any{"b": 2, "a": 1, "z": 9}}},
		{[]any{map[string]any{"b": 2, "a": 1}, orderedOf("c", 3, "a", 0)}},
		{[]any{deleted, orderedOf("b", 5)}},
		{[]any{orderedOf("x", []any{1, 2}), "no"}},
		{[]any{orderedOf("x", 1), orderedOf(), orderedOf("x", nil)}},
		{"no"},
	}
	for i, args := range cases {
		want, wantErr := fnMergeBefore(args, nil)
		got, err := fnMerge(args, nil)
		if fmt.Sprint(err) != fmt.Sprint(wantErr) {
			t.Fatalf("case %d: error %v, want %v", i, err, wantErr)
		}
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(gotJSON) != string(wantJSON) || reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Fatalf("case %d: %s, want %s", i, gotJSON, wantJSON)
		}
		if g, ok := got.(*evaluator.OrderedMap); ok && len(args) > 0 {
			if arr, ok := args[0].([]any); ok && len(arr) > 0 {
				if first, ok := arr[0].(*evaluator.OrderedMap); ok && g == first {
					t.Fatalf("case %d: the merge returned its first argument, not a copy", i)
				}
			}
		}
	}
}

// Merging a frame of many members with a few changes, as the generated
// controllers do at their steps.
func BenchmarkMergeFrame(b *testing.B) {
	frame := evaluator.NewOrderedMap()
	for i := range 200 {
		frame.Set(fmt.Sprintf("campo%03d", i), i)
	}
	args := []any{[]any{frame, orderedOf("campo007", "nuovo", "aggiunto", true)}}
	for _, c := range []struct {
		name  string
		merge func([]any, any) (any, error)
	}{{"prima", fnMergeBefore}, {"dopo", fnMerge}} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := c.merge(args, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
