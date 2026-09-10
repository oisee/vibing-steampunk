package main

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

// transportBridge is the ADT client and a connected WebSocket to ZADT_VSP.
func transportBridge(cmd *cobra.Command) (*adt.Client, *adt.DebugWebSocketClient, func(), error) {
	client, err := createADTClientFor(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	ws := adt.NewDebugWebSocketClient(cfg.BaseURL, cfg.Client, cfg.Username, cfg.Password, cfg.InsecureSkipVerify)
	if len(cfg.Cookies) > 0 {
		ws.SetCookies(cfg.Cookies)
	}
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
	transportCmd.AddCommand(transportMergeCmd, transportMoveCmd)
}
