package vsp

import (
	"fmt"
	"strings"
)

// buildObjectURL constructs an ADT object URL from type and name.
func buildObjectURL(objType, name string) string {
	name = strings.ToLower(name)
	switch objType {
	case "CLAS":
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	case "PROG":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	case "INCL":
		return fmt.Sprintf("/sap/bc/adt/programs/includes/%s", name)
	case "INTF":
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	case "FUGR":
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", name)
	case "DDLS":
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", name)
	default:
		return ""
	}
}
