package gnata

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateBytesGolden rewrites testdata/bytes_golden.json from the running
// implementation. The file in the repository was written by the gjson
// implementation (commit 7fa870c), before the byte paths moved to
// encoding/json/jsontext.
var updateBytesGolden = flag.Bool("update-bytes-golden", false, "rewrite testdata/bytes_golden.json")

var bytesDocuments = []string{
	`{"a":{"b":1,"c":"x","d":[1,2]},"arr":[{"p":1},{"p":2},{"p":[3,4]},{"q":5}],"n":null,"t":true,"f":false,"s":"  Hello   World ","num":12345678901234567890,"flt":1.5e3,"neg":-0.0,"list":["x","y","x"],"nums":[3,1.5,-2],"empty":[],"obj":{}}`,
	`{"a":1,"a":2,"o":{"k":"first","k":"second"},"arr":[{"p":1,"p":9}]}`,
	"{\"s\":\"bad\xff\xfe\",\"e\":\"\\u00e8\\n\\t\\\"\",\"k\xff\":1,\"mixed\":\"\\u0041\xff\"}",
	`{"\u0061":1,"a.b":2,"we ird":3,"0":"zero","list":["x","y"],"t\u00e9":4,"esc\"q":5}`,
	`{"big":123456789012345678901234567890,"exp":1E+2,"tiny":5e-324,"huge":1e400,"negint":-42,"zero":0,"frac":0.1}`,
	`{"lone":"\ud800","pair":"\ud83d\ude00","nul":"a\u0000b"}`,
	`[{"a":1},{"a":2},{"b":3}]`,
	`["x",1,null]`,
	`5`,
	`"top"`,
	`null`,
	`{"a":1} trailing`,
	`{"a":1}{"a":2}`,
	`{"a":1,`,
	`{"a":}`,
	`{"a":1,"b":[1,2}`,
	`{'a':1}`,
	``,
	`   `,
	` {"a" : { "b" : 7 } } `,
}

var bytesExpressions = []string{
	`a`, `a.b`, `a.c`, `a.d`, `a.zz`, `arr.p`, `arr.q`, `arr`, `n`, `t`, `f`, `s`, `num`, `flt`, `neg`, `o.k`, `o`,
	"`a.b`", "`we ird`", "`0`", "`t\u00e9`", "`esc\"q`", `list`, `e`, `mixed`, `lone`, `pair`, `nul`, `k`,
	`big`, `exp`, `tiny`, `huge`, `negint`, `zero`, `frac`, `b`,
	`a.b = 1`, `a.b != 1`, `a = 2`, `a = 1`, `s = "  Hello   World "`, `num = 12345678901234567890`, `big = 123456789012345678901234567890`,
	`flt = 1500`, `frac = 0.1`, `n = null`, `t = true`, `f = false`, `arr.p = 1`, `arr.p != 1`, `o.k = "first"`, `o.k = "second"`,
	`e = "è\n\t\""`, `a = "x"`, `empty = 1`, `zz = 1`, `zz != 1`,
	`$exists(a.b)`, `$exists(zz)`, `$exists(arr.p)`, `$exists(arr.zz)`, `$exists(n)`,
	`$string(num)`, `$string(flt)`, `$string(t)`, `$string(n)`, `$string(a)`, `$string(s)`,
	`$number(s)`, `$number(num)`, `$number(t)`, `$keys(a)`, `$keys(o)`, `$keys(obj)`, `$count(arr)`, `$count(list)`, `$count(arr.p)`,
	`$sum(nums)`, `$sum(arr.p)`, `$max(nums)`, `$min(nums)`, `$max(arr.p)`, `$average(nums)`, `$average(arr.p)`,
	`$contains(s, "World")`, `$contains(list, "y")`, `$contains(arr.p, "1")`, `$contains(e, "è")`,
	`$lowercase(s)`, `$uppercase(s)`, `$trim(s)`, `$length(s)`, `$length(e)`, `$length(mixed)`,
	`$type(a)`, `$type(list)`, `$type(num)`, `$type(n)`, `$type(t)`, `$type(s)`, `$type(zz)`,
	`$boolean(s)`, `$boolean(zero)`, `$boolean(empty)`, `$not(f)`, `$not(s)`, `$not(obj)`,
	`$distinct(list)`, `$distinct(nums)`, `$reverse(list)`, `$abs(neg)`, `$abs(negint)`, `$floor(flt)`, `$ceil(frac)`, `$sqrt(flt)`,
}

// bytesOutcome is what an evaluation gives, in a form that compares across
// implementations: the Go type, the value as encoding/json writes it, the error.
func bytesOutcome(v any, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		return fmt.Sprintf("%T unmarshalable: %v", v, merr)
	}
	return fmt.Sprintf("%T %s", v, b)
}

