package gnata_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/recolabs/gnata"
)

// reduceCase is an accumulating reduction and the same reduction written
// so that the accumulator is read through a block, which the owned
// accumulation does not recognize: the general $reduce, the oracle.
type reduceCase struct{ name, fast, input string }

func general(source string) string {
	for _, acc := range []string{"$xs", "$a", "$acc"} {
		source = strings.ReplaceAll(source, "$append("+acc+",", "$append(("+acc+"),")
		source = strings.ReplaceAll(source, "$merge(["+acc+",", "$merge([("+acc+"),")
	}
	return source
}

func evalOutcome(t *testing.T, source, input string) (string, string) {
	t.Helper()
	expression, err := gnata.Compile(source)
	if err != nil {
		t.Fatalf("%s: %v", source, err)
	}
	var data any
	if input != "" {
		if err := json.Unmarshal([]byte(input), &data); err != nil {
			t.Fatal(err)
		}
	}
	result, err := expression.Eval(context.Background(), data)
	if err != nil {
		code := err.Error()
		var je interface{ Error() string }
		_ = je
		return "", code
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded), ""
}

func TestReduceOwnedAccumulationMatchesTheGeneralReduce(t *testing.T) {
	steps := `{"steps":[{"id":"a","next":"b"},{"id":"b","branches":[{"next":"c"},{"next":"d"}],"type":"BRANCH"},{"id":"c","tool":{"errorPolicy":{"next":"e"}}},{"id":"d","next":""},{"id":"e"}]}`
	cases := []reduceCase{
		{"step-list", `$reduce(steps,function($xs,$s){$append($xs,[$filter($append($append([$s.next],[$s.branches.next]),[$s.tool.errorPolicy.next]),function($v){$type($v)="string" and $v!=""})])},[])`, steps},
		{"flat-append", `$reduce(steps,function($xs,$s){$append($xs,$s.id)},[])`, steps},
		{"append-arrays", `$reduce(steps,function($xs,$s){$append($xs,$s.branches.next)},[])`, steps},
		{"append-no-init", `$reduce(steps.id,function($xs,$s){$append($xs,$s)})`, steps},
		{"append-undefined-init", `$reduce(steps,function($xs,$s){$append($xs,$s.missing)},missing)`, steps},
		{"append-from-undefined", `$reduce(steps,function($xs,$s){$append($xs,$s.next)},missing)`, steps},
		{"append-index-and-array", `$reduce(steps,function($xs,$s,$i,$all){$append($xs,{"i":$i,"n":$count($all),"id":$s.id})},[])`, steps},
		{"append-sequence", `$reduce(steps,function($xs,$s){$append($xs,steps[id=$s.id].id)},[])`, steps},
		{"append-empty", `$reduce(steps,function($xs,$s){$append($xs,[])},[])`, steps},
		{"append-null", `$reduce([1,2,3],function($xs,$s){$append($xs,null)},[])`, ``},
		{"initial-not-mutated", `($init:=[0];$r:=$reduce([1,2,3],function($xs,$x){$append($xs,$x)},$init);[$init,$r,$reduce([4],function($xs,$x){$append($xs,$x)},$init)])`, ``},
		{"merge-set", `$reduce(steps.id,function($a,$s){$merge([$a,{$s:true}])},{})`, steps},
		{"merge-overwrite", `$reduce([{"k":1},{"j":2},{"k":3}],function($a,$s){$merge([$a,$s])},{"z":0})`, ``},
		{"merge-no-init", `$reduce([{"k":1},{"j":2}],function($a,$s){$merge([$a,$s])})`, ``},
		{"merge-undefined", `$reduce([{"k":1},{"j":2}],function($a,$s){$merge([$a,$s.missing])},{})`, ``},
		{"merge-data-object", `$reduce(steps,function($a,$s){$merge([$a,$s])},{})`, steps},
		{"merge-initial-not-mutated", `($init:={"z":0};$r:=$reduce(["a","b"],function($a,$s){$merge([$a,{$s:1}])},$init);[$init,$r])`, ``},
		{"merge-array-element", `$reduce([[{"k":1}],[{"j":2}]],function($a,$s){$merge([$a,$s])},{})`, ``},
		{"merge-not-object", `$reduce([1,2],function($a,$s){$merge([$a,$s])},{})`, ``},
		{"append-shadow", `$reduce([1,2],function($xs,$x){($xs:=[9];$append($xs,$x))},[])`, ``},
		{"append-reads-acc", `$reduce([1,2,3],function($xs,$x){$append($xs,$count($xs))},[])`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want, wantErr := evalOutcome(t, general(c.fast), c.input)
			got, gotErr := evalOutcome(t, c.fast, c.input)
			if got != want || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Fatalf("%s\n got %s %s\nwant %s %s", c.fast, got, gotErr, want, wantErr)
			}
		})
	}
}

// Building a list of n members costs n copies, not n²/2.
func BenchmarkReduceAppend(b *testing.B) {
	items := make([]any, 500)
	for i := range items {
		items[i] = map[string]any{"id": fmt.Sprintf("s%03d", i), "next": fmt.Sprintf("s%03d", i+1)}
	}
	for _, source := range []string{`$reduce(steps,function($xs,$s){$append($xs,[$s.next])},[])`, general(`$reduce(steps,function($xs,$s){$append($xs,[$s.next])},[])`)} {
		name := "owned"
		if strings.Contains(source, "($xs)") {
			name = "general"
		}
		b.Run(name, func(b *testing.B) {
			expression, err := gnata.Compile(source)
			if err != nil {
				b.Fatal(err)
			}
			data := map[string]any{"steps": items}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := expression.Eval(context.Background(), data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
