package datacluster

import (
	"math/big"
	"strings"
)

// decfloat16 and decfloat34 are IEEE 754-2008 decimal64 and decimal128 in
// the densely packed decimal encoding, stored little-endian. The value is
// coefficient × 10^exponent; the coefficient's leading digit lives in the
// combination field and the rest in ten-bit declets of three digits each.

func decodeDecfloat(raw []byte) (string, bool) {
	var bits [16]byte // big-endian working copy
	var expBits, declets, bias int
	switch len(raw) {
	case 8:
		expBits, declets, bias = 8, 5, 398
	case 16:
		expBits, declets, bias = 12, 11, 6176
	default:
		return "", false
	}
	for i := range raw {
		bits[len(raw)-1-i] = raw[i]
	}
	bit := func(i int) int { // i counted from the most significant bit
		return int(bits[i/8]>>(7-i%8)) & 1
	}
	field := func(from, n int) int {
		v := 0
		for i := 0; i < n; i++ {
			v = v<<1 | bit(from+i)
		}
		return v
	}
	sign := bit(0)
	comb := field(1, 5)
	var msd, expHigh int
	switch {
	case comb>>3 == 0b11:
		if comb == 0b11110 {
			return signed(sign, "Inf"), true
		}
		if comb == 0b11111 {
			return "NaN", true
		}
		expHigh = comb >> 1 & 0b11
		msd = 8 + comb&1
	default:
		expHigh = comb >> 3
		msd = comb & 0b111
	}
	exponent := expHigh<<expBits | field(6, expBits)
	exponent -= bias

	var digits strings.Builder
	digits.WriteByte('0' + byte(msd))
	pos := 6 + expBits
	for i := 0; i < declets; i++ {
		d := field(pos, 10)
		pos += 10
		a, b, c := declet(d)
		digits.WriteByte('0' + byte(a))
		digits.WriteByte('0' + byte(b))
		digits.WriteByte('0' + byte(c))
	}
	coeff := digits.String()
	if exponent >= 0 {
		v := new(big.Int)
		v.SetString(coeff+strings.Repeat("0", exponent), 10)
		return signed(sign, v.String()), true
	}
	return signed(sign, placeDecimal(coeff, -exponent)), true
}

func signed(sign int, s string) string {
	if sign == 1 {
		return "-" + s
	}
	return s
}

// declet expands ten densely packed bits into three decimal digits, after
// Cowlishaw's table.
func declet(d int) (int, int, int) {
	p, q, r := d>>9&1, d>>8&1, d>>7&1
	s, t, u := d>>6&1, d>>5&1, d>>4&1
	v, w, x, y := d>>3&1, d>>2&1, d>>1&1, d&1
	three := func(a, b, c int) int { return a<<2 | b<<1 | c }
	if v == 0 {
		return three(p, q, r), three(s, t, u), three(w, x, y)
	}
	switch w<<1 | x {
	case 0b00:
		return three(p, q, r), three(s, t, u), 8 + y
	case 0b01:
		return three(p, q, r), 8 + u, three(s, t, y)
	case 0b10:
		return 8 + r, three(s, t, u), three(p, q, y)
	}
	// w x == 11: the s t bits say which two digits are large.
	switch s<<1 | t {
	case 0b00:
		return 8 + r, 8 + u, three(p, q, y)
	case 0b01:
		return 8 + r, three(p, q, u), 8 + y
	case 0b10:
		return three(p, q, r), 8 + u, 8 + y
	}
	return 8 + r, 8 + u, 8 + y
}