func bytesOutcomes(t *testing.T) map[string]string {
	t.Helper()
	ctx := context.Background()
	out := map[string]string{}
	for di, doc := range bytesDocuments {
		var top map[string]json.RawMessage
		mapOK := json.Unmarshal([]byte(doc), &top) == nil
		for _, expr := range bytesExpressions {
			e, err := Compile(expr)
			if err != nil {
				t.Fatalf("Compile(%q): %v", expr, err)
			}
			key := fmt.Sprintf("doc%02d %s", di, expr)
			out[key+" | EvalBytes"] = bytesOutcome(e.EvalBytes(ctx, json.RawMessage(doc)))
			out[key+" | EvalBytesWithVars"] = bytesOutcome(e.EvalBytesWithVars(ctx, json.RawMessage(doc), nil))
			out[key+" | EvalBytesWithCustomEnvironmentAndVars"] = bytesOutcome(e.EvalBytesWithCustomEnvironmentAndVars(ctx, json.RawMessage(doc), nil, nil))
			if mapOK {
				out[key+" | EvalMap"] = bytesOutcome(e.EvalMap(ctx, top))
			}
		}
	}
	return out
}

func readBytesGolden(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "bytes_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if err := json.Unmarshal(b, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

// malformedDocuments are the documents (by index) whose outcomes changed:
// in {"a":1,"b":[1,2}, gjson returned the malformed array of b, which then
// decoded to nothing (null); jsontext refuses the array, so the evaluation
// falls back to the full decoder, which reports the syntax error, as Eval of
// the decoded document would. On the other malformed documents the lookups
// read only the valid part before the error, as gjson did.
var malformedDocuments = map[int]bool{15: true}

// TestByteEvaluationMatchesGjsonGolden evaluates every expression against
// every document through the byte entry points and compares with what the
// gjson implementation gave (testdata/bytes_golden.json): equal outcomes,
// except on malformedDocuments, where the outcome is the decoder's error
// and at least one outcome changed (else the list is stale).
func TestByteEvaluationMatchesGjsonGolden(t *testing.T) {
	got := bytesOutcomes(t)
	if *updateBytesGolden {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "bytes_golden.json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := readBytesGolden(t)
	if len(got) != len(want) {
		t.Fatalf("%d outcomes, golden has %d", len(got), len(want))
	}
	changed := map[int]int{}
	for key, w := range want {
		g := got[key]
		if g == w {
			continue
		}
		var doc int
		fmt.Sscanf(key, "doc%02d", &doc)
		if !malformedDocuments[doc] {
			t.Errorf("%s:\n got %s\nwant %s", key, g, w)
			continue
		}
		_, decodeErr := DecodeJSON(json.RawMessage(bytesDocuments[doc]))
		if decodeErr == nil || g != bytesOutcome(nil, decodeErr) {
			t.Errorf("%s on a malformed document: got %s, want the decoder's error %v", key, g, decodeErr)
		}
		changed[doc]++
	}
	for doc := range malformedDocuments {
		if changed[doc] == 0 {
			t.Errorf("doc%02d: no outcome changed, remove it from malformedDocuments", doc)
		}
	}
	t.Logf("%d outcomes, changed on malformed documents: %v", len(got), changed)
}

// TestByteEvaluationAgreesWithEval: on valid documents without duplicate
// names, the byte entry points give what Eval gives on the decoded document,
// or else what they gave with gjson (testdata/bytes_golden.json): the fast
// paths read numbers that are not canonical integers as float64, compare
// big integers as float64 and index arrays with a numeric step, where Eval
// keeps json.Number and maps the step over the elements.
func TestByteEvaluationAgreesWithEval(t *testing.T) {
	ctx := context.Background()
	golden := readBytesGolden(t)
	disagreements := 0
	for di, doc := range bytesDocuments {
		if strings.Contains(doc, `"a":1,"a":2`) || !json.Valid([]byte(doc)) {
			continue
		}
		decoded, err := DecodeJSON(json.RawMessage(doc))
		if err != nil {
			t.Fatal(err)
		}
		for _, expr := range bytesExpressions {
			e, err := Compile(expr)
			if err != nil {
				t.Fatal(err)
			}
			want := bytesOutcome(e.Eval(ctx, decoded))
			got := bytesOutcome(e.EvalBytes(ctx, json.RawMessage(doc)))
			if got == want {
				continue
			}
			disagreements++
			if got != golden[fmt.Sprintf("doc%02d %s | EvalBytes", di, expr)] {
				t.Errorf("doc%02d %s: EvalBytes %s, Eval %s, and gjson gave something else", di, expr, got, want)
			}
		}
	}
	t.Logf("%d disagreements with Eval, all as with gjson", disagreements)
}
