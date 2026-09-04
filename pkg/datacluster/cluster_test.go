package datacluster

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The INDX fixtures were written on a 7.58 system by a program that exported
// one structure with every elementary type, a two-row table of it, a nested
// structure, a bare CHAR and a bare INT4 — once with the default compression
// and once with COMPRESSION OFF. The values are known, so the test is exact.
//
// The BALDAT fixture is one application log from the same system, with the
// message tables the BAL layer stores, three of them empty, one holding three
// messages.

func loadHex(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// structValues is the row every STRUCT-typed object in the fixture holds.
var structValues = []any{
	"ABC", "000042", "20260904", "123456",
	int64(-7), int64(200), int64(-300), int64(1234567890123),
	"-12345.67", 2.5, "DEADBEEF", "a string value", "CAFE", "3.14159", "20260904123456.1234567",
}

var structTypes = []string{
	"CHAR", "NUMC", "DATS", "TIMS", "INT4", "INT1", "INT2", "INT8",
	"DEC", "FLTP", "RAW", "STRING", "XSTRING", "DF34", "DEC",
}

func TestParseINDXFixtures(t *testing.T) {
	for _, tc := range []struct {
		file       string
		compressed bool
		objects    int
	}{
		{"indx_plain.hex", false, 2},
		{"indx_compressed.hex", true, 5},
	} {
		t.Run(tc.file, func(t *testing.T) {
			c, err := Parse(loadHex(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if c.Compressed != tc.compressed || c.Codepage != "4103" || c.Version != 6 {
				t.Errorf("header: %+v", *c)
			}
			if tc.compressed && c.Algorithm != "LZH" {
				t.Errorf("algorithm %q", c.Algorithm)
			}
			if len(c.Objects) != tc.objects {
				t.Fatalf("%d objects, want %d", len(c.Objects), tc.objects)
			}

			st := c.Object("STRUCT")
			if st == nil || st.Kind != Structure || st.RowLength != 160 || len(st.Rows) != 1 {
				t.Fatalf("STRUCT: %+v", st)
			}
			if !tc.compressed && st.Size != 389 {
				t.Errorf("STRUCT size %d, want 389 in the plain body", st.Size)
			}
			checkFields(t, st, structTypes)
			if !reflect.DeepEqual(st.Rows[0], structValues) {
				t.Errorf("STRUCT row\n got %#v\nwant %#v", st.Rows[0], structValues)
			}
			if st.Fields[8].Decimals != 2 || st.Fields[14].Decimals != 7 {
				t.Errorf("decimals: %+v %+v", st.Fields[8], st.Fields[14])
			}

			tab := c.Object("TABLE")
			if tab == nil || tab.Kind != Table || len(tab.Rows) != 2 {
				t.Fatalf("TABLE: %+v", tab)
			}
			checkFields(t, tab, structTypes)
			if !reflect.DeepEqual(tab.Rows[0], structValues) {
				t.Errorf("TABLE row 1: %#v", tab.Rows[0])
			}
			row2 := append([]any(nil), structValues...)
			row2[0], row2[4] = "row2", int64(2)
			if !reflect.DeepEqual(tab.Rows[1], row2) {
				t.Errorf("TABLE row 2: %#v", tab.Rows[1])
			}

			if !tc.compressed {
				return
			}
			nested := c.Object("NESTED")
			if nested == nil || nested.RowLength != 192 || len(nested.Rows) != 1 {
				t.Fatalf("NESTED: %+v", nested)
			}
			wantPaths := []string{"1"}
			for i := 1; i <= 15; i++ {
				wantPaths = append(wantPaths, fmt.Sprintf("2.%d", i))
			}
			wantPaths = append(wantPaths, "3")
			var gotPaths []string
			for _, f := range nested.Fields {
				gotPaths = append(gotPaths, f.Path)
			}
			if !reflect.DeepEqual(gotPaths, wantPaths) {
				t.Errorf("NESTED paths %v", gotPaths)
			}
			want := append(append([]any{"HEAD"}, structValues...), "99")
			if !reflect.DeepEqual(nested.Rows[0], want) {
				t.Errorf("NESTED row %#v", nested.Rows[0])
			}
			// HEAD, an alignment filler, the inner structure (15 fields and 4
			// fillers), the tail, a trailing filler.
			if len(nested.Type.Children) != 5 || !nested.Type.Children[1].Filler || len(nested.Type.Children[2].Children) != 19 {
				t.Errorf("NESTED tree: %d children, inner %d", len(nested.Type.Children), len(nested.Type.Children[2].Children))
			}

			scalar := c.Object("SCALAR")
			if scalar == nil || scalar.Kind != Elementary || scalar.Fields[0].Type != "CHAR" || scalar.Rows[0][0] != "a bare elementary field" {
				t.Errorf("SCALAR: %+v", scalar)
			}
			number := c.Object("NUMBER")
			if number == nil || number.Kind != Elementary || number.Fields[0].Type != "INT4" || number.Rows[0][0] != int64(4711) {
				t.Errorf("NUMBER: %+v", number)
			}
		})
	}
}

func checkFields(t *testing.T, obj *Object, types []string) {
	t.Helper()
	if len(obj.Fields) != len(types) {
		t.Fatalf("%s: %d fields, want %d: %+v", obj.Name, len(obj.Fields), len(types), obj.Fields)
	}
	for i, f := range obj.Fields {
		if f.Type != types[i] {
			t.Errorf("%s field %d: %s, want %s", obj.Name, i+1, f.Type, types[i])
		}
	}
}

func TestParseBALDAT(t *testing.T) {
	c, err := Parse(loadHex(t, "baldat_a4h.hex"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range c.Objects {
		if o.Kind != Table {
			t.Errorf("%s is a %s", o.Name, o.Kind)
		}
		if len(o.Rows) > 0 {
			names = append(names, fmt.Sprintf("%s:%d", o.Name, len(o.Rows)))
		}
	}
	if got := strings.Join(names, " "); got != "T_2000:3 T_MHDR:3" {
		t.Errorf("tables with rows: %s", got)
	}
	msgs := c.Object("T_2000")
	// The BAL message row: number, four 50-character variables, two flags,
	// four flags, a one-plus-three include, then type, class, number, detail
	// level, problem class, sort, timestamp, count.
	if msgs.RowLength != 12+400+4+8+8+76 || len(msgs.Fields) != 5+2+4+4+8 {
		t.Fatalf("T_2000: row %d, %d fields", msgs.RowLength, len(msgs.Fields))
	}
	if empty := c.Object("T_1000"); empty == nil || len(empty.Rows) != 0 || empty.RowLength != 12+160+4+8+8+76 {
		t.Errorf("T_1000: %+v", empty)
	}
	row := msgs.Rows[0]
	if row[0] != "000001" || row[1] != "Periodic mode - checking all active runs" {
		t.Errorf("first message: %#v", row[:2])
	}
	tail := row[len(row)-8:]
	if tail[0] != "I" || tail[1] != "BL" || tail[2] != "001" || tail[7] != int64(1) {
		t.Errorf("message tail: %#v", tail)
	}
	if ts, _ := tail[6].(string); !strings.HasPrefix(ts, "2026") || !strings.Contains(ts, ".") {
		t.Errorf("timestamp: %#v", tail[6])
	}
	dir := c.Object("T_MHDR")
	if dir.Rows[2][0] != "000003" || dir.Rows[2][1] != "2" {
		t.Errorf("directory: %#v", dir.Rows[2])
	}
}

func TestRejects(t *testing.T) {
	blob := loadHex(t, "indx_plain.hex")
	for name, bad := range map[string][]byte{
		"short":     blob[:10],
		"no marker": append([]byte{0}, blob[1:]...),
		"format":    append(append([]byte{}, blob[:4]...), append([]byte{9}, blob[5:]...)...),
		"truncated": blob[:len(blob)/2],
		"no end":    blob[:len(blob)-1],
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	comp := loadHex(t, "indx_compressed.hex")
	damaged := append([]byte(nil), comp...)
	damaged[100] ^= 0xFF
	if _, err := Parse(damaged); err == nil {
		t.Error("damaged compressed body accepted")
	}
}

func TestJoin(t *testing.T) {
	whole := loadHex(t, "indx_plain.hex")
	var frags []Fragment
	for i := 0; i*512 < len(whole); i++ {
		end := (i + 1) * 512
		if end > len(whole) {
			end = len(whole)
		}
		padded := make([]byte, 512)
		copy(padded, whole[i*512:end])
		frags = append(frags, Fragment{Seq: i, Length: end - i*512, Data: padded})
	}
	// Out of order on purpose.
	frags[0], frags[1] = frags[1], frags[0]
	got, err := Join(frags)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, whole) {
		t.Errorf("joined %d bytes, want %d", len(got), len(whole))
	}
	if _, err := Join(frags[:1]); err == nil {
		t.Error("missing fragment accepted")
	}
	if _, err := Join(append(frags, frags[0])); err == nil {
		t.Error("duplicate fragment accepted")
	}
}

func TestReadExport(t *testing.T) {
	whole := loadHex(t, "indx_compressed.hex")
	first, second := whole[:300], whole[300:]
	pad := func(b []byte) string {
		p := make([]byte, 512)
		copy(p, b)
		return strings.ToUpper(hex.EncodeToString(p))
	}
	// SE16H writes CRLF, a client column, attributes that are not part of the
	// key, and the fragments in whatever order the database returned them.
	csv := "MANDT,RELID,SRTFD,SRTF2,AEDAT,USERA,CLUSTR,CLUSTD\r\n" +
		"001,ZV,VSPFIX,1,20260904,TESTUSER," + fmt.Sprint(len(second)) + "," + pad(second) + "\r\n" +
		"001,ZV,VSPFIX,0,20260904,TESTUSER,300," + pad(first) + "\r\n" +
		"001,ZV,OTHER,0,20260904,TESTUSER," + fmt.Sprint(len(whole)) + "," + strings.ToUpper(hex.EncodeToString(whole)) + "\r\n"
	records, err := ReadExport(strings.NewReader(csv), "AEDAT", "USERA")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("%d records", len(records))
	}
	if records[0].KeyString() != "RELID=ZV, SRTFD=VSPFIX" || records[0].Parts != 2 {
		t.Errorf("record 1: %s, %d parts", records[0].KeyString(), records[0].Parts)
	}
	if !reflect.DeepEqual(records[0].Blob, whole) || !reflect.DeepEqual(records[1].Blob, whole) {
		t.Error("blobs do not round-trip")
	}
	// Semicolons and lower-case hex, as SE16N writes them.
	semi := "RELID;SRTFD;SRTF2;CLUSTR;CLUSTD\n" +
		"ZV;VSPFIX;0;" + fmt.Sprint(len(whole)) + ";" + hex.EncodeToString(whole) + "\n"
	records, err = ReadExport(strings.NewReader(semi))
	if err != nil || len(records) != 1 || !reflect.DeepEqual(records[0].Blob, whole) {
		t.Errorf("semicolon export: %v, %d records", err, len(records))
	}
	if _, err := ReadExport(strings.NewReader("A,B\n1,2\n")); err == nil {
		t.Error("export without CLUSTD accepted")
	}
}

func TestDecfloat(t *testing.T) {
	for _, tc := range []struct {
		hex, want string
	}{
		// decfloat34 3.14159 as the fixture holds it.
		{"D9500600000000000000000000C00622", "3.14159"},
		// decfloat34 zero.
		{"00000000000000000000000000004022", "0"},
	} {
		raw, _ := hex.DecodeString(tc.hex)
		got, ok := decodeDecfloat(raw)
		if !ok || got != tc.want {
			t.Errorf("%s: got %q ok=%v, want %q", tc.hex, got, ok, tc.want)
		}
	}
}

func TestPacked(t *testing.T) {
	for _, tc := range []struct {
		hex  string
		dec  int
		want string
	}{
		{"000000001234567D", 2, "-12345.67"},
		{"000000001234567C", 2, "12345.67"},
		{"0C", 0, "0"},
		{"005C", 3, "0.005"},
		{"20260904123456123456 7C", 7, "20260904123456.1234567"},
	} {
		raw, _ := hex.DecodeString(strings.ReplaceAll(tc.hex, " ", ""))
		if got := decodePacked(raw, tc.dec); got != tc.want {
			t.Errorf("%s/%d: %q, want %q", tc.hex, tc.dec, got, tc.want)
		}
	}
}
