package vsp

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The two organizer operations ADT does not have — merging requests and
// moving an entry between them — go through ZADT_VSP's function bridge,
// so both need a WebSocket to the system, built the way the debugger's is.

var transportMergeCmd = &cobra.Command{
	Use:   "merge <SOURCE> [SOURCE ...] --into <TARGET>",
	Short: "Merge requests into one, the way SE09's Merge Requests does (needs ZADT_VSP)",
	Long: `Merge one or more modifiable requests into a target: their tasks and
objects go into the target and the sources are deleted. Both sides must be
yours and of the same kind. This is TR_MERGE_REQUESTS with its dialog off,
reached through ZADT_VSP's function bridge, since ADT has no such resource.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport merge TR-A TR-B --into TR-C`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		into, _ := cmd.Flags().GetString("into")
		if into == "" {
			return fmt.Errorf("--into <TARGET> is required")
		}
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		asJSON, _ := cmd.Flags().GetBool("json")
		var results []*adt.TransportMergeResult
		for _, src := range args {
			res, merr := client.MergeTransports(context.Background(), ws, src, into)
			if res != nil {
				results = append(results, res)
			}
			if merr != nil {
				if asJSON {
					_ = printJSON(results)
				}
				return merr
			}
			if !asJSON {
				fmt.Fprintf(os.Stderr, "%s merged into %s (%d object(s) there now", res.From, res.To, len(res.Objects))
				if res.SourceGone {
					fmt.Fprint(os.Stderr, "; the source is gone")
				}
				fmt.Fprintln(os.Stderr, ")")
			}
		}
		if asJSON {
			return printJSON(results)
		}
		return nil
	},
}

var transportTocCmd = &cobra.Command{
	Use:   "toc <REQUEST> --target <SYSTEM|/GROUP/>",
	Short: "Copy a request into a new transport of copies, the way SE01 does (needs ZADT_VSP)",
	Long: `Create a transport of copies for a target and copy the request's object
list into it: from each task that holds objects while the request is
modifiable, from the request itself once it is released. The copy is
TR_COPY_COMM with its dialog off, reached through ZADT_VSP's function bridge;
the request is created over ADT. ADT takes a target without a client (QAS,
not QAS.100). Nothing is released unless --release is given.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport toc TR-A --target QAS
  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport toc TR-A --target /GROUP/ --release`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts adt.TransportOfCopiesOptions
		opts.Target, _ = cmd.Flags().GetString("target")
		opts.Description, _ = cmd.Flags().GetString("description")
		opts.CTSProject, _ = cmd.Flags().GetString("cts-project")
		opts.Release, _ = cmd.Flags().GetBool("release")
		if opts.Target == "" {
			return fmt.Errorf("--target <SYSTEM|/GROUP/> is required")
		}
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		res, terr := client.CopyToTransportOfCopies(context.Background(), ws, args[0], opts)
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON && res != nil {
			if perr := printJSON(res); perr != nil {
				return perr
			}
		} else if res != nil && res.Transport != "" {
			fmt.Fprintf(os.Stderr, "%s: transport of copies of %s for %s, %d object(s) from %v",
				res.Transport, res.Source, res.Target, len(res.Objects), res.CopiedFrom)
			if res.Released {
				fmt.Fprint(os.Stderr, ", released")
			}
			fmt.Fprintln(os.Stderr)
			for _, sk := range res.Skipped {
				fmt.Fprintf(os.Stderr, "  skipped %s: %s\n", sk.Entry, sk.Reason)
			}
		}
		return terr
	},
}

var transportMoveCmd = &cobra.Command{
	Use:   "move <\"TYPE NAME\"> --from <REQUEST> --to <REQUEST>",
	Short: "Move one object entry from one request to another (needs ZADT_VSP)",
	Long: `Move one entry: it is appended to your modifiable task of the target (the
request itself when you have none) and deleted from the task of the source
that holds it. The object is R3TR unless a PGMID is given.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport move "PROG ZDEMO_RUN" --from TR-A --to TR-B
  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport move "LIMU METH ZCL_DEMO RUN" --from TR-A --to TR-B`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := adt.ParseTransportObject(args[0])
		if err != nil {
			return err
		}
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		if from == "" || to == "" {
			return fmt.Errorf("--from and --to are required")
		}
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		res, err := client.MoveTransportObject(context.Background(), ws, key, from, to)
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON && res != nil {
			if perr := printJSON(res); perr != nil {
				return perr
			}
			return err
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%s: %s -> %s\n", key, res.FromTask, res.ToTask)
		return nil
	},
}

