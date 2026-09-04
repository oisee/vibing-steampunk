package datacluster

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

// ABAP type codes as the kernel writes them into a descriptor. The first ten
// match the classic RFC type numbers; the rest were read off a fixture that
// exported one field of every kind.
const (
	typeChar       = 0x00
	typeDate       = 0x01
	typePacked     = 0x02
	typeTime       = 0x03
	typeRaw        = 0x04
	typeNumc       = 0x06
	typeFloat      = 0x07
	typeInt        = 0x08
	typeInt2       = 0x09
	typeInt1       = 0x0A
	typeStructure  = 0x0E // flat
	typeDeep       = 0x0F // structure containing strings or nested structures
	typeString     = 0x13
	typeXString    = 0x14
	typeDecfloat16 = 0x17
	typeDecfloat34 = 0x18
	typeInt8       = 0x1B
)

var typeNames = map[byte]string{
	typeChar: "CHAR", typeDate: "DATS", typePacked: "DEC", typeTime: "TIMS", typeRaw: "RAW",
	typeNumc: "NUMC", typeFloat: "FLTP", typeInt: "INT4", typeInt2: "INT2", typeInt1: "INT1",
	typeStructure: "STRUCT", typeDeep: "STRUCT", typeString: "STRING", typeXString: "XSTRING",
	typeDecfloat16: "DF16", typeDecfloat34: "DF34", typeInt8: "INT8",
}

// TypeName renders a type code the way DDIC would.
func TypeName(code byte) string {
	if n, ok := typeNames[code]; ok {
		return n
	}
	return fmt.Sprintf("TYPE%02X", code)
}

func isStringType(code byte) bool { return code == typeString || code == typeXString }

type decoder struct {
	utf16     bool
	bigEndian bool
}

func newDecoder(codepage string) (*decoder, error) {
	switch codepage {
	case "4103":
		return &decoder{utf16: true}, nil
	case "4102":
		return &decoder{utf16: true, bigEndian: true}, nil
	}
	if len(codepage) != 4 {
		return nil, fmt.Errorf("datacluster: unreadable code page %q in header", codepage)
	}
	// A single-byte page: bytes are taken as they are, which is right for the
	// ASCII range and a stand-in for the rest.
	return &decoder{}, nil
}

func (d *decoder) text(raw []byte) string {
	if d.utf16 {
		s, err := decodeUTF16(raw, d.bigEndian)
		if err == nil {
			return s
		}
	}
	// A single-byte page, read as Latin-1: right for 1100, and for the
	// others the ASCII range is right and the rest is at least one rune per
	// byte rather than an invalid UTF-8 sequence.
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

func decodeUTF16(raw []byte, bigEndian bool) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("%d bytes is not a whole number of UTF-16 units", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		if bigEndian {
			units[i] = binary.BigEndian.Uint16(raw[2*i:])
		} else {
			units[i] = binary.LittleEndian.Uint16(raw[2*i:])
		}
	}
	return string(utf16.Decode(units)), nil
}

// value decodes one fixed-length field. Character types come back as
// strings with trailing blanks removed, numbers as int64, float64 or a
// decimal string, raw bytes as upper-case hex.
func (d *decoder) value(n *Node, raw []byte) any {
	switch n.TypeCode {
	case typeChar:
		return strings.TrimRight(d.text(raw), " ")
	case typeNumc, typeDate, typeTime:
		return d.text(raw)
	case typeRaw:
		return strings.ToUpper(hex.EncodeToString(raw))
	case typeInt:
		if len(raw) == 4 {
			return int64(int32(binary.LittleEndian.Uint32(raw)))
		}
	case typeInt2:
		if len(raw) == 2 {
			return int64(int16(binary.LittleEndian.Uint16(raw)))
		}
	case typeInt1:
		if len(raw) == 1 {
			return int64(raw[0])
		}
	case typeInt8:
		if len(raw) == 8 {
			return int64(binary.LittleEndian.Uint64(raw))
		}
	case typeFloat:
		if len(raw) == 8 {
			return math.Float64frombits(binary.LittleEndian.Uint64(raw))
		}
	case typePacked:
		return decodePacked(raw, n.Decimals)
	case typeDecfloat16, typeDecfloat34:
		if s, ok := decodeDecfloat(raw); ok {
			return s
		}
	}
	return strings.ToUpper(hex.EncodeToString(raw))
}

func (d *decoder) stringValue(n *Node, raw []byte) any {
	if n.TypeCode == typeXString {
		return strings.ToUpper(hex.EncodeToString(raw))
	}
	return d.text(raw)
}

// decodePacked renders a BCD number: two digits per byte, the last nibble the
// sign, C or F for positive and D for negative.
func decodePacked(raw []byte, decimals int) string {
	if len(raw) == 0 {
		return ""
	}
	var digits strings.Builder
	for i, b := range raw {
		digits.WriteByte('0' + b>>4)
		if i < len(raw)-1 {
			digits.WriteByte('0' + b&0x0F)
		}
	}
	sign := ""
	if raw[len(raw)-1]&0x0F == 0x0D {
		sign = "-"
	}
	return sign + placeDecimal(digits.String(), decimals)
}

// placeDecimal turns a digit string into a number with the decimal point
// placed from the right, leading zeros gone.
func placeDecimal(digits string, decimals int) string {
	if decimals > 0 {
		for len(digits) <= decimals {
			digits = "0" + digits
		}
		digits = digits[:len(digits)-decimals] + "." + digits[len(digits)-decimals:]
	}
	trimmed := strings.TrimLeft(digits, "0")
	if trimmed == "" || trimmed[0] == '.' {
		trimmed = "0" + trimmed
	}
	return trimmed
}

// UTF16Text decodes a whole buffer as UTF-16LE, which is how a Unicode
// system stores text it compresses on its own, source code included.
func UTF16Text(raw []byte) (string, error) {
	return decodeUTF16(raw, false)
}
