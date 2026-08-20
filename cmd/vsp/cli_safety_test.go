package main

import (
	"reflect"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mcp"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
)

func TestSafetyFlagsAreInheritedByCLISubcommands(t *testing.T) {
	flag := searchCmd.InheritedFlags().Lookup("allow-transportable-edits")
	if flag == nil {
		t.Fatal("--allow-transportable-edits is not inherited by CLI subcommands")
	}
}

func TestResolveCLISafetyPreservesProfileAndAppliesExplicitOverrides(t *testing.T) {
	oldCfg := cfg
	cfg = &mcp.Config{
		ReadOnly:                false,
		AllowedPackages:         []string{"ZFLAG*"},
		AllowedTransports:       []string{"REQ-FLAG*"},
		AllowTransportableEdits: true,
	}
	t.Cleanup(func() { cfg = oldCfg })

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("read-only", false, "")
	cmd.Flags().StringSlice("allowed-packages", nil, "")
	cmd.Flags().StringSlice("allowed-transports", nil, "")
	cmd.Flags().Bool("allow-transportable-edits", false, "")
	if err := cmd.Flags().Set("allow-transportable-edits", "true"); err != nil {
		t.Fatal(err)
	}

	profile := &config.SystemConfig{
		ReadOnly:          true,
		AllowedPackages:   []string{"ZPROFILE*"},
		AllowedTransports: []string{"REQ-PROFILE*"},
	}
	safety := resolveCLISafety(cmd, profile)

	if !safety.ReadOnly {
		t.Error("profile read_only setting was not preserved")
	}
	if !reflect.DeepEqual(safety.AllowedPackages, profile.AllowedPackages) {
		t.Fatalf("allowed packages = %v, want %v", safety.AllowedPackages, profile.AllowedPackages)
	}
	if !reflect.DeepEqual(safety.AllowedTransports, profile.AllowedTransports) {
		t.Fatalf("allowed transports = %v, want %v", safety.AllowedTransports, profile.AllowedTransports)
	}
	if !safety.AllowTransportableEdits {
		t.Error("explicit flag did not override the profile")
	}
}

func TestResolveCLISafetyUsesEnvironmentOverride(t *testing.T) {
	oldCfg := cfg
	cfg = &mcp.Config{AllowTransportableEdits: true}
	t.Cleanup(func() { cfg = oldCfg })
	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "true")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("allow-transportable-edits", false, "")
	safety := resolveCLISafety(cmd, &config.SystemConfig{})
	if !safety.AllowTransportableEdits {
		t.Error("SAP_ALLOW_TRANSPORTABLE_EDITS was not propagated")
	}
}

func TestGetClientInstallsResolvedSafety(t *testing.T) {
	params := &systemParams{
		URL:      "https://example.invalid",
		User:     "user",
		Password: "password",
		Client:   "001",
		Language: "EN",
	}
	params.Safety.AllowTransportableEdits = true
	params.Safety.AllowedTransports = []string{"REQ-TEST*"}

	client, err := getClient(params)
	if err != nil {
		t.Fatal(err)
	}
	if !client.Safety().AllowTransportableEdits {
		t.Error("resolved safety was not installed on the ADT client")
	}
	if !reflect.DeepEqual(client.Safety().AllowedTransports, params.Safety.AllowedTransports) {
		t.Fatalf("allowed transports = %v, want %v", client.Safety().AllowedTransports, params.Safety.AllowedTransports)
	}
}
