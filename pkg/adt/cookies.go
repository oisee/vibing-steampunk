package adt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadCookiesFromFile loads cookies from a Netscape-format cookie file.
// The file format has 7 tab-separated fields per line:
// domain, flag, path, secure, expiration, name, value
//
// Also supports simple key=value format as fallback.
func LoadCookiesFromFile(cookieFile string) (map[string]string, error) {
	cookies := make(map[string]string)

	file, err := os.Open(cookieFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse Netscape format (7 fields separated by tabs). A tabbed,
		// incomplete line is a partially written Netscape record, not a simple
		// cookie. Reject it so a concurrent file refresh never replaces a good
		// in-memory session with a fragment.
		parts := strings.Split(line, "\t")
		if len(parts) >= 7 {
			// domain, flag, path, secure, expiration, name, value
			name := parts[5]
			value := parts[6]
			if name == "" {
				return nil, fmt.Errorf("invalid Netscape cookie record: empty name")
			}
			cookies[name] = value
		} else if strings.Contains(line, "\t") {
			return nil, fmt.Errorf("invalid Netscape cookie record")
		} else if strings.Contains(line, "=") {
			// Simple key=value format fallback
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 || strings.TrimSpace(kv[0]) == "" {
				return nil, fmt.Errorf("invalid cookie record")
			}
			cookies[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		} else {
			return nil, fmt.Errorf("invalid cookie record")
		}
	}

	return cookies, scanner.Err()
}

// NewCookieFileReauthFunc returns a bounded, on-demand reader for a cookie
// file. It deliberately has no watcher: a failed safe read is what triggers a
// single recovery attempt. The callback accepts a stable, non-empty parse only;
// a missing, empty, malformed, or concurrently changing file leaves the
// transport's current cookies untouched.
func NewCookieFileReauthFunc(cookieFile string) (func(context.Context) (map[string]string, error), error) {
	absPath, err := filepath.Abs(cookieFile)
	if err != nil {
		return nil, fmt.Errorf("resolving cookie file path: %w", err)
	}

	return func(ctx context.Context) (map[string]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		before, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("reading refreshed cookie file: %w", err)
		}
		if before.Size() == 0 {
			return nil, fmt.Errorf("reading refreshed cookie file: file is empty")
		}

		cookies, err := LoadCookiesFromFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("reading refreshed cookie file: %w", err)
		}
		if len(cookies) == 0 {
			return nil, fmt.Errorf("reading refreshed cookie file: no valid cookies")
		}

		after, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("checking refreshed cookie file: %w", err)
		}
		if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return nil, fmt.Errorf("reading refreshed cookie file: file changed while being read")
		}
		return cookies, nil
	}, nil
}

// ParseCookieString parses a cookie string in the format "key1=val1; key2=val2".
func ParseCookieString(cookieString string) map[string]string {
	cookies := make(map[string]string)
	for _, cookie := range strings.Split(cookieString, ";") {
		cookie = strings.TrimSpace(cookie)
		if strings.Contains(cookie, "=") {
			kv := strings.SplitN(cookie, "=", 2)
			if len(kv) == 2 {
				cookies[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	return cookies
}