var transportAddCmd = &cobra.Command{
	Use:   "add <REQUEST> <\"TYPE NAME\"> [\"TYPE NAME\" ...]",
	Short: "Add object entries to a request, as SE09 does (needs ZADT_VSP)",
	Long: `Add entries to your modifiable task of the request (the request itself when
you have none, or the task when you name one). The object is R3TR unless a
PGMID is given. A TABU entry takes the keys of the rows it carries with
--key: the key fields end to end at their full length, '*' as a generic tail.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport add TR-A "LIMU REPT ZDEMO" "PROG ZDEMO2"
  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport add TR-A "TABU ZDEMO_CONF" --key 100KEY1 --key "100KEY2*"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		keys, _ := cmd.Flags().GetStringArray("key")
		var entries []adt.TransportEntry
		for _, a := range args[1:] {
			key, err := adt.ParseTransportObject(a)
			if err != nil {
				return err
			}
			entries = append(entries, adt.TransportEntry{TransportObjectKey: key})
		}
		if len(keys) > 0 {
			if len(entries) != 1 {
				return fmt.Errorf("--key belongs to a single TABU entry")
			}
			entries[0].Keys = keys
		}
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		res, err := client.AddTransportObjects(context.Background(), ws, args[0], entries)
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON && res != nil {
			if perr := printJSON(res); perr != nil {
				return perr
			}
			return err
		}
		if err != nil {
			return err
		}
		for _, k := range res.Added {
			fmt.Fprintf(os.Stderr, "%s -> %s\n", k, res.Task)
		}
		if res.Keys > 0 {
			fmt.Fprintf(os.Stderr, "%d table key(s)\n", res.Keys)
		}
		return nil
	},
}

var transportRemoveCmd = &cobra.Command{
	Use:   "remove <REQUEST> <\"TYPE NAME\">",
	Short: "Take one object entry out of a request, as SE09 does (needs ZADT_VSP)",
	Long: `Delete one entry, with the table keys it carries, from whichever task of
the request holds it. The object is R3TR unless a PGMID is given.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport remove TR-A "PROG ZDEMO"
  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport remove TR-A "LIMU REPT ZDEMO"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := adt.ParseTransportObject(args[1])
		if err != nil {
			return err
		}
		client, ws, closeWS, err := transportBridge(cmd)
		if err != nil {
			return err
		}
		defer closeWS()
		res, err := client.RemoveTransportObject(context.Background(), ws, args[0], key)
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON && res != nil {
			if perr := printJSON(res); perr != nil {
				return perr
			}
			return err
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%s removed from %s\n", key, res.Task)
		return nil
	},
}

// transportBridge is the ADT client and a connected WebSocket to ZADT_VSP,
// authenticated as that client is: the profile's cookie_file, cookie_string or
// single sign-on session, or its password when it has none. The global
// cfg.Cookies it used to read is never set for a CLI subcommand, so a profile
// on cookies reached ZADT_VSP with no credential at all.
func transportBridge(cmd *cobra.Command) (*adt.Client, *adt.DebugWebSocketClient, func(), error) {
	client, err := createADTClientFor(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	ws := client.NewDebugWebSocketClient()
	if err := ws.Connect(context.Background()); err != nil {
		return nil, nil, nil, fmt.Errorf("ZADT_VSP's function bridge (WebSocket) is not reachable: %w — it needs ZADT_VSP installed (vsp system install_zadt_vsp)", err)
	}
	return client, ws, func() { ws.Close() }, nil
}

func init() {
	transportMergeCmd.Flags().String("into", "", "The request the sources are merged into")
	transportMergeCmd.Flags().Bool("json", false, "Emit JSON")
	transportMoveCmd.Flags().String("from", "", "The request the entry leaves")
	transportMoveCmd.Flags().String("to", "", "The request the entry goes into")
	transportMoveCmd.Flags().Bool("json", false, "Emit JSON")
	transportTocCmd.Flags().String("target", "", "The system (QAS) or target group (/GROUP/) the copy is for")
	transportTocCmd.Flags().String("description", "", "Description; defaults to \"ToC \" and the original's")
	transportTocCmd.Flags().String("cts-project", "", "CTS project; defaults to the system's cts_project")
	transportTocCmd.Flags().Bool("release", false, "Release the transport of copies once it is filled")
	transportTocCmd.Flags().Bool("json", false, "Emit JSON")
	transportAddCmd.Flags().StringArray("key", nil, "Table key of a TABU entry (repeatable)")
	transportAddCmd.Flags().Bool("json", false, "Emit JSON")
	transportRemoveCmd.Flags().Bool("json", false, "Emit JSON")
	transportCmd.AddCommand(transportMergeCmd, transportMoveCmd, transportTocCmd, transportAddCmd, transportRemoveCmd)
}
