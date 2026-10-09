package vsp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	landscapeCmd.AddCommand(landscapeListCmd, landscapeImportCmd, landscapeScanCmd)
	landscapeCmd.PersistentFlags().String("file", "", "Landscape file to read (default: SAP GUI's own, found automatically)")
	landscapeCmd.PersistentFlags().Bool("all", false, "Read every landscape this machine can reach, VMs included (see 'landscape scan')")
	landscapeCmd.PersistentFlags().Bool("include", true, "Follow <Include> entries, which is where a shared company landscape lives")
	landscapeCmd.PersistentFlags().StringSlice("domain", nil, "DNS domains to qualify short host names with (default: detected)")
	landscapeListCmd.Flags().String("filter", "", "Only systems whose ID or name contains this")
	landscapeListCmd.Flags().Bool("probe", false, "Try each candidate address and report which one answers")
	landscapeListCmd.Flags().Bool("resolve", false, "Resolve short host names to the name a certificate will match (implied by --probe)")
	landscapeImportCmd.Flags().String("client", "", "SAP client for the imported systems")
	landscapeImportCmd.Flags().Bool("sso", true, "Mark imported systems as browser single sign-on")
	landscapeImportCmd.Flags().Bool("write", false, "Actually write .vsp.json (without this, print what would be added)")
	rootCmd.AddCommand(landscapeCmd)
}

var landscapeCmd = &cobra.Command{
	Use:   "landscape",
	Short: "Read the systems SAP GUI already knows about",
	Long: `Read SAPUILandscape.xml — the list of systems SAP Logon shows — and turn it
into vsp configuration.

The file holds what a system is called, where its message server or application
server lives, and which SAProuter and SNC identity it uses. It says nothing about
HTTP, but the instance number is derivable from the ports it does give, and SAP's
port convention turns that into candidate ADT addresses. They are candidates: a
system behind a web dispatcher answers somewhere else entirely, which is what
--probe is for.

Under WSL the file is looked for on the Windows side, because that is where SAP
GUI wrote it.`,
}

var landscapeScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Find every landscape this machine can reach, including inside VMs",
	Long: `List the landscape files SAP GUI wrote on this machine, and the ones inside
running virtual machines, with how many systems each holds.

A Mac with a Windows VM is the same situation as WSL: SAP Logon runs in the
guest, so the file that carries the company's shared <Include> is in there and
not out here. Reaching into a guest is never automatic — this reports what is
reachable, and the reference it prints is what --file takes:

  vsp landscape scan
  vsp landscape list --file "parallels:Windows 11:C:\Users\you\AppData\Roaming\SAP\Common\SAPUILandscape.xml"

Nothing is read from a guest that is not running, and nothing is written.`,
	RunE: runLandscapeScan,
}

func runLandscapeScan(cmd *cobra.Command, args []string) error {
	sources := adt.ScanLandscapeSources(cmd.Context())
	if len(sources) == 0 {
		fmt.Println("No landscape found. SAP GUI writes one when a system is configured;")
		fmt.Println("pass --file, or set SAPLOGON_LSXML_FILE, if yours lives somewhere else.")
		return nil
	}

	fmt.Printf("%-16s %8s %9s  %s\n", "KIND", "SYSTEMS", "INCLUDES", "REFERENCE")
	fmt.Println(strings.Repeat("-", 110))
	for _, s := range sources {
		if s.Err != "" {
			fmt.Printf("%-16s %8s %9s  %s\n", s.Kind, "-", "-", s.Ref)
			fmt.Printf("%-16s %8s %9s  ! %s\n", "", "", "", s.Err)
			continue
		}
		fmt.Printf("%-16s %8d %9d  %s\n", s.Kind, s.Systems, s.Includes, s.Ref)
	}
	fmt.Println()
	fmt.Println("Includes are counted, not followed — 'list' follows them.")
	return nil
}

var landscapeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the systems in the landscape file",
	RunE:  runLandscapeList,
}

var landscapeImportCmd = &cobra.Command{
	Use:   "import [system-id...]",
	Short: "Turn landscape entries into .vsp.json systems",
	Long: `Add systems from the landscape file to .vsp.json.

Name the system IDs to import, or pass none to import everything the filter
leaves. Nothing is written without --write: the default prints what would change,
because a landscape can hold a hundred systems and few of them are yours.`,
	RunE: runLandscapeImport,
}

// loadLandscape reads the landscape file and any includes it names.
//
// The second return is the sources it could not read or could not parse. They
// have always been noted on stderr, which is right, but the callers went on to
// print "No systems matched" — and the source that failed is very often the
// shared company landscape, which is where the system being looked for lives.
// An empty list is only an answer when the whole landscape was read.
func loadLandscape(cmd *cobra.Command) ([]adt.LandscapeSystem, []adt.Unsearched, error) {
	explicit, _ := cmd.Flags().GetString("file")
	follow, _ := cmd.Flags().GetBool("include")

	all, _ := cmd.Flags().GetBool("all")

	ctx := cmd.Context()
	var paths []string
	var missed []adt.Unsearched
	switch {
	case explicit != "":
		paths = []string{explicit}
	case all:
		for _, src := range adt.ScanLandscapeSources(ctx) {
			if src.Err != "" {
				// --all means "everything this machine can reach", and a source
				// dropped here never reaches the walk below, so nothing later
				// knows it existed.
				missed = append(missed, adt.Unsearched{Object: src.Ref, Reason: src.Err})
				continue
			}
			paths = append(paths, src.Ref)
		}
	default:
		paths = adt.FindLandscapeFiles(ctx, "")
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("no landscape file found — pass --file, try --all, or set SAPLOGON_LSXML_FILE")
	}

	// A landscape file names includes, and an included file may name more, so
	// this walks rather than reads. Each source is visited once: a shared
	// landscape that includes a file which includes it back would otherwise
	// spin forever.
	type source struct {
		name string
		// vm is the guest a source came from, so its includes are resolved
		// where they are reachable: a company landscape included by UNC path
		// can be opened from inside that VM and nowhere else.
		vm   string
		read func() ([]byte, error)
	}
	queue := make([]source, 0, len(paths))
	for _, p := range paths {
		p := p
		vm, _, _ := adt.ParseParallelsRef(p)
		queue = append(queue, source{name: p, vm: vm, read: func() ([]byte, error) { return adt.ReadLandscapeSource(ctx, p) }})
	}

	seen := map[string]bool{}
	// The cached copy of the shared landscape is the same bytes as the include
	// that names it. Reading both would list every shared system twice, so a
	// source already seen by content is skipped — which also makes the network
	// read unnecessary rather than merely redundant.
	seenContent := map[[32]byte]bool{}
	var systems []adt.LandscapeSystem
	read := 0

	for len(queue) > 0 {
		src := queue[0]
		queue = queue[1:]
		if seen[src.name] {
			continue
		}
		seen[src.name] = true

		blob, err := src.read()
		if err != nil {
			// One unreachable source must not lose the systems already found:
			// the shared landscape sits on a company share that is not always
			// mounted, and the local file alone is still worth having.
			fmt.Fprintf(os.Stderr, "note: %v\n", err)
			missed = append(missed, adt.Unsearched{Object: src.name, Reason: err.Error()})
			continue
		}
		if digest := sha256.Sum256(blob); seenContent[digest] {
			continue
		} else {
			seenContent[digest] = true
		}

		lf, err := adt.ParseLandscapeBytes(blob, src.name)
		if err != nil {
			// Local parsing, not a remote call — but the file still holds
			// systems that are now missing from the list, and a UNC read that
			// returns a logon page instead of XML lands here too.
			fmt.Fprintf(os.Stderr, "note: %v\n", err)
			missed = append(missed, adt.Unsearched{Object: src.name, Reason: err.Error()})
			continue
		}
		read++
		systems = append(systems, lf.Systems(src.name)...)

		if !follow {
			continue
		}
		for _, inc := range lf.Includes {
			inc := inc
			if src.vm != "" {
				if winPath, ok := adt.WindowsPathFromIncludeURL(inc.URL); ok {
					vm := src.vm
					queue = append(queue, source{
						name: adt.ParallelsRef(vm, winPath),
						vm:   vm,
						read: func() ([]byte, error) { return adt.ReadParallelsFile(ctx, vm, winPath) },
					})
					continue
				}
			}
			queue = append(queue, source{
				name: inc.URL,
				read: func() ([]byte, error) { return adt.ReadLandscapeInclude(ctx, inc.URL) },
			})
		}
	}
	if note := adt.UnsearchedNote(missed, read+len(missed), "landscape file"); note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	return dedupeSystems(systems), missed, nil
}

