package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

var textsCmd = &cobra.Command{
	Use:   "texts",
	Short: "A program's text pool: selection texts, text symbols, headings — read, set, or sync from the source",
	Long: `The text pool of a program over ADT: selection texts (S), text symbols (I)
and list headings (H).

  vsp texts get ZDEMO_RUN                          # every entry, with its kind
  vsp texts set ZDEMO_RUN P_DEVC="Package to scan" S_OBJ="Object names"
  vsp texts set ZDEMO_RUN --kind I 001="Nothing found"
  vsp texts sync ZDEMO_RUN                         # from "~t: comments in the source
  vsp texts sync ZDEMO_RUN --dry-run

sync reads the source and takes every trailing comment of the form "~t: on a
PARAMETERS, SELECT-OPTIONS or named SELECTION-SCREEN COMMENT line as that
field's selection text:

  PARAMETERS p_devc TYPE tadir-devclass DEFAULT '$TMP'. "~t: Package to scan
  SELECT-OPTIONS s_obj FOR tadir-obj_name.            "~t: Object names

The text lives beside the field, versioned with the code; the text pool is
written from it, in the logon language unless --lang says otherwise.`,
}

var textsGetCmd = &cobra.Command{
	Use:   "get <PROGRAM>",
	Short: "Read the text pool",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, lang, err := docsClient(cmd)
		if err != nil {
			return err
		}
		entries, err := client.GetTextPoolInLanguage(context.Background(), args[0], lang)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(entries)
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "no texts")
			return nil
		}
		for _, e := range entries {
			fmt.Printf("%s  %-16s %s\n", e.ID, e.Key, e.Text)
		}
		return nil
	},
}

var textsSetCmd = &cobra.Command{
	Use:   "set <PROGRAM> KEY=TEXT [KEY=TEXT ...]",
	Short: "Write texts; keys not named keep theirs",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := map[string]string{}
		for _, kv := range args[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || strings.TrimSpace(k) == "" {
				return fmt.Errorf("%q: want KEY=TEXT", kv)
			}
			entries[strings.ToUpper(strings.TrimSpace(k))] = v
		}
		return writeTexts(cmd, args[0], entries)
	},
}

var textsSyncCmd = &cobra.Command{
	Use:   "sync <PROGRAM>",
	Short: "Write the selection texts found as \"~t: comments in the source",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		ctx := context.Background()
		source, err := client.GetSource(ctx, "PROG", strings.ToUpper(args[0]), nil)
		if err != nil {
			return err
		}
		entries := adt.SelectionTextsFromSource(source)
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "no \"~t: comments in the source; nothing to write")
			return nil
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			keys := make([]string, 0, len(entries))
			for k := range entries {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%-16s %s\n", k, entries[k])
			}
			fmt.Fprintf(os.Stderr, "%d selection text(s) found; not written (--dry-run)\n", len(entries))
			return nil
		}
		return writeTexts(cmd, args[0], entries)
	},
}

func writeTexts(cmd *cobra.Command, program string, entries map[string]string) error {
	client, lang, err := docsClient(cmd)
	if err != nil {
		return err
	}
	kind, _ := cmd.Flags().GetString("kind")
	transport, _ := cmd.Flags().GetString("transport")
	if err := client.WriteTextPool(context.Background(), program, lang, kind, entries, transport); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d text(s) written to %s (%s, %s)\n", len(entries), strings.ToUpper(program), adt.TextPoolKinds[strings.ToUpper(kind)], strings.ToUpper(lang))
	return nil
}

func init() {
	for _, c := range []*cobra.Command{textsGetCmd, textsSetCmd, textsSyncCmd} {
		c.Flags().String("lang", "", "Language (ISO code); the system's logon language by default")
	}
	textsGetCmd.Flags().Bool("json", false, "Emit JSON")
	for _, c := range []*cobra.Command{textsSetCmd, textsSyncCmd} {
		c.Flags().String("kind", "S", "Text kind: S selection texts, I text symbols, H headings")
		c.Flags().String("transport", "", "Transport request for a transportable program")
	}
	textsSyncCmd.Flags().Bool("dry-run", false, "Show what would be written")
	textsCmd.AddCommand(textsGetCmd, textsSetCmd, textsSyncCmd)
	rootCmd.AddCommand(textsCmd)
}
