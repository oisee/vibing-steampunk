package jseval
import ("strings";"testing")
func TestTranspiledABAP(t *testing.T) {
	code := `
function makeInt() {
  let obj = {value: 0};
  obj.set = function(v) {
    if (typeof v === "number") { obj.value = v; }
    else if (v !== undefined && v !== null && v.get) { obj.value = v.get(); }
  };
  obj.get = function() { return obj.value; };
  return obj;
}
var abap = {};
abap.types = { Integer: function(opts) { return makeInt(); } };
abap.IntegerFactory = { get: function(n) { let i = makeInt(); i.value = n; return i; } };
abap.compare = { le: function(a, b) { return a.get() <= b.get(); } };
abap.operators = { add: function(a, b) { let r = makeInt(); r.value = a.get() + b.get(); return r; } };
let syIdx = makeInt();
abap.builtin = { sy: { get: function() { return {index: syIdx}; } } };
let outBuf = "";
abap.statements = { write: function(s) { outBuf = outBuf + s.get(); } };

let lv_sum = new abap.types.Integer({});
let lv_i = new abap.types.Integer({});
lv_sum.set(abap.IntegerFactory.get(0));
lv_i.set(abap.IntegerFactory.get(1));
let unique1 = 1;
while (abap.compare.le(lv_i, abap.IntegerFactory.get(10))) {
  abap.builtin.sy.get().index.set(unique1);
  unique1 = unique1 + 1;
  lv_sum.set(abap.operators.add(lv_sum, lv_i));
  lv_i.set(abap.operators.add(lv_i, abap.IntegerFactory.get(1)));
}
abap.statements.write(lv_sum);
console.log(outBuf);
`
	out, err := Eval(code)
	if err != nil { t.Fatalf("error: %v", err) }
	got := strings.TrimSpace(out)
	t.Logf("ABAP WHILE sum(1..10) = %s", got)
	if got != "55" { t.Errorf("got %q, want 55", got) }
}
