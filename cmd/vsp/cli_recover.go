package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// recoverFailedCreateCmd is the CLI wrapper around adt.Client.RecoverFailedCreate.
// It lets an operator clean up a zombie object left behind by an earlier
// failed CreateObject without going through MCP, and without needing the
// original session's lock handle. Safe to retry because the underlying
// primitive is idempotent: if the object no longer exists the command
// returns "already_clean" instead of failing.
var recoverFailedCreateCmd = &cobra.Command{
	Use:   "recover-failed-create <object-type> <name>",
	Short: "Clean up a zombie object left behind by a failed CreateObject",
	Long: `Recover a zombie object that a previous ` + "`create`" + ` attempt left
in SAP after a 5xx or partial-commit failure. The command probes whether
SAP currently persists the object; if yes, it runs best-effort
compensating cleanup (orphan-lock release, fresh lock acquisition,
DeleteObject); if no, it returns an idempotent no-op.

This is the operator-facing counterpart of the automatic reconcile path
that runs inside CreateObject itself — use it when an earlier create
left a stale lock / TR attachment that normal edit/delete flows cannot
clear because the original session's lock handle is gone.

Examples:
  vsp recover-failed-create CLAS ZCL_EXAMPLE --package '$TMP'
  vsp recover-failed-create PROG ZTEST --package '$TMP' --transport DEVK900001
  vsp recover-failed-create FUNC Z_MY_FM --parent ZMY_FG --package '$ZADT_VSP'`,
	Args: cobra.ExactArgs(2),
	RunE: runRecoverFailedCreate,
}

func init() {
	recoverFailedCreateCmd.Flags().String("package", "", "Package name (required for the safety gate)")
	recoverFailedCreateCmd.Flags().String("parent", "", "Parent function group (required for FUNC recovery)")
	recoverFailedCreateCmd.Flags().String("transport", "", "Transport request the zombie was attached to (optional)")
	recoverFailedCreateCmd.Flags().String("format", "text", "Output format: text or json")
	_ = recoverFailedCreateCmd.MarkFlagRequired("package")
	rootCmd.AddCommand(recoverFailedCreateCmd)
}

func runRecoverFailedCreate(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	objectType := normalizeCreatableType(args[0])
	if objectType == "" {
		return fmt.Errorf("unsupported object type %q (want CLAS, PROG, INTF, FUGR, FUNC, DDLS, BDEF, SRVD, SRVB, TABL, DEVC)", args[0])
	}
	name := strings.ToUpper(strings.TrimSpace(args[1]))
	pkg, _ := cmd.Flags().GetString("package")
	parent, _ := cmd.Flags().GetString("parent")
	transport, _ := cmd.Flags().GetString("transport")
	format, _ := cmd.Flags().GetString("format")

	opts := adt.CreateObjectOptions{
		ObjectType:  adt.CreatableObjectType(objectType),
		Name:        name,
		PackageName: strings.ToUpper(pkg),
		ParentName:  strings.ToUpper(parent),
		Transport:   strings.ToUpper(transport),
	}

	fmt.Fprintf(os.Stderr, "Recovering zombie %s %s from package %s", objectType, name, pkg)
	if transport != "" {
		fmt.Fprintf(os.Stderr, " (transport %s)", transport)
	}
	fmt.Fprintln(os.Stderr, "...")

	pce := client.RecoverFailedCreate(context.Background(), opts)
	status := classifyRecoverStatus(pce)

	if format == "json" {
		result := map[string]any{
			"status":          status,
			"object_url":      pce.ObjectURL,
			"package":         pce.Package,
			"transport":       pce.Transport,
			"cleanup_actions": pce.CleanupActions,
		}
		if len(pce.ManualSteps) > 0 {
			result["manual_steps"] = pce.ManualSteps
		}
		if pce.OriginalErr != nil {
			result["last_error"] = pce.OriginalErr.Error()
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return exitCodeForStatus(status)
	}

	fmt.Printf("Status:     %s\n", status)
	if pce.ObjectURL != "" {
		fmt.Printf("Object URL: %s\n", pce.ObjectURL)
	}
	if pce.Package != "" {
		fmt.Printf("Package:    %s\n", pce.Package)
	}
	if pce.Transport != "" {
		fmt.Printf("Transport:  %s\n", pce.Transport)
	}
	if len(pce.CleanupActions) > 0 {
		fmt.Println("Cleanup actions attempted:")
		for _, a := range pce.CleanupActions {
			fmt.Printf("  - %s\n", a)
		}
	}
	if len(pce.ManualSteps) > 0 {
		fmt.Println("Manual recovery steps required:")
		for _, s := range pce.ManualSteps {
			fmt.Printf("  ! %s\n", s)
		}
	}
	if pce.OriginalErr != nil {
		fmt.Printf("Last error: %v\n", pce.OriginalErr)
	}

	return exitCodeForStatus(status)
}

// normalizeCreatableType accepts either the short-form category name
// a user would type ("CLAS", "PROG", "FUGR") or the already-canonical
// TYPE/SUBTYPE form ("CLAS/OC") that pkg/adt exposes internally, and
// returns the canonical form suitable for adt.CreatableObjectType.
// Returns an empty string for anything unrecognised so the caller can
// emit a proper error.
func normalizeCreatableType(raw string) adt.CreatableObjectType {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if strings.Contains(s, "/") {
		// Already canonical — trust the caller.
		return adt.CreatableObjectType(s)
	}
	switch s {
	case "CLAS":
		return adt.ObjectTypeClass
	case "PROG":
		return adt.ObjectTypeProgram
	case "INCL", "INCLUDE":
		return adt.ObjectTypeInclude
	case "INTF":
		return adt.ObjectTypeInterface
	case "FUGR":
		return adt.ObjectTypeFunctionGroup
	case "FUNC":
		return adt.ObjectTypeFunctionMod
	case "TABL":
		return adt.ObjectTypeTable
	case "DEVC", "PACKAGE":
		return adt.ObjectTypePackage
	case "DDLS":
		return adt.ObjectTypeDDLS
	case "BDEF":
		return adt.ObjectTypeBDEF
	case "SRVD":
		return adt.ObjectTypeSRVD
	case "SRVB":
		return adt.ObjectTypeSRVB
	}
	return ""
}

// classifyRecoverStatus folds the PartialCreateError shape into a
// short status string — same classification MCP's handler produces.
func classifyRecoverStatus(pce *adt.PartialCreateError) string {
	switch {
	case pce == nil:
		return "unknown"
	case pce.CleanupOK && len(pce.CleanupActions) == 1 &&
		strings.Contains(pce.CleanupActions[0], "nothing to recover"):
		return "already_clean"
	case pce.CleanupOK:
		return "cleaned"
	case pce.OriginalErr != nil && strings.Contains(pce.OriginalErr.Error(), "existence probe failed"):
		return "probe_failed"
	default:
		return "partial"
	}
}

// exitCodeForStatus returns non-nil only when the recovery could not
// finish — "cleaned" / "already_clean" exit 0 so the command is safe
// to chain into a pipeline; "partial" / "probe_failed" return an
// error so CI and shell scripts notice.
func exitCodeForStatus(status string) error {
	switch status {
	case "cleaned", "already_clean":
		return nil
	case "partial":
		return errors.New("cleanup could not finish — see manual recovery steps above")
	case "probe_failed":
		return errors.New("existence probe inconclusive — retry after checking network / auth")
	default:
		return fmt.Errorf("unexpected recovery status: %s", status)
	}
}
