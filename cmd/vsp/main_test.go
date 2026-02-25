package main

import (
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func withTestConfig(t *testing.T, c *mcp.Config) {
	t.Helper()
	old := cfg
	cfg = c
	t.Cleanup(func() {
		cfg = old
	})
}

func withViperTransport(t *testing.T, value string) {
	t.Helper()
	old := viper.Get("TRANSPORT")
	viper.Set("TRANSPORT", value)
	t.Cleanup(func() {
		viper.Set("TRANSPORT", old)
	})
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("transport", "stdio", "")
	return cmd
}

func TestValidateConfig_TransportNormalization(t *testing.T) {
	withTestConfig(t, &mcp.Config{
		BaseURL:   "https://sap.example.com:44300",
		Mode:      "focused",
		Transport: " HTTP-Streamable ",
	})

	if err := validateConfig(); err != nil {
		t.Fatalf("validateConfig() unexpected error: %v", err)
	}
	if cfg.Transport != "http-streamable" {
		t.Fatalf("expected normalized transport http-streamable, got %s", cfg.Transport)
	}
}

func TestValidateConfig_TransportDefault(t *testing.T) {
	withTestConfig(t, &mcp.Config{
		BaseURL:   "https://sap.example.com:44300",
		Mode:      "focused",
		Transport: "   ",
	})

	if err := validateConfig(); err != nil {
		t.Fatalf("validateConfig() unexpected error: %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Fatalf("expected default transport stdio, got %s", cfg.Transport)
	}
}

func TestValidateConfig_TransportInvalid(t *testing.T) {
	withTestConfig(t, &mcp.Config{
		BaseURL:   "https://sap.example.com:44300",
		Mode:      "focused",
		Transport: "sse",
	})

	if err := validateConfig(); err == nil {
		t.Fatal("expected invalid transport error")
	}
}

func TestResolveConfig_TransportFromViperWhenFlagNotChanged(t *testing.T) {
	withTestConfig(t, &mcp.Config{
		Transport: "stdio",
	})
	withViperTransport(t, "http-streamable")

	cmd := newTestCmd()
	resolveConfig(cmd)

	if cfg.Transport != "http-streamable" {
		t.Fatalf("expected transport from viper, got %s", cfg.Transport)
	}
}

func TestResolveConfig_TransportFlagPrecedence(t *testing.T) {
	withTestConfig(t, &mcp.Config{
		Transport: "stdio",
	})
	withViperTransport(t, "http-streamable")

	cmd := newTestCmd()
	if err := cmd.Flags().Set("transport", "stdio"); err != nil {
		t.Fatalf("failed to set transport flag: %v", err)
	}

	resolveConfig(cmd)

	if cfg.Transport != "stdio" {
		t.Fatalf("expected flag precedence to keep stdio, got %s", cfg.Transport)
	}
}

func TestRootCmdHasTransportFlag(t *testing.T) {
	flag := rootCmd.Flags().Lookup("transport")
	if flag == nil {
		t.Fatal("transport flag is not registered on root command")
	}
	if flag.DefValue != "stdio" {
		t.Fatalf("expected transport default stdio, got %s", flag.DefValue)
	}
}
