package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(mcpToVspCmd)
	configCmd.AddCommand(vspToMcpCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage vsp configuration files",
	Long: `Manage vsp configuration files for different usage modes.

vsp supports three configuration methods:

1. .env file (or SAP_* env vars) - Default system for MCP server mode
2. .vsp.json - Multiple systems for CLI mode (vsp -s <system>)
3. .mcp.json - Claude Desktop MCP server configuration

Priority (highest to lowest):
  CLI flags > Environment variables > .env file > Defaults

Use 'vsp config init' to create example configuration files.
Use 'vsp config show' to display effective configuration.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create example configuration files",
	Long: `Create example configuration files with documentation.

Files created:
  .env.example           - Environment variables for default system
  .vsp.json.example      - Multiple systems for CLI mode
  .mcp.json.example      - Claude Desktop configuration

These are created as .example files to avoid overwriting existing configs.
Copy and edit them to create your actual configuration.`,
	RunE: runConfigInit,
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	files := map[string]string{
		".env.example":          envExample,
		".vsp.json.example":     vspSystemsExample,
		".mcp.json.example":     mcpJsonExample,
	}

	created := 0
	for name, content := range files {
		if _, err := os.Stat(name); err == nil {
			fmt.Printf("  SKIP %s (already exists)\n", name)
			continue
		}

		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to create %s: %w", name, err)
		}
		fmt.Printf("  CREATE %s\n", name)
		created++
	}

	fmt.Printf("\nCreated %d example files.\n", created)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Copy .env.example to .env and fill in your SAP credentials")
	fmt.Println("  2. Copy .vsp.json.example to .vsp.json for CLI mode")
	fmt.Println("  3. Copy .mcp.json.example to .mcp.json for Claude Desktop")
	fmt.Println("\nSee each file for detailed documentation.")

	return nil
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective configuration",
	Long: `Display the effective configuration from all sources.

Shows:
  - Environment variables (SAP_*)
  - Systems from .vsp-systems.json
  - .mcp.json if present`,
	RunE: runConfigShow,
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	fmt.Println("=== vsp Configuration ===")
	fmt.Println()

	// Environment variables
	fmt.Println("Environment Variables (SAP_*):")
	envVars := []string{"SAP_URL", "SAP_USER", "SAP_CLIENT", "SAP_LANGUAGE", "SAP_INSECURE", "SAP_MODE"}
	hasEnv := false
	for _, key := range envVars {
		if val := os.Getenv(key); val != "" {
			if key == "SAP_PASSWORD" {
				val = "***"
			}
			fmt.Printf("  %s=%s\n", key, val)
			hasEnv = true
		}
	}
	if os.Getenv("SAP_PASSWORD") != "" {
		fmt.Printf("  SAP_PASSWORD=***\n")
		hasEnv = true
	}
	if !hasEnv {
		fmt.Println("  (none set)")
	}

	// .env file
	fmt.Println("\n.env File:")
	if _, err := os.Stat(".env"); err == nil {
		fmt.Println("  Found: .env")
	} else {
		fmt.Println("  Not found")
	}

	// Systems config
	fmt.Println("\nSystems Config (.vsp.json):")
	cfg, path, err := config.LoadSystems()
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if cfg == nil {
		fmt.Println("  Not found")
	} else {
		fmt.Printf("  Found: %s\n", path)
		fmt.Printf("  Default: %s\n", cfg.Default)
		fmt.Println("  Systems:")
		for name, sys := range cfg.Systems {
			pwdStatus := "env"
			envKey := fmt.Sprintf("VSP_%s_PASSWORD", strings.ToUpper(name))
			if sys.Password != "" {
				pwdStatus = "inline"
			} else if os.Getenv(envKey) != "" {
				pwdStatus = "env ✓"
			} else {
				pwdStatus = "env ✗ (need " + envKey + ")"
			}
			marker := ""
			if name == cfg.Default {
				marker = " (default)"
			}
			fmt.Printf("    %s: %s [%s@%s] pwd:%s%s\n", name, sys.URL, sys.User, sys.Client, pwdStatus, marker)
		}
	}

	// MCP config
	fmt.Println("\nMCP Config (.mcp.json):")
	if mcpCfg, err := loadMCPConfig(); err == nil {
		fmt.Printf("  Found: .mcp.json\n")
		if vsp, ok := mcpCfg["mcpServers"].(map[string]interface{})["vsp"]; ok {
			if vspMap, ok := vsp.(map[string]interface{}); ok {
				if cmd, ok := vspMap["command"].(string); ok {
					fmt.Printf("  Command: %s\n", cmd)
				}
				if args, ok := vspMap["args"].([]interface{}); ok {
					argStrs := make([]string, len(args))
					for i, a := range args {
						argStrs[i] = fmt.Sprint(a)
					}
					// Mask password
					for i, a := range argStrs {
						if a == "--password" || a == "-p" {
							if i+1 < len(argStrs) {
								argStrs[i+1] = "***"
							}
						}
					}
					fmt.Printf("  Args: %s\n", strings.Join(argStrs, " "))
				}
			}
		} else {
			fmt.Println("  No 'vsp' server configured")
		}
	} else if os.IsNotExist(err) {
		fmt.Println("  Not found")
	} else {
		fmt.Printf("  Error: %v\n", err)
	}

	return nil
}

func loadMCPConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// --- mcp-to-vsp command ---

var mcpToVspCmd = &cobra.Command{
	Use:   "mcp-to-vsp",
	Short: "Import systems from .mcp.json to .vsp.json",
	Long: `Parse .mcp.json and create/update .vsp.json with system entries.

For each vsp-* server in .mcp.json, extracts:
  - URL from --url arg or env.SAP_URL
  - User from --user arg or env.SAP_USER
  - Password from env.SAP_PASSWORD (if not a placeholder)
  - Client from --client arg
  - Cookie auth from --cookie-file or --cookie-string
  - Other settings (insecure, read-only, etc.)`,
	RunE: runMcpToVsp,
}

func runMcpToVsp(cmd *cobra.Command, args []string) error {
	mcpCfg, err := loadMCPConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(".mcp.json not found in current directory")
		}
		return fmt.Errorf("failed to read .mcp.json: %w", err)
	}

	servers, ok := mcpCfg["mcpServers"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no mcpServers found in .mcp.json")
	}

	// Load existing .vsp.json or create new
	vspCfg, _, _ := config.LoadSystems()
	if vspCfg == nil {
		vspCfg = &config.SystemsConfig{
			Systems: make(map[string]config.SystemConfig),
		}
	}

	imported := 0
	for name, server := range servers {
		serverMap, ok := server.(map[string]interface{})
		if !ok {
			continue
		}

		// Parse args to extract settings
		sys := parseServerArgs(serverMap)
		if sys.URL == "" {
			fmt.Printf("  SKIP %s (no URL found)\n", name)
			continue
		}

		// Determine system name (strip 'vsp-' prefix if present)
		sysName := name
		if strings.HasPrefix(name, "vsp-") {
			sysName = strings.TrimPrefix(name, "vsp-")
		} else if name == "vsp" {
			sysName = "default"
		}

		// Check if exists
		action := "ADD"
		if _, exists := vspCfg.Systems[sysName]; exists {
			action = "UPDATE"
		}

		vspCfg.Systems[sysName] = sys
		pwdInfo := ""
		if sys.Password != "" {
			pwdInfo = " (pwd:imported)"
		}
		fmt.Printf("  %s %s: %s [%s@%s]%s\n", action, sysName, sys.URL, sys.User, sys.Client, pwdInfo)
		imported++

		// Set first system as default if none set
		if vspCfg.Default == "" {
			vspCfg.Default = sysName
		}
	}

	if imported == 0 {
		fmt.Println("No vsp servers found in .mcp.json")
		return nil
	}

	// Write .vsp.json
	data, err := json.MarshalIndent(vspCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(".vsp.json", data, 0600); err != nil {
		return fmt.Errorf("failed to write .vsp.json: %w", err)
	}

	fmt.Printf("\nImported %d systems to .vsp.json\n", imported)
	fmt.Println("Set passwords via: VSP_<SYSTEM>_PASSWORD environment variables")
	return nil
}

func parseServerArgs(serverMap map[string]interface{}) config.SystemConfig {
	sys := config.SystemConfig{
		Client:   "001",
		Language: "EN",
	}

	// Parse args array
	if args, ok := serverMap["args"].([]interface{}); ok {
		for i := 0; i < len(args)-1; i++ {
			arg := fmt.Sprint(args[i])
			val := fmt.Sprint(args[i+1])

			switch arg {
			case "--url", "-u":
				sys.URL = val
			case "--user":
				sys.User = val
			case "--client":
				sys.Client = val
			case "--language":
				sys.Language = val
			case "--cookie-file":
				sys.CookieFile = val
			case "--cookie-string":
				sys.CookieString = val
			case "--insecure":
				sys.Insecure = true
				continue // insecure is a flag, not key-value
			case "--read-only":
				sys.ReadOnly = true
				continue
			}
		}
		// Check for standalone flags
		for _, arg := range args {
			switch fmt.Sprint(arg) {
			case "--insecure":
				sys.Insecure = true
			case "--read-only":
				sys.ReadOnly = true
			}
		}
	}

	// Also check env block for all settings
	if env, ok := serverMap["env"].(map[string]interface{}); ok {
		if sys.URL == "" {
			if url, ok := env["SAP_URL"].(string); ok {
				sys.URL = url
			}
		}
		if sys.User == "" {
			if user, ok := env["SAP_USER"].(string); ok {
				sys.User = user
			}
		}
		// Import password from env block (if it's a real value, not a placeholder)
		if pwd, ok := env["SAP_PASSWORD"].(string); ok {
			if pwd != "" && pwd != "YOUR_PASSWORD_HERE" && !strings.HasPrefix(pwd, "YOUR_") {
				sys.Password = pwd
			}
		}
		// Client
		if client, ok := env["SAP_CLIENT"].(string); ok && client != "" {
			sys.Client = client
		}
		// Language
		if lang, ok := env["SAP_LANGUAGE"].(string); ok && lang != "" {
			sys.Language = lang
		}
		// Insecure
		if insecure, ok := env["SAP_INSECURE"].(string); ok {
			sys.Insecure = insecure == "true"
		}
		// Cookie file
		if cookieFile, ok := env["SAP_COOKIE_FILE"].(string); ok && cookieFile != "" {
			sys.CookieFile = cookieFile
		}
		// Cookie string
		if cookieStr, ok := env["SAP_COOKIE_STRING"].(string); ok && cookieStr != "" {
			sys.CookieString = cookieStr
		}
	}

	return sys
}

// --- vsp-to-mcp command ---

var vspToMcpCmd = &cobra.Command{
	Use:   "vsp-to-mcp",
	Short: "Export systems from .vsp.json to .mcp.json",
	Long: `Generate .mcp.json entries from .vsp.json systems.

Creates mcpServers entries for each system in .vsp.json.
Passwords are placed in the 'env' block (you need to fill them in).`,
	RunE: runVspToMcp,
}

func runVspToMcp(cmd *cobra.Command, args []string) error {
	vspCfg, path, err := config.LoadSystems()
	if err != nil {
		return fmt.Errorf("failed to load .vsp.json: %w", err)
	}
	if vspCfg == nil {
		return fmt.Errorf(".vsp.json not found. Run 'vsp config init' first")
	}

	fmt.Printf("Reading from: %s\n\n", path)

	// Load existing .mcp.json or create new
	mcpCfg, _ := loadMCPConfig()
	if mcpCfg == nil {
		mcpCfg = make(map[string]interface{})
	}

	servers, ok := mcpCfg["mcpServers"].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
		mcpCfg["mcpServers"] = servers
	}

	// Get executable path
	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = "vsp"
	}

	exported := 0
	for name, sys := range vspCfg.Systems {
		// Build server entry
		serverName := "vsp"
		if name != "default" && name != vspCfg.Default {
			serverName = "vsp-" + name
		}

		serverArgs := []string{
			"--url", sys.URL,
		}

		// Cookie auth or user/password auth
		if sys.CookieFile != "" {
			serverArgs = append(serverArgs, "--cookie-file", sys.CookieFile)
		} else if sys.CookieString != "" {
			serverArgs = append(serverArgs, "--cookie-string", sys.CookieString)
		} else if sys.User != "" {
			serverArgs = append(serverArgs, "--user", sys.User)
		}

		serverArgs = append(serverArgs, "--client", sys.Client)

		if sys.Insecure {
			serverArgs = append(serverArgs, "--insecure")
		}
		if sys.ReadOnly {
			serverArgs = append(serverArgs, "--read-only")
		}
		if len(sys.AllowedPackages) > 0 {
			serverArgs = append(serverArgs, "--allowed-packages", strings.Join(sys.AllowedPackages, ","))
		}

		// Build env block - only add password placeholder if using user auth
		envBlock := make(map[string]string)
		if sys.CookieFile == "" && sys.CookieString == "" {
			envBlock["SAP_PASSWORD"] = "YOUR_PASSWORD_HERE"
		}

		server := map[string]interface{}{
			"command": execPath,
			"args":    serverArgs,
		}
		if len(envBlock) > 0 {
			server["env"] = envBlock
		}

		action := "ADD"
		if _, exists := servers[serverName]; exists {
			action = "UPDATE"
		}

		servers[serverName] = server
		fmt.Printf("  %s %s: %s [%s]\n", action, serverName, sys.URL, sys.User)
		exported++
	}

	// Write .mcp.json
	data, err := json.MarshalIndent(mcpCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(".mcp.json", data, 0600); err != nil {
		return fmt.Errorf("failed to write .mcp.json: %w", err)
	}

	fmt.Printf("\nExported %d systems to .mcp.json\n", exported)
	fmt.Println("IMPORTANT: Edit .mcp.json and fill in SAP_PASSWORD values!")
	return nil
}

// Example configuration files

const envExample = `# vsp Environment Configuration
# Copy this file to .env and fill in your SAP credentials.
#
# This is the DEFAULT system used when running vsp without --system flag.
# For multiple systems, use .vsp-systems.json instead.
#
# Priority: CLI flags > Environment variables > .env > Defaults

# SAP Connection (required)
SAP_URL=http://your-sap-host:50000
SAP_USER=YOUR_USERNAME
SAP_PASSWORD=YOUR_PASSWORD

# SAP Options (optional)
SAP_CLIENT=001
SAP_LANGUAGE=EN
SAP_INSECURE=false

# Tool Mode (optional)
# focused = 19 essential tools (default)
# expert = all 45+ tools
SAP_MODE=focused

# Safety Options (optional)
# SAP_READ_ONLY=false
# SAP_BLOCK_FREE_SQL=false
# SAP_ALLOWED_PACKAGES=Z*,Y*
# SAP_ALLOWED_OPS=RSQ

# Feature Flags (optional, values: auto/on/off)
# SAP_FEATURE_HANA=auto
# SAP_FEATURE_ABAPGIT=auto
# SAP_FEATURE_RAP=auto
# SAP_FEATURE_AMDP=auto
# SAP_FEATURE_UI5=auto
# SAP_FEATURE_TRANSPORT=auto
`

var vspSystemsExample = func() string {
	cfg := config.SystemsConfig{
		Default: "dev",
		Systems: map[string]config.SystemConfig{
			"dev": {
				URL:      "http://dev-sap.example.com:50000",
				User:     "DEVELOPER",
				Client:   "001",
				Language: "EN",
			},
			"a4h": {
				URL:      "http://a4h.local:50000",
				User:     "ADMIN",
				Client:   "001",
				Insecure: true,
			},
			"prod": {
				URL:             "https://prod-sap.example.com:44300",
				User:            "READONLY",
				Client:          "100",
				ReadOnly:        true,
				AllowedPackages: []string{"Z*", "Y*"},
			},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")

	return fmt.Sprintf(`// vsp Systems Configuration
// Copy this file to .vsp.json and edit for your systems.
//
// Usage: vsp -s <system> <command>
// Example: vsp -s dev search "ZCL_*"
//
// Passwords are loaded from environment variables:
//   VSP_<SYSTEM>_PASSWORD (e.g., VSP_DEV_PASSWORD, VSP_A4H_PASSWORD)
//
// Config file locations (searched in order):
//   .vsp.json                (current directory, preferred)
//   .vsp/systems.json        (current directory)
//   ~/.vsp.json              (home directory)
//   ~/.vsp/systems.json      (home directory)

%s
`, string(data))
}()

var mcpJsonExample = func() string {
	// Get the executable path for the example
	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = "/path/to/vsp"
	}
	execPath = filepath.Base(execPath) // Just use the binary name

	return fmt.Sprintf(`{
  "_comment": "vsp MCP Server Configuration for Claude Desktop",
  "_docs": [
    "Copy this file to .mcp.json in your project directory.",
    "Or add to ~/.config/claude/claude_desktop_config.json",
    "",
    "Password options:",
    "  1. Use 'env' block (recommended): SAP_PASSWORD loaded from environment",
    "  2. Use --password in args (not recommended): visible in config",
    "",
    "Multiple systems: Create separate server entries (vsp-dev, vsp-prod, etc.)"
  ],
  "mcpServers": {
    "vsp": {
      "command": "%s",
      "args": [
        "--url", "http://your-sap-host:50000",
        "--user", "YOUR_USERNAME",
        "--client", "001",
        "--mode", "focused"
      ],
      "env": {
        "SAP_PASSWORD": "YOUR_PASSWORD_HERE"
      }
    },
    "vsp-dev": {
      "command": "%s",
      "args": ["--url", "http://dev:50000", "--user", "DEV_USER"],
      "env": {"SAP_PASSWORD": "dev_password"}
    },
    "vsp-prod": {
      "command": "%s",
      "args": [
        "--url", "https://prod:44300",
        "--user", "PROD_USER",
        "--read-only",
        "--allowed-packages", "Z*,Y*"
      ],
      "env": {"SAP_PASSWORD": "prod_password"}
    }
  }
}
`, execPath, execPath, execPath)
}()
