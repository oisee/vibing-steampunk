// Test: function pointers, callbacks, vtable dispatch
// These patterns are what QuickJS uses heavily

#include <stddef.h>

// === Pattern 1: Simple function pointer call ===
typedef int (*binop_fn)(int, int);

int add(int a, int b) { return a + b; }
int mul(int a, int b) { return a * b; }

int apply(binop_fn fn, int a, int b) {
    return fn(a, b);
}

// === Pattern 2: Struct with function pointer (vtable) ===
typedef struct {
    int (*eval)(int);
    int (*combine)(int, int);
    int id;
} Operations;

int double_it(int x) { return x + x; }
int square_it(int x) { return x * x; }
int sum(int a, int b) { return a + b; }
int product(int a, int b) { return a * b; }

int run_op(Operations* ops, int x) {
    return ops->eval(x);
}

int run_combine(Operations* ops, int a, int b) {
    return ops->combine(a, b);
}

// === Pattern 3: Array of function pointers (dispatch table) ===
typedef int (*unary_fn)(int);

int negate(int x) { return -x; }
int inc(int x) { return x + 1; }
int dec(int x) { return x - 1; }

static unary_fn dispatch_table[] = {negate, inc, dec, double_it, square_it};

int dispatch(int op, int x) {
    if (op < 0 || op > 4) return 0;
    return dispatch_table[op](x);
}

// === Pattern 4: Callback pattern (like QuickJS malloc/free) ===
typedef struct {
    void* (*alloc)(size_t size);
    void (*dealloc)(void* ptr);
} Allocator;

// Simple bump allocator simulation
static char heap[1024];
static int heap_ptr = 0;

void* bump_alloc(size_t size) {
    if (heap_ptr + (int)size > 1024) return 0;
    void* p = &heap[heap_ptr];
    heap_ptr += (int)size;
    return p;
}

void bump_free(void* ptr) {
    // no-op for bump allocator
    (void)ptr;
}

int test_allocator(Allocator* a) {
    void* p = a->alloc(16);
    if (!p) return 0;
    a->dealloc(p);
    return 1;
}

// === Pattern 5: Recursive with function pointer ===
int fold(int* arr, int len, int init, binop_fn fn) {
    int result = init;
    for (int i = 0; i < len; i++) {
        result = fn(result, arr[i]);
    }
    return result;
}
