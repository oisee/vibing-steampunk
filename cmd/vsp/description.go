package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var descriptionCmd = &cobra.Command{
	Use:   "description [PROG|CLAS|INTF|FUGR|FUNC|INCL|TABL|DDLS] <NAME> [TEXT]",
	Short: "An object's short text — the title SE38 shows — read, or set without touching the source",
	Long: `The description is the short text of the object: SE38's title, the header a
list prints, TRDIRT for a program. Creating over ADT sets it once; this
changes it later, on its own lock, and writes nothing when it is already so.

  vsp description ZDEMO_XFER
  vsp description ZDEMO_XFER "DPL snapshot transfer: download / upload / transplant"
  vsp description CLAS ZCL_DEMO "Demo class"
  vsp description FUNC ZDEMO_FM --parent ZDEMO_FG "Calculates tax"`,
	Args: cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		objectType, name, text := "PROG", "", ""
		switch {
		case len(args) == 3:
			objectType, name, text = args[0], args[1], args[2]
		case len(args) == 2 && isObjectTypeToken(args[0]):
			objectType, name = args[0], args[1]
		case len(args) == 2:
			name, text = args[0], args[1]
		default:
			name = args[0]
		}
		client, _, err := docsClient(cmd)
		if err != nil {
			return err
		}
		parent, _ := cmd.Flags().GetString("parent")
		asJSON, _ := cmd.Flags().GetBool("json")
		if text == "" {
			res, gerr := client.GetDescription(context.Background(), objectType, name, parent)
			if gerr != nil {
				return gerr
			}
			if asJSON {
				return printJSON(res)
			}
			fmt.Println(res.Old)
			return nil
		}
		transport, _ := cmd.Flags().GetString("transport")
		res, err := client.SetDescription(context.Background(), objectType, name, parent, text, transport)
		if asJSON && res != nil {
			if perr := printJSON(res); perr != nil {
				return perr
			}
			return err
		}
		if err != nil {
			return err
		}
		if !res.Changed {
			fmt.Fprintf(os.Stderr, "already %q\n", res.Old)
			return nil
		}
		fmt.Fprintf(os.Stderr, "%s: %q -> %q\n", res.ObjectURL, res.Old, res.New)
		return nil
	},
}

func isObjectTypeToken(s string) bool {
	switch strings.ToUpper(s) {
	case "PROG", "CLAS", "INTF", "FUGR", "FUNC", "INCL", "TABL", "DDLS":
		return true
	}
	return false
}

func init() {
	descriptionCmd.Flags().String("parent", "", "Function group of a function module")
	descriptionCmd.Flags().String("transport", "", "Transport request; the one the object is locked in is reused when empty")
	descriptionCmd.Flags().Bool("json", false, "Emit JSON")
	rootCmd.AddCommand(descriptionCmd)
}
