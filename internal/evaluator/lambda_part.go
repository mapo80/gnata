package evaluator

import "github.com/recolabs/gnata/internal/parser"

// EvalLambdaPart evaluates part, a node of l's body, as callFunction
// evaluates l's body when l is called with args: the same call depth and
// overflow check, a child of l's closure with the parameters bound (missing
// arguments bound to undefined), the same focus. It lets a higher-order
// builtin evaluate one argument of the call l's body makes without making
// that call (reduceOwned).
func EvalLambdaPart(l *Lambda, part *parser.Node, args []any, focus any, env *Environment) (any, error) {
	if err := env.Context().Err(); err != nil {
		return nil, err
	}
	counter := env.callCounter()
	counter.depth++
	defer func() { counter.depth-- }()
	if counter.depth > counter.max {
		return nil, stackOverflowError(counter)
	}
	child := NewChildEnvironment(l.Closure)
	child.calls = counter
	for i, param := range l.Params {
		if i < len(args) {
			child.Bind(param, args[i])
		} else {
			child.Bind(param, nil)
		}
	}
	return Eval(part, focus, child)
}
