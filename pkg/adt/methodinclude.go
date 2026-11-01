package adt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// A cross-reference row says which *include* holds a reference, and for a class
// that include is a method: ZCL_X===========CM001. Upward tracing wants the
// method, and the number is not the answer on its own.
//
// The mapping is in TMDIR — (CLASSNAME, METHODINDX, METHODNAME) — and the CM
// suffix is that index in **base 36**, digits then letters. CL_DEP_TREE has
// CM001 to CM009 and CM00A to CM00Z, and TMDIR gives CM00Z's method index as
// 35 (HANDLE_EXPAND_NC), on 7.40 and on 7.50. It was first read as hexadecimal
// from a class whose methods stopped at CM00A; that decodes the first sixteen
// right, fails on CM00G and reads CM010 as 16 instead of 36.
//
// Two things this replaces, both of which looked reasonable and were wrong:
// reading the include as a program (a class-pool include is not addressable
// that way — 500 and 404), and taking the number as a position in the source (it
// is assigned when a method is created, so a method written later and inserted
// earlier gets a higher number).
//
// SAP's own resolver, CL_OO_CLASSNAME_SERVICE=>GET_METHOD_BY_INCLUDE, does not
// read TMDIR at all: it issues SYSTEM-CALL QUERY METHOD INCLUDE, a kernel call
// unavailable to us. TMDIR is the same knowledge in a table, and a table is
// readable over plain ADT with no Z code.

// MethodInclude is a class-pool include decoded into what it holds.
type MethodInclude struct {
	Include string `json:"include"`
	Class   string `json:"class"`
	// Method is empty when the include is not a method — a class's definition
	// or implementation section rather than one of its methods.
	Method string `json:"method,omitempty"`
	// Index is the method number the include encodes, or 0 for a section.
	Index int `json:"index,omitempty"`
	// Section names what the include is when it is not a method: CI, CU, CO,
	// CCDEF and so on. Reported rather than dropped, because "this reference
	// sits in the class definition" is a real answer.
	Section string `json:"section,omitempty"`
}

// splitClassInclude takes a class-pool include apart. The name is padded with
// '=' to a fixed width and the last characters name the section.
func splitClassInclude(include string) (class, section string, ok bool) {
	inc := strings.TrimSpace(strings.ToUpper(include))
	i := strings.Index(inc, "=")
	if i <= 0 {
		return "", "", false
	}
	class = inc[:i]
	section = strings.TrimLeft(inc[i:], "=")
	if class == "" || section == "" {
		return "", "", false
	}
	return class, section, true
}

// methodIndexFromSection decodes CM001 into 1. A section that is not a method
// yields false rather than zero, because zero is a legitimate index.
func methodIndexFromSection(section string) (int, bool) {
	if !strings.HasPrefix(section, "CM") || len(section) != 5 {
		return 0, false
	}
	// Include names are upper case; ParseInt would take cmxyz too.
	for _, r := range section[2:] {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(section[2:], 36, 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

// DecodeMethodIncludes resolves class-pool includes to the methods they hold.
//
// Includes are grouped by class so a class costs one query however many of its
// methods appear — a cross-reference sweep produces many rows from few classes,
// and asking per row would turn one question into hundreds.
func (c *Client) DecodeMethodIncludes(ctx context.Context, includes []string) (map[string]MethodInclude, error) {
	out := make(map[string]MethodInclude, len(includes))
	wanted := map[string]map[int]string{} // class → index → include

	for _, inc := range includes {
		class, section, ok := splitClassInclude(inc)
		if !ok {
			continue
		}
		entry := MethodInclude{Include: inc, Class: class, Section: section}
		idx, isMethod := methodIndexFromSection(section)
		if !isMethod {
			// A definition or implementation section. Complete as it stands.
			out[inc] = entry
			continue
		}
		entry.Index = idx
		entry.Section = ""
		out[inc] = entry
		if wanted[class] == nil {
			wanted[class] = map[int]string{}
		}
		wanted[class][idx] = inc
	}
	if len(wanted) == 0 {
		return out, nil
	}

	for class, byIndex := range wanted {
		if err := checkSQLLiteral(class); err != nil {
			continue
		}
		rows, err := c.RunQuery(ctx,
			fmt.Sprintf("SELECT CLASSNAME, METHODINDX, METHODNAME FROM TMDIR WHERE CLASSNAME = '%s'", class),
			5000)
		if err != nil || rows == nil {
			// The class stays decoded as far as it went: caller sees the class
			// and the index, and no method name. That is less than we wanted
			// and more than nothing, and it does not pretend the method is
			// absent.
			continue
		}
		for _, row := range rows.Rows {
			idx, convErr := strconv.Atoi(strings.TrimSpace(cell(row, "METHODINDX")))
			if convErr != nil {
				continue
			}
			inc, ok := byIndex[idx]
			if !ok {
				continue
			}
			name := strings.TrimSpace(cell(row, "METHODNAME"))
			if name == "" {
				continue
			}
			entry := out[inc]
			entry.Method = name
			out[inc] = entry
		}
	}
	return out, nil
}

// Qualified names the thing an include holds, as a person would say it.
func (m MethodInclude) Qualified() string {
	switch {
	case m.Method != "":
		return m.Class + "=>" + m.Method
	case m.Section != "":
		return m.Class + " (" + m.Section + ")"
	default:
		return m.Class
	}
}
