package gnata

import (
	"encoding/json"

	"github.com/recolabs/gnata/internal/rawjson"
)

// walkPureSteps resolves a chain of pure field-name path steps against a
// rawjson document, auto-mapping through arrays at every step exactly like the
// full evaluator does, without ever unmarshaling the document. Returns
// (value, false) when the walker cannot represent the result (a step landed
// on a scalar, or the field is genuinely absent) — the caller should fall
// back to full evaluation in that case.
//
// Mirrors evalName's []any case in internal/evaluator/eval_helpers.go on
// decoded values — keep the two in sync; see the note on evalName.
func walkPureSteps(steps []string, root *rawjson.Result) (any, bool) {
	var cur any = *root
	for _, step := range steps {
		next, ok := stepValue(step, cur)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return finalizeStepValue(cur), true
}

func stepValue(step string, cur any) (any, bool) {
	switch v := cur.(type) {
	case rawjson.Result:
		return stepSingle(step, &v)
	case []rawjson.Result:
		return stepArray(step, v)
	default:
		return nil, false
	}
}

func stepSingle(step string, r *rawjson.Result) (any, bool) {
	switch {
	case r.IsObject():
		// A literal-key scan via ForEach with an early exit: the first
		// member of that name, without building the full key/value map
		// just to read one entry.
		var val rawjson.Result
		found := false
		r.ForEach(func(key, value rawjson.Result) bool {
			if key.Str == step {
				val, found = value, true
				return false
			}
			return true
		})
		if !found {
			return nil, false
		}
		return val, true
	case r.IsArray():
		return stepArray(step, r.Array())
	default:
		return nil, false
	}
}

// stepArray applies a field-lookup step across every element of an array,
// flattening one level of nested-array results into the output — matching
// JSONata's array auto-mapping semantics.
func stepArray(step string, arr []rawjson.Result) (any, bool) {
	flat := make([]rawjson.Result, 0, len(arr))
	fieldFound := false
	for i := range arr {
		val, ok := stepSingle(step, &arr[i])
		if !ok {
			continue
		}
		fieldFound = true
		switch inner := val.(type) {
		case rawjson.Result:
			if inner.IsArray() {
				flat = append(flat, inner.Array()...)
			} else {
				flat = append(flat, inner)
			}
		case []rawjson.Result:
			flat = append(flat, inner...)
		}
	}
	switch {
	case len(flat) == 0 && fieldFound:
		return []rawjson.Result{}, true
	case len(flat) == 0:
		return nil, false
	case len(flat) == 1:
		return flat[0], true
	default:
		return flat, true
	}
}

func finalizeStepValue(cur any) any {
	switch v := cur.(type) {
	case rawjson.Result:
		return rawValueToAny(&v)
	case []rawjson.Result:
		out := make([]any, len(v))
		for i := range v {
			out[i] = rawValueToAny(&v[i])
		}
		return out
	default:
		return nil
	}
}

// walkPureStepsBytes resolves steps against raw JSON bytes. Returns
// (value, false) when the caller should fall back to full evaluation.
func walkPureStepsBytes(steps []string, data []byte) (any, bool) {
	root := rawjson.Parse(data)
	if !root.Exists() {
		return nil, false
	}
	return walkPureSteps(steps, &root)
}

// walkPureStepsMapBytes resolves steps against a map of top-level field names
// to raw JSON values (EvalMap's input shape). The first step is an O(1) map
// lookup by field name; remaining steps walk rawjson.Result the same way as
// walkPureStepsBytes. Returns (value, false) when the caller should fall
// back to full evaluation.
func walkPureStepsMapBytes(steps []string, mapData map[string]json.RawMessage) (any, bool) {
	root, rest, ok := firstStepFromMap(steps, mapData)
	if !ok {
		return nil, false
	}
	return walkPureSteps(rest, &root)
}

// walkPureStepsValues resolves steps against raw JSON bytes and returns the
// leaf values as a []rawjson.Result, for callers (aggregate and any-match fast
// paths) that need to reduce over the raw values themselves rather than a
// finalized Go value. A single scalar result is returned as a one-element
// slice; an array-typed result is exploded into its elements. ok is false
// when the walker cannot represent the result.
func walkPureStepsValues(steps []string, data []byte) (values []rawjson.Result, ok bool) {
	root := rawjson.Parse(data)
	if !root.Exists() {
		return nil, false
	}
	return walkPureStepsValuesFrom(steps, &root)
}

// walkPureStepsMapValues is walkPureStepsValues for EvalMap's input shape.
func walkPureStepsMapValues(steps []string, mapData map[string]json.RawMessage) (values []rawjson.Result, ok bool) {
	root, rest, firstOK := firstStepFromMap(steps, mapData)
	if !firstOK {
		return nil, false
	}
	return walkPureStepsValuesFrom(rest, &root)
}

// firstStepFromMap resolves the first path step via an O(1) map lookup,
// returning the parsed remainder as a rawjson.Result root plus the remaining
// steps to walk from there.
func firstStepFromMap(steps []string, mapData map[string]json.RawMessage) (root rawjson.Result, rest []string, ok bool) {
	if len(steps) == 0 || mapData == nil {
		return rawjson.Result{}, nil, false
	}
	raw, exists := mapData[steps[0]]
	if !exists {
		return rawjson.Result{}, nil, false
	}
	root = rawjson.Parse(raw)
	if !root.Exists() {
		return rawjson.Result{}, nil, false
	}
	return root, steps[1:], true
}

func walkPureStepsValuesFrom(steps []string, root *rawjson.Result) (values []rawjson.Result, ok bool) {
	var cur any = *root
	for _, step := range steps {
		next, stepOK := stepValue(step, cur)
		if !stepOK {
			return nil, false
		}
		cur = next
	}
	switch v := cur.(type) {
	case []rawjson.Result:
		return v, true
	case rawjson.Result:
		if v.IsArray() {
			return v.Array(), true
		}
		return []rawjson.Result{v}, true
	default:
		return nil, false
	}
}
