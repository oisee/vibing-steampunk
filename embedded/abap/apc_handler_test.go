package embedded

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func apcHandlerSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "src", "zcl_vsp_apc_handler.clas.abap"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The handler finds its services beyond the four it always has, and only
// active classes named ZCL_VSP_*_SERVICE that implement ZIF_VSP_SERVICE: an
// arbitrary class cannot put itself on the WebSocket.
func TestAPCHandlerDiscoversServicesByNameAndInterface(t *testing.T) {
	src := apcHandlerSource(t)
	disc := strings.ToUpper(strings.Join(methodStatements(abapStatements(src), "DISCOVER_SERVICES"), "\n"))
	for _, want := range []string{
		"FROM SEOMETAREL",
		"REFCLSNAME = 'ZIF_VSP_SERVICE'",
		"RELTYPE = '1'",
		"VERSION = '1'",
		`CLSNAME LIKE 'ZCL\_VSP\_%\_SERVICE' ESCAPE '\'`,
		"ORDER BY CLSNAME",
		"CREATE OBJECT LO_SERVICE TYPE (LS_CLASS-CLSNAME)",
		"ADD_SERVICE( LO_SERVICE )",
	} {
		if !strings.Contains(disc, want) {
			t.Errorf("discover_services lacks %s:\n%s", want, disc)
		}
	}
	// A class that cannot be created (abapGit missing under the git service)
	// is skipped, not fatal.
	if !regexp.MustCompile(`CATCH CX_SY_CREATE_OBJECT_ERROR`).MatchString(disc) {
		t.Error("discover_services must skip a class that cannot be created")
	}
	// The optional services are not named any more: they are found.
	up := strings.ToUpper(src)
	for _, name := range []string{"ZCL_VSP_GIT_SERVICE", "ZCL_VSP_TRANSPORT_SERVICE", "ZCL_VSP_FORM_SERVICE"} {
		if strings.Contains(up, name) {
			t.Errorf("the handler names %s; it should find it", name)
		}
	}
}

// Every service, the four built in and the ones found, goes through
// add_service, which refuses a second claim on a domain and records both
// classes; with such a record the welcome fails and so does every request.
func TestAPCHandlerDuplicateDomainIsAnError(t *testing.T) {
	src := apcHandlerSource(t)
	stmts := abapStatements(src)
	ctor := strings.ToUpper(strings.Join(methodStatements(stmts, "CLASS_CONSTRUCTOR"), "\n"))
	if strings.Contains(ctor, "APPEND") || strings.Count(ctor, "ADD_SERVICE( NEW ") != 4 || !strings.Contains(ctor, "DISCOVER_SERVICES( )") {
		t.Errorf("class_constructor must add the four built-in services through add_service and then discover:\n%s", ctor)
	}
	add := strings.ToUpper(strings.Join(methodStatements(stmts, "ADD_SERVICE"), "\n"))
	if !strings.Contains(add, "GET_DOMAIN( ) = LV_DOMAIN") || !strings.Contains(add, "GV_SERVICES_ERROR") ||
		strings.Count(add, "GET_CLASS_NAME(") != 2 {
		t.Errorf("add_service must name both classes of a duplicate domain:\n%s", add)
	}
	if strings.Contains(ctor, "RAISE") || strings.Contains(add, "RAISE") {
		t.Error("a duplicate must not raise from the class constructor: the class would be unusable for the session")
	}
	start := strings.ToUpper(strings.Join(methodStatements(stmts, "IF_APC_WSP_EXTENSION~ON_START"), "\n"))
	if !strings.Contains(start, "IF GV_SERVICES_ERROR IS NOT INITIAL") || !strings.Contains(start, "IV_CODE = 'DUPLICATE_DOMAIN'") {
		t.Error("on_start must answer the welcome with DUPLICATE_DOMAIN")
	}
	route := strings.ToUpper(strings.Join(methodStatements(stmts, "ROUTE_MESSAGE"), "\n"))
	if !strings.HasPrefix(route, "IF GV_SERVICES_ERROR IS NOT INITIAL\n") {
		t.Errorf("route_message must refuse every request first while a duplicate is recorded:\n%s", route)
	}
}
