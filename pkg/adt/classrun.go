package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// classNamePattern accepts ABAP class names: letters, digits, underscores
// and the slashes of a namespace (/NS/CL_X). The name becomes a URL path
// segment, so nothing else may pass.
var classNamePattern = regexp.MustCompile(`^[A-Z0-9_/]+$`)

// RunClass runs the main method of a class that implements
// IF_OO_ADT_CLASSRUN, as "Run as ABAP Application (Console)" (F9) does in
// Eclipse ADT, and returns what the class wrote to the console.
//
// The request is an ADT request, so ADT breakpoints apply to it and to RFC
// sessions it starts. That makes it a trigger for debugging code that has no
// HTTP or RFC entry of its own: a class that starts an asynchronous RFC,
// which runs a report a few seconds later, returns at once and leaves time
// for DebuggerListen.
//
// It executes code: --read-only and --disallowed-ops W refuse it, and with
// --allowed-packages the class must be in an allowed package. Both checks
// run before anything is sent.
func (c *Client) RunClass(ctx context.Context, className string) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(className))
	if name == "" || !classNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid class name %q", className)
	}
	if err := c.checkSafety(OpWorkflow, "RunClass"); err != nil {
		return "", err
	}
	classURL := "/sap/bc/adt/oo/classes/" + url.PathEscape(strings.ToLower(name))
	if err := c.checkObjectPackageSafety(ctx, classURL, false); err != nil {
		return "", err
	}
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/oo/classrun/"+url.PathEscape(name), &RequestOptions{
		Method: http.MethodPost,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("running class %s: %w", name, err)
	}
	return string(resp.Body), nil
}
