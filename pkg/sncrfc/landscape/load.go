package landscape

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Load reads existing SAP landscape sources and declared includes in memory.
// Local paths and file: URIs refer to the same filesystem source. HTTPS sources
// receive no credentials/cookies and never redirect. No source fallback/retry.
func Load(ctx context.Context, paths []string) (*Landscape, error) {
	result := &Landscape{}
	active := map[string]bool{}
	done := map[string]bool{}
	total := 0
	visited := 0
	var read func(string, int) error
	read = func(source string, depth int) error {
		fail := func(code string) error { return &LoadError{Code: code, Include: depth > 0} }
		if depth > 8 {
			return fail("include_limit")
		}
		location, u, err := sourceLocation(source)
		if err != nil {
			var e *LoadError
			if errors.As(err, &e) {
				copy := *e
				copy.Include = depth > 0
				return &copy
			}
			return fail("invalid_source")
		}
		if active[location] {
			return fail("include_cycle")
		}
		if done[location] {
			return nil
		}
		if visited >= 32 {
			return fail("include_limit")
		}
		visited++
		active[location] = true
		defer delete(active, location)
		var reader io.ReadCloser
		if u != nil {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
			if err != nil {
				return fail("invalid_source")
			}
			// New non-reusable connections avoid automatic retries on stale connections.
			base, ok := http.DefaultTransport.(*http.Transport)
			if !ok {
				return fail("https_unavailable")
			}
			transport := base.Clone()
			transport.DisableKeepAlives = true
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			resp, err := client.Do(req)
			if err != nil {
				return fail("https_unavailable")
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return &LoadError{Code: "https_status", Include: depth > 0, Status: resp.StatusCode}
			}
			reader = resp.Body
		} else {
			f, err := os.Open(location)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fail("file_not_found")
				}
				if errors.Is(err, os.ErrPermission) {
					return fail("file_access_denied")
				}
				return fail("source_read_failed")
			}
			reader = f
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxXML+1))
		closeErr := reader.Close()
		total += len(data)
		if err != nil || closeErr != nil {
			return fail("source_read_failed")
		}
		if len(data) > maxXML || total > 16<<20 {
			return fail("size_limit")
		}
		l, err := Parse(strings.NewReader(string(data)))
		if err != nil {
			return fail("invalid_xml")
		}
		sort.SliceStable(l.Includes, func(i, j int) bool { return l.Includes[i].Index < l.Includes[j].Index })
		for _, inc := range l.Includes {
			if inc.URL == "" {
				return fail("empty_include")
			}
			child, err := includeLocation(location, u, inc.URL)
			if err != nil {
				return fail("invalid_source")
			}
			if err := read(child, depth+1); err != nil {
				return err
			}
		}
		for _, s := range l.Services {
			if !contains(result.Services, s) {
				result.Services = append(result.Services, s)
			}
		}
		for _, s := range l.Servers {
			if !contains(result.Servers, s) {
				result.Servers = append(result.Servers, s)
			}
		}
		for _, r := range l.Routers {
			if !contains(result.Routers, r) {
				result.Routers = append(result.Routers, r)
			}
		}
		done[location] = true
		return nil
	}
	for _, source := range paths {
		if err := read(source, 0); err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, &LoadError{Code: "no_sources"}
	}
	return result, nil
}

func sourceLocation(source string) (string, *url.URL, error) {
	fail := func(code string) (string, *url.URL, error) { return "", nil, &LoadError{Code: code} }
	// Native paths are recognized before URL parsing so drive letters and filename
	// percent signs/hashes retain their filesystem meaning.
	if filepath.IsAbs(source) || !hasScheme(source) {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return fail("invalid_source")
		}
		return absolute, nil, nil
	}
	u, err := url.Parse(source)
	if err != nil || u.User != nil || u.Fragment != "" {
		return fail("invalid_source")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Host == "" {
			return fail("invalid_source")
		}
		return u.String(), u, nil
	case "file":
		if u.RawQuery != "" || u.Opaque != "" || u.Path == "" {
			return fail("invalid_source")
		}
		path := u.Path
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			path = "//" + u.Host + "/" + strings.TrimPrefix(path, "/")
		}
		path = filepath.FromSlash(path)
		if !filepath.IsAbs(path) {
			return fail("invalid_source")
		}
		return filepath.Clean(path), nil, nil
	default:
		return fail("unsupported_scheme")
	}
}

func hasScheme(source string) bool {
	colon := strings.IndexByte(source, ':')
	if colon <= 0 {
		return false
	}
	for i, c := range source[:colon] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && ((c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'))) {
			return false
		}
	}
	return true
}

func includeLocation(parent string, parentURL *url.URL, child string) (string, error) {
	if parentURL != nil {
		ref, err := url.Parse(child)
		if err != nil {
			return "", err
		}
		return parentURL.ResolveReference(ref).String(), nil
	}
	if filepath.IsAbs(child) || hasScheme(child) {
		return child, nil
	}
	return filepath.Join(filepath.Dir(parent), child), nil
}

func contains[T comparable](items []T, want T) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}
