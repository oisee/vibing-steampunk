package vsp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

// --- transport command ---

var transportCmd = &cobra.Command{
	Use:   "transport",
	Short: "Transport management",
	Long: `Manage CTS transport requests.

Examples:
  vsp transport list
  vsp transport get A4HK900094`,
}

var transportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List transport requests",
	Long: `List transport requests of a user: workbench and customizing, modifiable
and released by default.

The listing is read from GET /sap/bc/adt/cts/transportrequests with explicit
requestType and requestStatus (without them the organizer answers with
released requests only), falling back to the saved Transport Organizer
search configuration and then to the E070/E07T tables. The source that
answered is reported on stderr.

Examples:
  vsp transport list
  vsp transport list --user DEVELOPER
  vsp transport list --status D                      # modifiable only
  vsp transport list --source config                 # as Eclipse: saved search configuration
  vsp transport list --status R --released-from 20260101 --released-to 20261231`,
	RunE: runTransportList,
}

var transportGetCmd = &cobra.Command{
	Use:   "get <number>",
	Short: "Get transport details",
	Long: `Get detailed information about a transport request.

Examples:
  vsp transport get A4HK900094`,
	Args: cobra.ExactArgs(1),
	RunE: runTransportGet,
}

func runTransportList(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	user, _ := cmd.Flags().GetString("user")
	requestType, _ := cmd.Flags().GetString("type")
	requestStatus, _ := cmd.Flags().GetString("status")
	releasedFrom, _ := cmd.Flags().GetString("released-from")
	releasedTo, _ := cmd.Flags().GetString("released-to")
	source, _ := cmd.Flags().GetString("source")
	configURI, _ := cmd.Flags().GetString("config-uri")
	noTargets, _ := cmd.Flags().GetBool("no-targets")

	ctx := context.Background()
	res, err := client.QueryTransports(ctx, adt.TransportQuery{
		User:            user,
		RequestTypes:    requestType,
		RequestStatuses: requestStatus,
		ReleasedFrom:    releasedFrom,
		ReleasedTo:      releasedTo,
		Source:          source,
		ConfigURI:       configURI,
		Targets:         !noTargets,
	})
	if err != nil {
		return fmt.Errorf("listing transports failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "source: %s; %s\n", res.Source, res.Query.Describe())
	if res.ConfigURI != "" {
		fmt.Fprintf(os.Stderr, "search configuration: %s\n", res.ConfigURI)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(os.Stderr, "note: %s\n", n)
	}

	transports := adt.FlattenTransports(res.Transports)
	if len(transports) == 0 {
		fmt.Println("No transport requests found.")
		return nil
	}

	fmt.Printf("%-12s %-12s %-8s %-10s %-11s %s\n", "NUMBER", "OWNER", "STATUS", "TYPE", "BUCKET", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 92))
	for _, t := range transports {
		status := t.Status
		if t.StatusText != "" {
			status = t.StatusText
		}
		fmt.Printf("%-12s %-12s %-8s %-10s %-11s %s\n", t.Number, t.Owner, status, t.Type, t.Bucket, t.Description)
	}
	fmt.Printf("\n%d transport(s)\n", len(transports))
	return nil
}

func runTransportGet(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}

	client, err := getClient(params)
	if err != nil {
		return err
	}

	number := strings.ToUpper(args[0])

	ctx := context.Background()
	details, err := client.GetTransport(ctx, number)
	if err != nil {
		return fmt.Errorf("getting transport failed: %w", err)
	}

	fmt.Printf("Transport: %s\n", details.Number)
	fmt.Printf("Owner:     %s\n", details.Owner)
	fmt.Printf("Type:      %s\n", details.Type)
	fmt.Printf("Status:    %s\n", details.StatusText)
	if details.Target != "" {
		fmt.Printf("Target:    %s\n", details.Target)
	}
	fmt.Printf("Desc:      %s\n", details.Description)

	if len(details.Tasks) > 0 {
		fmt.Printf("\nTasks (%d):\n", len(details.Tasks))
		for _, task := range details.Tasks {
			fmt.Printf("  %s  %-12s  %-8s  %s\n", task.Number, task.Owner, task.StatusText, task.Description)
			for _, obj := range task.Objects {
				fmt.Printf("    %s %s %s\n", obj.PgmID, obj.Type, obj.Name)
			}
		}
	}

	if len(details.Objects) > 0 {
		fmt.Printf("\nObjects (%d):\n", len(details.Objects))
		for _, obj := range details.Objects {
			fmt.Printf("  %s %s %s\n", obj.PgmID, obj.Type, obj.Name)
		}
	}

	return nil
}

func init() {
	// Transport list flags
	transportListCmd.Flags().String("user", "", "Filter by user (default: current user, '*' for every user — source sql only)")
	transportListCmd.Flags().String("type", "", "Request types: letters of K (workbench), W (customizing), T (transport of copies); default KWT")
	transportListCmd.Flags().String("status", "", "Request statuses: letters of D (modifiable), R (released); default DR")
	transportListCmd.Flags().String("released-from", "", "YYYYMMDD; with --released-to bounds the released requests (default: last 14 days)")
	transportListCmd.Flags().String("released-to", "", "YYYYMMDD; see --released-from")
	transportListCmd.Flags().String("source", "", "auto (default), params, config or sql; see 'vsp transport list --help'")
	transportListCmd.Flags().String("config-uri", "", "Search configuration for --source config (default: the one saved for the user)")
	transportListCmd.Flags().Bool("no-targets", false, "Do not group by transport target and CTS project")

	rootCmd.AddCommand(transportCmd)

	// Transport subcommands
	transportCmd.AddCommand(transportListCmd)
	transportCmd.AddCommand(transportGetCmd)
}
