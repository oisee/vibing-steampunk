// Mini VM: simulates QuickJS patterns (function pointers, malloc, eval)
// Small enough for transpiler, tests all critical patterns

typedef int (*opfn)(int, int);
typedef void* (*alloc_fn)(int);
typedef void (*free_fn)(void*);

// Memory model (like QuickJS)
typedef struct {
    alloc_fn malloc_func;
    free_fn free_func;
} Runtime;

// Value type (like JSValue)
typedef struct {
    int tag;   // 0=int, 1=float, 2=string_offset
    int val;
} Value;

// --- Basic ops ---
int op_add(int a, int b) { return a + b; }
int op_sub(int a, int b) { return a - b; }
int op_mul(int a, int b) { return a * b; }
int op_div(int a, int b) { return b != 0 ? a / b : 0; }
int op_mod(int a, int b) { return b != 0 ? a % b : 0; }
int op_neg(int a, int b) { (void)b; return -a; }
int op_eq(int a, int b) { return a == b ? 1 : 0; }
int op_lt(int a, int b) { return a < b ? 1 : 0; }

// --- Dispatch table (like QuickJS opcode dispatch) ---
static opfn op_table[] = {
    op_add, op_sub, op_mul, op_div,
    op_mod, op_neg, op_eq,  op_lt
};

int exec_op(int opcode, int a, int b) {
    if (opcode < 0 || opcode > 7) return 0;
    return op_table[opcode](a, b);
}

// --- Stack machine (like QuickJS bytecode interpreter) ---
static int stack[256];
static int sp = 0;

void push(int v) { if (sp < 256) stack[sp++] = v; }
int pop(void) { return sp > 0 ? stack[--sp] : 0; }
int peek(void) { return sp > 0 ? stack[sp-1] : 0; }

// Execute a sequence of bytecodes
// Format: [opcode, ...args]
// Opcodes: 0=push_const(val), 1=binop(op), 2=halt
int eval_bytecode(int* code, int len) {
    int pc = 0;
    while (pc < len) {
        int op = code[pc++];
        switch (op) {
            case 0: // push_const
                if (pc < len) push(code[pc++]);
                break;
            case 1: { // binop
                if (pc < len) {
                    int binop = code[pc++];
                    int b = pop();
                    int a = pop();
                    push(exec_op(binop, a, b));
                }
                break;
            }
            case 2: // halt
                return pop();
            default:
                return -1;
        }
    }
    return pop();
}

// --- Value operations (like JSValue) ---
Value make_int(int v) {
    Value r;
    r.tag = 0;
    r.val = v;
    return r;
}

int value_to_int(Value v) {
    return v.tag == 0 ? v.val : 0;
}

Value value_add(Value a, Value b) {
    return make_int(value_to_int(a) + value_to_int(b));
}

Value value_mul(Value a, Value b) {
    return make_int(value_to_int(a) * value_to_int(b));
}

// --- Test harness ---
int test_direct(void) {
    return op_add(3, 4); // = 7
}

int test_dispatch(void) {
    // exec_op(0=add, 10, 20) = 30
    // exec_op(2=mul, 6, 7) = 42
    return exec_op(0, 10, 20) + exec_op(2, 6, 7); // = 72
}

int test_stack_vm(void) {
    // Evaluate: (3 + 4) * 5 = 35
    // Bytecode: push 3, push 4, add, push 5, mul, halt
    int code[] = {0,3, 0,4, 1,0, 0,5, 1,2, 2};
    sp = 0;
    return eval_bytecode(code, 11);
}

int test_all(void) {
    int ok = 0;
    if (test_direct() == 7) ok++;
    if (test_dispatch() == 72) ok++;
    if (test_stack_vm() == 35) ok++;
    if (op_eq(42, 42) == 1) ok++;
    if (op_lt(3, 7) == 1) ok++;
    if (exec_op(5, 42, 0) == -42) ok++;  // neg
    return ok; // should be 6
}
