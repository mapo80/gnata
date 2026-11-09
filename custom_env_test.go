package gnata_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/recolabs/gnata"
)

// A custom environment binds its functions over the shared standard
// library: a custom function shadows the standard one of the same name, the
// other standard functions stay reachable, and the parent operator, lambdas
// and recursion behave as in an environment with the library registered.
func TestCustomEnvironmentOverTheSharedLibrary(t *testing.T) {
	env := gnata.NewCustomEnvironment(map[string]gnata.CustomFunc{
		"now":   func([]any, any) (any, error) { return "fixed", nil },
		"twice": func(a []any, _ any) (any, error) { return a[0].(float64) * 2, nil },
	})
	data := map[string]any{"Account": map[string]any{"Name": "Firefly", "Order": []any{
		map[string]any{"Product": []any{map[string]any{"Name": "Hat", "Price": 34.5}, map[string]any{"Name": "Bag", "Price": 21.25}}},
	}}}
	cases := []struct{ expr, want string }{
		{`$now()`, `fixed`},
		{`$twice(21)`, `42`},
		{`$uppercase(Account.Name)`, `FIREFLY`},
		{`$sum(Account.Order.Product.Price)`, `55.75`},
		{`Account.Order.Product.{"n": Name, "a": %.%.Name}`, `[map[a:Firefly n:Hat] map[a:Firefly n:Bag]]`},
		{`($f := function($n) { $n <= 1 ? 1 : $n * $f($n - 1) }; $f(5))`, `120`},
		{`$map([1, 2], $twice)`, `[2 4]`},
		{`$v + 1`, `8`},
	}
	for _, c := range cases {
		e, err := gnata.Compile(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		got, err := e.EvalWithCustomEnvironmentAndVars(context.Background(), data, env, map[string]any{"v": float64(7)})
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if s := fmt.Sprint(gnata.NormalizeValue(got)); s != c.want {
			t.Fatalf("%s = %s, want %s", c.expr, s, c.want)
		}
	}
	// The standard $now is untouched for evaluations without the custom
	// environment.
	e, _ := gnata.Compile(`$now()`)
	got, err := e.EvalWithVars(context.Background(), nil, nil)
	if err != nil || got == "fixed" {
		t.Fatalf("standard $now = %v, %v", got, err)
	}
}

// Environments built at the same time, each with its own functions, evaluate
// concurrently without seeing each other's functions or call depth.
func TestCustomEnvironmentsEvaluateConcurrently(t *testing.T) {
	e, err := gnata.Compile(`($f := function($n) { $n <= 0 ? $id() : $f($n - 1) }; $f(50))`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 64 {
		wg.Go(func() {
			env := gnata.NewCustomEnvironment(map[string]gnata.CustomFunc{"id": func([]any, any) (any, error) { return float64(i), nil }})
			for range 20 {
				got, err := e.EvalWithCustomEnvironmentAndVars(context.Background(), nil, env, nil)
				if err != nil || got != float64(i) {
					errs <- fmt.Errorf("environment %d: %v %v", i, got, err)
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// Building a custom environment binds the custom functions only.
func TestCustomEnvironmentDoesNotRegisterTheLibraryAgain(t *testing.T) {
	funcs := map[string]gnata.CustomFunc{"a": nil, "b": nil, "c": nil}
	allocs := testing.AllocsPerRun(100, func() { gnata.NewCustomEnvironment(funcs) })
	if allocs > 12 {
		t.Fatalf("%v allocations to build an environment of 3 functions", allocs)
	}
}

func BenchmarkNewCustomEnvironment(b *testing.B) {
	funcs := map[string]gnata.CustomFunc{}
	for i := range 20 {
		funcs[fmt.Sprintf("f%d", i)] = func([]any, any) (any, error) { return nil, nil }
	}
	b.ReportAllocs()
	for b.Loop() {
		gnata.NewCustomEnvironment(funcs)
	}
}
