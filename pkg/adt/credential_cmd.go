package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// credentialCmdTimeout is the default timeout for external credential commands.
const credentialCmdTimeout = 30 * time.Second

// credentialResult is the expected JSON output from a credential command.
type credentialResult struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RunCredentialCmd executes an external credential command and parses JSON output.
//
// The command is executed via exec.Command (argv-based, no shell) to prevent
// shell injection when the command is sourced from env/config. The command must
// write JSON to stdout: {"username": "...", "password": "..."}.
//
// Stderr from the command is discarded (never logged, may contain secrets).
// Stdout is read into a byte buffer and zeroed after JSON parsing.
func RunCredentialCmd(ctx context.Context, args []string, verbose bool) (username, password string, err error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("credential-cmd: empty command")
	}

	// Apply timeout to the context.
	ctx, cancel := context.WithTimeout(ctx, credentialCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stderr = io.Discard // Discard stderr — may contain secrets.
	cmd.Stdin = nil  // No stdin — non-interactive.

	if verbose {
		fmt.Fprintf(os.Stderr, "[CREDENTIAL-CMD] Executing: %s (%d args)\n", args[0], len(args)-1)
	}

	output, err := cmd.Output()
	defer zeroBytes(output) // Zero buffer after parsing, even on error paths.
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", fmt.Errorf("credential-cmd: timed out after %s", credentialCmdTimeout)
		}
		// Never include output in error — may contain partial secrets.
		return "", "", fmt.Errorf("credential-cmd: command failed: %w", err)
	}

	var result credentialResult
	if err := json.Unmarshal(output, &result); err != nil {
		return "", "", fmt.Errorf("credential-cmd: invalid JSON output: %w", err)
	}

	if result.Username == "" {
		return "", "", fmt.Errorf("credential-cmd: missing 'username' field in JSON output")
	}
	if result.Password == "" {
		return "", "", fmt.Errorf("credential-cmd: missing 'password' field in JSON output")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[CREDENTIAL-CMD] Credentials received for user: %s\n", result.Username)
	}

	return result.Username, result.Password, nil
}

// ParseCredentialCmd splits a credential command string into argv tokens.
// Uses strings.Fields (whitespace splitting) — no shell quoting support.
// For complex quoting, use a wrapper script.
func ParseCredentialCmd(cmdStr string) []string {
	return strings.Fields(cmdStr)
}
