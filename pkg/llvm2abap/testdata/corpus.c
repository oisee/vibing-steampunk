// === TIER 1: Leaf functions (no control flow) ===

int add(int a, int b) { return a + b; }
int sub(int a, int b) { return a - b; }
int mul(int a, int b) { return a * b; }
int div_s(int a, int b) { return a / b; }
int rem_s(int a, int b) { return a % b; }
int negate(int x) { return -x; }
int identity(int x) { return x; }

// Bitwise
int and_(int a, int b) { return a & b; }
int or_(int a, int b) { return a | b; }
int xor_(int a, int b) { return a ^ b; }
int shl(int a, int b) { return a << b; }
int shr(int a, int b) { return a >> b; }

// Multi-expression
int quadratic(int a, int b, int c, int x) {
    return a * x * x + b * x + c;
}

// Float
double fadd(double a, double b) { return a + b; }
double fmul(double a, double b) { return a * b; }

// i64
long long add64(long long a, long long b) { return a + b; }

// === TIER 2: Control flow (if/else) ===

int abs_val(int x) { return x < 0 ? -x : x; }
int max(int a, int b) { return a > b ? a : b; }
int min(int a, int b) { return a < b ? a : b; }
int clamp(int x, int lo, int hi) {
    if (x < lo) return lo;
    if (x > hi) return hi;
    return x;
}
int sign(int x) {
    if (x > 0) return 1;
    if (x < 0) return -1;
    return 0;
}

// === TIER 3: Loops ===

int sum_to(int n) {
    int s = 0;
    for (int i = 1; i <= n; i++) s += i;
    return s;
}

int factorial(int n) {
    int r = 1;
    for (int i = 2; i <= n; i++) r *= i;
    return r;
}

int fibonacci(int n) {
    if (n <= 1) return n;
    int a = 0, b = 1;
    for (int i = 2; i <= n; i++) {
        int t = a + b;
        a = b;
        b = t;
    }
    return b;
}

int gcd(int a, int b) {
    while (b != 0) {
        int t = b;
        b = a % b;
        a = t;
    }
    return a;
}

int is_prime(int n) {
    if (n < 2) return 0;
    for (int i = 2; i * i <= n; i++)
        if (n % i == 0) return 0;
    return 1;
}

// === TIER 4: Function calls (non-leaf) ===

int double_val(int x) { return add(x, x); }
int square(int x) { return mul(x, x); }
int cube(int x) { return mul(mul(x, x), x); }

int factorial_rec(int n) {
    if (n <= 1) return 1;
    return n * factorial_rec(n - 1);
}

int fib_rec(int n) {
    if (n <= 1) return n;
    return fib_rec(n - 1) + fib_rec(n - 2);
}

// === TIER 5: Structs & pointers ===

typedef struct { int x; int y; } Point;

int point_sum(Point* p) { return p->x + p->y; }
void point_set(Point* p, int x, int y) { p->x = x; p->y = y; }

int array_sum(int* arr, int len) {
    int s = 0;
    for (int i = 0; i < len; i++) s += arr[i];
    return s;
}
