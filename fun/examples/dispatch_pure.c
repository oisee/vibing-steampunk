// Pure function pointer dispatch — no memory access needed
typedef int (*binop_fn)(int, int);

int add(int a, int b) { return a + b; }
int mul(int a, int b) { return a * b; }
int sub(int a, int b) { return a - b; }

int apply(binop_fn fn, int a, int b) {
    return fn(a, b);
}

int double_it(int x) { return x + x; }
int square_it(int x) { return x * x; }
int negate(int x) { return -x; }

typedef int (*unary_fn)(int);

int apply1(unary_fn fn, int x) {
    return fn(x);
}

// Chain: apply two operations
int chain(unary_fn f, unary_fn g, int x) {
    return g(f(x));
}

// Fold array with binop
int fold3(binop_fn fn, int a, int b, int c) {
    return fn(fn(a, b), c);
}
