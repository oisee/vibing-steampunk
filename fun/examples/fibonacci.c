// Example C functions for LLVM → ABAP compilation
// Compile: clang -S -emit-llvm -O1 fibonacci.c -o fibonacci.ll

int add(int a, int b) { return a + b; }

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

double lerp(double a, double b, double t) {
    return a + (b - a) * t;
}
