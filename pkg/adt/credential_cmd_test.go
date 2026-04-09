package adt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeCredHelper writes a helper script to tmpDir that outputs the given text.
// Returns the command args to execute it.
func writeCredHelper(t *testing.T, tmpDir, output string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(tmpDir, "cred.cmd")
		// Use @echo off + echo to avoid cmd noise. Note: echo in batch
		// does not interpret JSON special chars.
		content := "@echo off\r\necho " + output + "\r\n"
		if err := os.WriteFile(script, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write helper script: %v", err)
		}
		return []string{"cmd", "/c", script}
	}
	script := filepath.Join(tmpDir, "cred.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", output)
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write helper script: %v", err)
	}
	return []string{"sh", script}
}

// writeFailHelper writes a helper script that exits with a non-zero code.
func writeFailHelper(t *testing.T, tmpDir string, exitCode int) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(tmpDir, "fail.cmd")
		content := fmt.Sprintf("@exit /b %d\r\n", exitCode)
		if err := os.WriteFile(script, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write fail script: %v", err)
		}
		return []string{"cmd", "/c", script}
	}
	script := filepath.Join(tmpDir, "fail.sh")
	content := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write fail script: %v", err)
	}
	return []string{"sh", script}
}

func TestCredentialCmd_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	args := writeCredHelper(t, tmpDir, `{"username":"admin@example.com","password":"secret123"}`)

	user, pass, err := RunCredentialCmd(context.Background(), args, false)
	if err != nil {
		t.Fatalf("RunCredentialCmd failed: %v", err)
	}
	if user != "admin@example.com" {
		t.Errorf("expected username admin@example.com, got %q", user)
	}
	if pass != "secret123" {
		t.Errorf("expected password secret123, got %q", pass)
	}
}

func TestCredentialCmd_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	args := writeCredHelper(t, tmpDir, "not-json-at-all")

	_, _, err := RunCredentialCmd(context.Background(), args, false)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' error, got: %v", err)
	}
}

func TestCredentialCmd_Timeout(t *testing.T) {
	// Use an immediately-expired context.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // Ensure context is expired.

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/c", "ping", "-n", "10", "127.0.0.1"}
	} else {
		args = []string{"sleep", "10"}
	}

	_, _, err := RunCredentialCmd(ctx, args, false)
	if err == nil {
		t.Fatal("expected error for timeout, got nil")
	}
	if !strings.Contains(err.Error(), "command failed") && !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout or command failed error, got: %v", err)
	}
}

func TestCredentialCmd_NonZeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	args := writeFailHelper(t, tmpDir, 1)

	_, _, err := RunCredentialCmd(context.Background(), args, false)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("expected 'command failed' error, got: %v", err)
	}
}

func TestCredentialCmd_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"missing username", `{"password":"pass"}`, "missing 'username'"},
		{"missing password", `{"username":"user"}`, "missing 'password'"},
		{"empty username", `{"username":"","password":"pass"}`, "missing 'username'"},
		{"empty password", `{"username":"user","password":""}`, "missing 'password'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			args := writeCredHelper(t, tmpDir, tt.json)

			_, _, err := RunCredentialCmd(context.Background(), args, false)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestCredentialCmd_EmptyCommand(t *testing.T) {
	_, _, err := RunCredentialCmd(context.Background(), nil, false)
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected 'empty command' error, got: %v", err)
	}
}

func TestParseCredentialCmd(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"keepassxc-cli show -s db.kdbx SAP/K0B", 5},
		{"simple-cmd", 1},
		{"cmd arg1 arg2", 3},
		{"", 0},
		{"  spaced  cmd  ", 2},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseCredentialCmd(tt.input)
			if len(got) != tt.want {
				t.Errorf("ParseCredentialCmd(%q) = %d args, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestCredentialCmd_VerboseMode(t *testing.T) {
	tmpDir := t.TempDir()
	args := writeCredHelper(t, tmpDir, `{"username":"user","password":"pass"}`)

	user, pass, err := RunCredentialCmd(context.Background(), args, true)
	if err != nil {
		t.Fatalf("RunCredentialCmd (verbose) failed: %v", err)
	}
	if user != "user" || pass != "pass" {
		t.Errorf("unexpected credentials: user=%q pass=%q", user, pass)
	}
}
