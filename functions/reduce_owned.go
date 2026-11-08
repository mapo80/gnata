package functions

import (
	"reflect"

	"github.com/recolabs/gnata/internal/evaluator"
	"github.com/recolabs/gnata/internal/parser"
)

// An accumulation is a $reduce whose function only grows its accumulator:
// function($acc, ...){ $append($acc, E) } or function($acc, ...){ $merge([$acc, E]) },
// E not reading $acc. Each step of the general $reduce copies the whole
// accumulator into a new array or object, so building a list or an object of
// n members costs n²/2 copies (the generated controllers build their step
// lists and visited sets this way: 85 MB of $append in a correction turn).
// The accumulator such a function returns is never seen by anyone but the
// next step, so the reduction can grow one array or object it owns,
// evaluating only E at each step: the same members in the same order.
type accumulation struct {
	merge  bool
	lambda *evaluator.Lambda
	part   *parser.Node
}

// plainVariable reports a bare $name reference.
func plainVariable(n *parser.Node, name string) bool {
	return n != nil && n.Type == parser.NodeVariable && n.Value == name && len(n.Stages) == 0 && n.Group == nil &&
		!n.KeepArray && !n.KeepSingletonArray && !n.ConsArray && n.Focus == "" && n.Index == "" && len(n.Steps) == 0
}

var (
	nodeType     = reflect.TypeFor[*parser.Node]()
	appendTarget = reflect.ValueOf(fnAppend).Pointer()
	mergeTarget  = reflect.ValueOf(fnMerge).Pointer()
)

// mentions reports whether any node reachable from n names variable name
// (a reference, a binding, an @ or # variable).
func mentions(n *parser.Node, name string) bool {
	found := false
	seen := map[*parser.Node]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		if found || !v.IsValid() {
			return
		}
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			if v.Type() == nodeType {
				node := v.Interface().(*parser.Node)
				if seen[node] {
					return
				}
				seen[node] = true
				if (node.Type == parser.NodeVariable && node.Value == name) || node.Focus == name || node.Index == name {
					found = true
					return
				}
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := range v.Len() {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(n))
	return found
}

func builtinIs(v any, target uintptr) bool {
	switch f := v.(type) {
	case evaluator.EnvAwareBuiltin:
		return reflect.ValueOf(f).Pointer() == target
	case evaluator.BuiltinFunction:
		return reflect.ValueOf(f).Pointer() == target
	}
	return false
}

// accumulationOf recognizes an accumulating reduce function, or nil.
func accumulationOf(fn any) *accumulation {
	l, ok := fn.(*evaluator.Lambda)
	if !ok || l.Sig != "" || len(l.Params) < 2 || l.Body == nil {
		return nil
	}
	body, acc := l.Body, l.Params[0]
	if body.Type != parser.NodeFunction || body.KeepArray || body.Group != nil || len(body.Stages) != 0 || body.Procedure == nil ||
		body.Procedure.Type != parser.NodeVariable || len(body.Procedure.Stages) != 0 || body.Procedure.Group != nil {
		return nil
	}
	name := body.Procedure.Value
	for _, p := range l.Params {
		if p == name {
			return nil
		}
	}
	builtin, found := l.Closure.Lookup(name)
	if !found {
		return nil
	}
	switch {
	case name == "append" && builtinIs(builtin, appendTarget) && len(body.Arguments) == 2 && plainVariable(body.Arguments[0], acc) &&
		body.Arguments[1].Type != parser.NodePlaceholder && !mentions(body.Arguments[1], acc):
		return &accumulation{lambda: l, part: body.Arguments[1]}
	case name == "merge" && builtinIs(builtin, mergeTarget) && len(body.Arguments) == 1:
		arr := body.Arguments[0]
		if arr.Type != parser.NodeUnary || arr.Value != "[" || len(arr.Expressions) != 2 || arr.ConsArray || arr.KeepArray || arr.Group != nil ||
			len(arr.Stages) != 0 || !plainVariable(arr.Expressions[0], acc) || mentions(arr.Expressions[1], acc) {
			return nil
		}
		return &accumulation{merge: true, lambda: l, part: arr.Expressions[1]}
	}
	return nil
}

// reduceOwned runs the reduction of an accumulation from acc over
// arr[start:]; ok false when a value takes it outside the cases it mirrors
// exactly (the caller then runs the general reduction from the start).
func reduceOwned(a *accumulation, arr, arrAny []any, start int, acc, focus any, env *evaluator.Environment, arity int) (any, bool, error) {
	args := make([]any, arity)
	owned := false
	for i := start; i < len(arr); i++ {
		args[0] = acc
		if arity > 1 {
			args[1] = arr[i]
		}
		if arity > 2 {
			args[2] = float64(i)
		}
		if arity > 3 {
			args[3] = arrAny
		}
		v, err := evaluator.EvalLambdaPart(a.lambda, a.part, args, focus, env)
		if err != nil {
			return nil, false, err
		}
		if a.merge {
			// $merge([$acc, v]): the array constructor keeps an object and
			// skips undefined; anything else is the general path's.
			if (acc != nil && !evaluator.IsMap(acc)) || (v != nil && !evaluator.IsMap(v)) {
				return nil, false, nil
			}
			var result *evaluator.OrderedMap
			if owned {
				result = acc.(*evaluator.OrderedMap)
			} else if first, ok := acc.(*evaluator.OrderedMap); ok {
				result = first.CloneWithCapacity(first.Len() + evaluator.MapLen(v))
			} else {
				result = evaluator.NewOrderedMapWithCapacity(evaluator.MapLen(acc) + evaluator.MapLen(v))
				if acc != nil {
					evaluator.MapRange(acc, func(k string, x any) bool { result.Set(k, x); return true })
				}
			}
			if v != nil {
				evaluator.MapRange(v, func(k string, x any) bool { result.Set(k, x); return true })
			}
			acc, owned = result, true
			continue
		}
		// $append($acc, v), as fnAppend.
		if acc == nil {
			acc, owned = v, false
			continue
		}
		if v == nil {
			continue
		}
		b := wrapArray(v)
		var cur []any
		if owned {
			cur = acc.([]any)
		} else {
			cur = wrapArray(acc)
		}
		if err := env.CheckSequence(len(cur) + len(b)); err != nil {
			return nil, false, err
		}
		const maxAppendSize = 10_000_000
		if len(cur)+len(b) > maxAppendSize {
			_, err := fnAppend([]any{acc, v}, focus, env)
			return nil, false, err
		}
		if !owned {
			grown := make([]any, len(cur), max(len(cur)+len(b), 2*len(cur)))
			copy(grown, cur)
			cur = grown
		}
		acc, owned = append(cur, b...), true
	}
	return acc, true, nil
}
