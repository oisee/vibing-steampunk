package vsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var w3miCmd = &cobra.Command{
	Use:   "w3mi",
	Short: "Read binary objects out of the MIME repository (SMW0) — images, audio, any uploaded file",
	Long: `Read the MIME repository that SMW0 maintains and abapGit serialises as W3MI.

The bytes are not in a column. SMW0 writes them into WWWDATA as an INDX-style
data cluster, LZH-compressed and split over as many rows as it takes, and inside
that cluster they are a table of fixed 255-byte lines whose last line is padded
with zeroes. The cluster alone therefore cannot say where the file ends — the
true length is the filesize parameter in WWWPARAMS, and truncating to it is what
turns the lines back into a file.

Both halves are plain table reads, so this needs nothing installed on the system:
no abapGit, no RFC, no ZADT_VSP.

Examples:
  vsp w3mi list                              # every MIME object
  vsp w3mi list 'ZORK%'                      # SQL LIKE on the object id
  vsp w3mi get ZORK-MINI.Z3 --out game.z3
  vsp w3mi get ZDEMO_LOGO.PNG --abapgit-dir src/   # the abapGit pair`,
}

var w3miListCmd = &cobra.Command{
	Use:   "list [LIKE-PATTERN]",
	Short: "List MIME repository objects with their size and type",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		pattern := ""
		if len(args) == 1 {
			pattern = args[0]
		}
		top, _ := cmd.Flags().GetInt("top")

		objects, err := client.ListW3MI(context.Background(), pattern, top)
		if err != nil {
			return err
		}
		sort.Slice(objects, func(i, j int) bool { return objects[i].Name < objects[j].Name })

		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(objects)
		}
		if len(objects) == 0 {
			fmt.Println("no MIME objects matched")
			return nil
		}
		fmt.Printf("%-40s %10s  %-28s %s\n", "OBJID", "SIZE", "MIMETYPE", "VER")
		fmt.Println(strings.Repeat("-", 90))
		for _, o := range objects {
			fmt.Printf("%-40s %10d  %-28s %s\n", o.Name, o.Size, o.MimeType, o.Version)
		}
		fmt.Printf("\n%d object(s)\n", len(objects))
		return nil
	},
}

var w3miGetCmd = &cobra.Command{
	Use:   "get <OBJID>",
	Short: "Read one MIME object and write the file",
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

		obj, err := client.GetW3MI(context.Background(), args[0])
		if err != nil {
			return err
		}

		out, _ := cmd.Flags().GetString("out")
		abapgitDir, _ := cmd.Flags().GetString("abapgit-dir")
		if out == "" && abapgitDir == "" {
			return fmt.Errorf("give --out <file> or --abapgit-dir <dir>; refusing to write binary to the terminal")
		}

		if out != "" {
			if err := os.WriteFile(out, obj.Data, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", out, err)
			}
			fmt.Fprintf(os.Stderr, "%s: %d bytes -> %s (%d lines of %d, %d bytes of padding dropped)\n",
				obj.Name, obj.Size, out, obj.Lines, 255, obj.Padding)
		}

		if abapgitDir != "" {
			dataPath, xmlPath, err := writeAbapGitW3MI(abapgitDir, obj.Name, obj.Extension, obj.MimeType, obj.Version, obj.Size, obj.Data)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s: %d bytes -> %s + %s\n", obj.Name, obj.Size, dataPath, xmlPath)
			fmt.Fprintf(os.Stderr, "note: the WWWPARAMS `filename` parameter is deliberately NOT written — "+
				"it holds the uploader's workstation path, which is a live identifier.\n")
		}
		return nil
	},
}

// writeAbapGitW3MI writes the pair abapGit expects: the bytes as
// <name>.w3mi.data.<ext> and the metadata as <name>.w3mi.xml, with the object
// id percent-escaped the way abapGit escapes it in a filename.
//
// The `filename` WWWPARAMS entry is left out on purpose. It records the path the
// file was uploaded from — a user name, a host, a directory layout — and these
// files are meant to be committable.
func writeAbapGitW3MI(dir, objID, ext, mimeType, version string, size int, data []byte) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", dir, err)
	}

	base := abapGitEscapeName(objID)
	suffix := strings.TrimPrefix(strings.ToLower(ext), ".")
	if suffix == "" {
		suffix = "bin"
	}
	dataPath := filepath.Join(dir, base+".w3mi.data."+suffix)
	xmlPath := filepath.Join(dir, base+".w3mi.xml")

	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", dataPath, err)
	}

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<abapGit version=\"v1.0.0\" serializer=\"LCL_OBJECT_W3MI\" serializer_version=\"v1.0.0\">\n")
	b.WriteString(" <asx:abap xmlns:asx=\"http://www.sap.com/abapxml\" version=\"1.0\">\n")
	b.WriteString("  <asx:values>\n")
	fmt.Fprintf(&b, "   <NAME>%s</NAME>\n", xmlEscape(objID))
	b.WriteString("   <PARAMS>\n")
	for _, p := range []struct{ name, value string }{
		{"fileextension", ext},
		{"filesize", fmt.Sprint(size)},
		{"mimetype", mimeType},
		{"version", version},
	} {
		if p.value == "" {
			continue
		}
		fmt.Fprintf(&b, "    <item><NAME>%s</NAME><VALUE>%s</VALUE></item>\n",
			p.name, xmlEscape(p.value))
	}
	b.WriteString("   </PARAMS>\n  </asx:values>\n </asx:abap>\n</abapGit>\n")

	if err := os.WriteFile(xmlPath, []byte(b.String()), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", xmlPath, err)
	}
	return dataPath, xmlPath, nil
}

// abapGitEscapeName lowercases the object id and percent-escapes the characters
// abapGit escapes in a filename, so "ZORK-MINI.Z3" becomes "zork-mini%2ez3".
func abapGitEscapeName(objID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(objID) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%02x", r)
		}
	}
	return b.String()
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

func init() {
	w3miListCmd.Flags().Int("top", 1000, "Maximum WWWPARAMS rows to read")
	w3miListCmd.Flags().Bool("json", false, "Emit JSON")
	w3miGetCmd.Flags().String("out", "", "Write the file here")
	w3miGetCmd.Flags().String("abapgit-dir", "", "Write the abapGit pair (.w3mi.data.* and .w3mi.xml) into this directory")
	w3miCmd.AddCommand(w3miListCmd, w3miGetCmd)
	rootCmd.AddCommand(w3miCmd)
}