// dedupeSystems drops repeats. Reading everything at once means the same system
// arrives from a local file and from the shared landscape it includes; the
// first occurrence wins, so a local entry keeps its own name.
func dedupeSystems(systems []adt.LandscapeSystem) []adt.LandscapeSystem {
	seen := make(map[string]bool, len(systems))
	out := systems[:0]
	for _, s := range systems {
		key := s.SystemID + "|" + s.Host + "|" + s.InstanceNr
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// filterSystems narrows the list by a substring and by explicit system IDs.
func filterSystems(systems []adt.LandscapeSystem, filter string, ids []string) []adt.LandscapeSystem {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[strings.ToUpper(id)] = true
	}
	filter = strings.ToLower(filter)

	out := systems[:0:0]
	for _, s := range systems {
		if len(wanted) > 0 && !wanted[s.SystemID] {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(s.SystemID), filter) &&
			!strings.Contains(strings.ToLower(s.Name), filter) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// searchDomains returns the domains to qualify short host names with: whatever
// the caller named, otherwise whatever this machine can work out.
func searchDomains(cmd *cobra.Command) []string {
	if explicit, _ := cmd.Flags().GetStringSlice("domain"); len(explicit) > 0 {
		return explicit
	}
	return adt.DNSSearchDomains(cmd.Context())
}

func runLandscapeList(cmd *cobra.Command, args []string) error {
	systems, missed, err := loadLandscape(cmd)
	if err != nil {
		return err
	}
	filter, _ := cmd.Flags().GetString("filter")
	systems = filterSystems(systems, filter, args)
	if len(systems) == 0 {
		if len(missed) > 0 {
			// The shared landscape is the one that fails to open, and it is
			// also the one holding the system being looked for. "No systems
			// matched" over an unread file sends the reader to check their
			// spelling.
			fmt.Printf("No systems matched — but %d landscape file(s) could not be read (see above).\n", len(missed))
			return nil
		}
		fmt.Println("No systems matched.")
		return nil
	}

	probe, _ := cmd.Flags().GetBool("probe")
	domains := searchDomains(cmd)

	// Resolving is a DNS lookup per system, and the ones that fail are the slow
	// ones — on a landscape of a hundred and sixty that turns a listing into a
	// wait. It earns its cost when the address is about to be used, so it is
	// done for --probe and on request, and skipped for a plain look.
	resolve, _ := cmd.Flags().GetBool("resolve")
	if probe || resolve {
		adt.CanonicalHosts(cmd.Context(), systems, domains)
	}
	fmt.Printf("%-6s %-34s %-28s %-4s %s\n", "SID", "NAME", "HOST", "NR", "ADT")
	fmt.Println(strings.Repeat("-", 110))
	for _, s := range systems {
		adtCol := "-"
		if candidates := s.CandidateURLs(domains...); len(candidates) > 0 {
			adtCol = candidates[0]
			if probe {
				if live := probeADT(cmd.Context(), candidates); live != "" {
					adtCol = live + "  (отвечает)"
				} else {
					adtCol = "не отвечает ни по одному из " + fmt.Sprint(len(candidates))
				}
			}
		}
		fmt.Printf("%-6s %-34s %-28s %-4s %s\n",
			s.SystemID, truncate(s.Name, 34), truncate(s.Host, 28), s.InstanceNr, adtCol)
	}
	if !probe {
		fmt.Println("\nAddresses are candidates: the default one first, then those derived")
		fmt.Println("from the instance number. Add --probe to see which one answers, or")
		fmt.Println("--resolve to expand short host names without connecting.")
	}
	return nil
}

// probeADT returns the first candidate whose ADT node answers at all.
//
// Any answer counts, including a redirect to an identity provider or a 401: the
// question here is which address is an ABAP system, not whether this user is
// signed in to it.
func probeADT(ctx context.Context, candidates []string) string {
	client := &http.Client{Timeout: 6 * time.Second}
	for _, base := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, base+"/sap/bc/adt/discovery", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 500 {
			return base
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func runLandscapeImport(cmd *cobra.Command, args []string) error {
	systems, missed, err := loadLandscape(cmd)
	if err != nil {
		return err
	}
	filter, _ := cmd.Flags().GetString("filter")
	systems = filterSystems(systems, filter, args)
	if len(systems) == 0 {
		if len(missed) > 0 {
			return fmt.Errorf("no systems matched, and %d landscape file(s) could not be read — the one you want may be in them", len(missed))
		}
		return fmt.Errorf("no systems matched — nothing to import")
	}

	domains := searchDomains(cmd)
	adt.CanonicalHosts(cmd.Context(), systems, domains)
	clientNr, _ := cmd.Flags().GetString("client")
	useSSO, _ := cmd.Flags().GetBool("sso")
	write, _ := cmd.Flags().GetBool("write")

	cfg, path, err := config.LoadSystems()
	if err != nil {
		return fmt.Errorf("loading systems config: %w", err)
	}
	if cfg == nil {
		cfg = &config.SystemsConfig{Systems: map[string]config.SystemConfig{}}
		path = ".vsp.json"
	}
	if cfg.Systems == nil {
		cfg.Systems = map[string]config.SystemConfig{}
	}

	added, skipped := make([]string, 0, len(systems)), make([]string, 0)
	for _, s := range systems {
		name := strings.ToLower(s.SystemID)
		if _, exists := cfg.Systems[name]; exists {
			// A system already configured has been tuned by hand — a derived
			// address is not a good enough reason to overwrite that.
			skipped = append(skipped, name)
			continue
		}
		candidates := s.CandidateURLs(domains...)
		if len(candidates) == 0 {
			skipped = append(skipped, name)
			continue
		}
		entry := config.SystemConfig{
			URL:      candidates[0],
			Client:   clientNr,
			Language: "EN",
		}
		if useSSO {
			entry.Auth = "sso"
		}
		cfg.Systems[name] = entry
		added = append(added, name)
	}

	sort.Strings(added)
	sort.Strings(skipped)
	for _, name := range added {
		fmt.Printf("  + %-8s %s\n", name, cfg.Systems[name].URL)
	}
	if len(skipped) > 0 {
		fmt.Printf("\n  already configured, left alone: %s\n", strings.Join(skipped, ", "))
	}
	if len(added) == 0 {
		fmt.Println("Nothing to add.")
		return nil
	}

	if !write {
		fmt.Printf("\n%d system(s) would be added to %s. Re-run with --write.\n", len(added), path)
		fmt.Println("The addresses are derived — check them with `vsp landscape list --probe` first.")
		return nil
	}
	if err := cfg.SaveToFile(path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("\nWrote %d system(s) to %s.\n", len(added), path)
	return nil
}
