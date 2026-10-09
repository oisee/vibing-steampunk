package landscape

// LoadError identifies failures without retaining source paths, URLs or XML.
type LoadError struct {
	Code    string
	Include bool
	Status  int
}

var errorReasons = map[string]string{
	"file_not_found":     "landscape file not found",
	"file_access_denied": "landscape file access denied",
	"source_read_failed": "landscape read failed",
	"invalid_xml":        "invalid landscape XML",
	"size_limit":         "landscape size cap exceeded",
	"include_cycle":      "landscape include cycle",
	"include_limit":      "landscape include count or depth cap exceeded",
	"unsupported_scheme": "unsupported landscape source scheme",
	"invalid_source":     "invalid landscape source",
	"https_unavailable":  "HTTPS landscape source unavailable",
	"https_status":       "HTTPS landscape source returned an unsuccessful status",
	"empty_include":      "empty landscape include",
	"no_sources":         "no landscape sources supplied",
}

func ErrorReason(code string) (string, bool) { reason, ok := errorReasons[code]; return reason, ok }
func (e *LoadError) Error() string {
	if reason, ok := ErrorReason(e.Code); ok {
		return reason
	}
	return "landscape load failed"
}
