package gnata_test

import (
	"testing"
)

// The environment of a lambda call is reused once the call ends, unless a
// closure was made in it: closures made in a call (lambdas, partial
// applications, compositions, transforms, $eval) keep reading the
// parameters they captured after many other calls reused the pool.
func TestCallEnvironmentsCapturedByClosuresAreKept(t *testing.T) {
	churn := `$sum($map([1..300], function($i){$i}))`
	for _, test := range []struct{ name, source, want string }{
		{"lambda", `($mk:=function($x){function($y){$x+$y}};$fs:=$map([1,2,3],$mk);` + churn + `;$map($fs,function($f){$f(10)}))`, `[11,12,13]`},
		{"thunk", `($mk:=function($x){function(){$x}};$fs:=$map([1,2,3],$mk);` + churn + `;$map($fs,function($f){$f()}))`, `[1,2,3]`},
		{"partial", `($mk:=function($n){$substring(?,0,$n)};$fs:=$map([1,2,3],$mk);` + churn + `;$map($fs,function($f){$f("abcdef")}))`, `["a","ab","abc"]`},
		{"composition", `($mk:=function($x){($g:=function($v){$v&$x};$g~>$uppercase)};$fs:=$map(["a","b"],$mk);` + churn + `;$map($fs,function($f){$f("z")}))`, `["ZA","ZB"]`},
		{"transform", `($mk:=function($k){|$|{"k":$k}|};$fs:=$map([1,2],$mk);` + churn + `;$map($fs,function($t){$t({"a":0})}))`, `[{"a":0,"k":1},{"a":0,"k":2}]`},
		{"eval", `($mk:=function($x){$eval("function($y){$y*" & $string($x) & "}")};$fs:=$map([2,3],$mk);` + churn + `;$map($fs,function($f){$f(5)}))`, `[10,15]`},
		{"recursion", `($f:=function($n){$n<=1?1:$n*$f($n-1)};$map([1,2,3,4,5],$f))`, `[1,2,6,24,120]`},
		{"nested-calls", `$map([1,2,3],function($x){$map([10,20],function($y){$x*$y})})`, `[[10,20],[20,40],[30,60]]`},
		{"closure-in-block", `($fs:=$map([1,2],function($x){($z:=$x*2;function(){$z})});` + churn + `;$map($fs,function($f){$f()}))`, `[2,4]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			for range 3 {
				got, err := evalOutcome(t, test.source, "")
				if err != "" || got != test.want {
					t.Fatalf("%s = %s %s, want %s", test.source, got, err, test.want)
				}
			}
		})
	}
}
