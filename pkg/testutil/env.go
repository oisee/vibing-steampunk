// ABOUTME: Test utilities for loading environment variables from .env files.
// ABOUTME: Used by integration tests to load SAP credentials.

package testutil

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	envOnce   sync.Once
	envLoaded bool
)

// LoadEnv reads a .env file and sets environment variables.
// Searches for .env in current directory and up to 5 parent directories.
// Environment variables already set take precedence over .env values.
// Safe to call multiple times - only loads once.
func LoadEnv() {
	envOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			return
		}

		for range 6 {
			envPath := filepath.Join(dir, ".env")
			if _, err := os.Stat(envPath); err == nil {
				if parseEnvFile(envPath) == nil {
					envLoaded = true
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	})
}

// EnvLoaded returns true if .env file was successfully loaded.
func EnvLoaded() bool {
	return envLoaded
}

// parseEnvFile reads a .env file and sets environment variables.
func parseEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse KEY=VALUE
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Only set if not already set (env vars take precedence)
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
	return scanner.Err()
}
