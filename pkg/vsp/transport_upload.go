package vsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Upload transport: a released request's cofile and data file into the
// connected system's DIR_TRANS, and the request into that system's import
// buffer, through ZADT_VSP's transport service. Nothing is ever imported.

var transportUploadCmd = &cobra.Command{
	Use:   "upload --cofile K<nr>.<SID> --datafile R<nr>.<SID>",
	Short: "Put a released request's files into DIR_TRANS and its import queue -- never import (needs ZADT_VSP)",
	Long: `Write a released request's cofile and data file into DIR_TRANS of the
connected system (cofiles/ and data/) and add the request to that system's
import buffer. The import itself is not done, and cannot be done, by vsp: a
human imports the request in STMS.

The files keep their names: K<6 digits>.<SID> and R<6 digits>.<SID>, the same
number and SID, both required, 50 MB together at most. A file that already
exists in DIR_TRANS is never overwritten, and a request already in the buffer
is not added again. The target is always the connected system and client.

Requires --enable-transports (SAP_ENABLE_TRANSPORTS=true); refused under
--read-only and --transport-read-only; --allowed-transports applies to the
request. Needs ZADT_VSP with ZCL_VSP_TRANSPORT_SERVICE (vsp install zadt-vsp),
and S_CTS_ADMI with EPS1 (files) and TADD (buffer) on the system.

  SAP_ENABLE_TRANSPORTS=true vsp -s qassys transport upload --cofile ./K900123.DEV --datafile ./R900123.DEV`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cofile, _ := cmd.Flags().GetString("cofile")
		datafile, _ := cmd.Flags().GetString("datafile")
		if cofile == "" || datafile == "" {
			return fmt.Errorf("both files are required: --cofile K<nr>.<SID> and --datafile R<nr>.<SID>")
		}
		// Every gate before a file is read or a connection opened. A
		// .vsp.json system's client does not see SAP_READ_ONLY, so the CLI's
		// own read-only test comes first, as for the other write commands.
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		if cliReadOnly(params) {
			return fmt.Errorf("transport write operation 'UploadTransport' is blocked: read-only mode enabled (read_only in .vsp.json, or SAP_READ_ONLY)")
		}
		client, err := createADTClientFor(cmd)
		if err != nil {
			return err
		}
		if err = client.CheckTransportUpload(""); err != nil {
			return err
		}
		request, _, _, err := adt.TransportRequestFromFileNames(filepath.Base(cofile), filepath.Base(datafile))
		if err != nil {
			return err
		}
		if err = client.CheckTransportUpload(request); err != nil {
			return err
		}
		files, err := adt.ReadTransportFiles(cofile, datafile)
		if err != nil {
			return err
		}
		ws, closeWS, err := transportServiceWS(client)
		if err != nil {
			return err
		}
		defer closeWS()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		res, uerr := client.UploadTransport(ctx, ws, files)
		asJSON, _ := cmd.Flags().GetBool("json")
		out := uploadOutput{Upload: res}
		if uerr == nil && res != nil && res.Job != nil {
			// The add runs as a background job; wait for its outcome as
			// long as asked, then say what is known -- nothing more.
			wait, _ := cmd.Flags().GetDuration("wait")
			if wait > 0 {
				wctx, cancel := context.WithTimeout(ctx, wait)
				// The job pushes its outcome to this connection when it is
				// done; if the connection drops, a new one asks for it.
				var extra []func()
				reconnect := func(context.Context) (adt.TransportService, error) {
					ws2, close2, err := transportServiceWS(client)
					if err == nil {
						extra = append(extra, close2)
					}
					return ws2, err
				}
				st, _ := client.WaitTransportAdd(wctx, ws, res.Request, res.Job.Count, reconnect)
				for _, f := range extra {
					f()
				}
				cancel()
				out.Status = st
			}
		}
		if asJSON && res != nil {
			if perr := printJSON(out); perr != nil {
				return perr
			}
		} else if res != nil {
			fmt.Fprintf(os.Stderr, "%s -> %s client %s\n", res.Request, res.System, res.Client)
			if res.FilesWritten {
				fmt.Fprintf(os.Stderr, "  wrote %s (%d bytes) and %s (%d bytes)\n", res.DataPath, res.DataSize, res.CofilePath, res.CofileSize)
			}
			if res.RolledBack {
				fmt.Fprintln(os.Stderr, "  the files this upload wrote were deleted again")
			} else if !res.FilesWritten && res.CofileState != "" {
				fmt.Fprintf(os.Stderr, "  in DIR_TRANS now: cofile %s, data file %s\n", res.CofileState, res.DataState)
			}
			if res.Job != nil {
				fmt.Fprintf(os.Stderr, "  job %s %s released\n", res.Job.Name, res.Job.Count)
			}
			if out.Status != nil {
				printAddStatus(out.Status)
			} else if res.Note != "" {
				fmt.Fprintln(os.Stderr, "  "+res.Note)
			}
		}
		if uerr != nil {
			return uerr
		}
		if out.Status != nil && out.Status.Outcome != adt.TransportQueued {
			return fmt.Errorf("%s: %s", out.Status.Outcome, out.Status.Note)
		}
		return nil
	},
}

// uploadOutput is what vsp transport upload --json prints: the upload, and
// the add's status when it was waited for.
type uploadOutput struct {
	Upload *adt.TransportUploadResult `json:"upload"`
	Status *adt.TransportAddStatus    `json:"status,omitempty"`
}

func printAddStatus(st *adt.TransportAddStatus) {
	if st.Push != nil {
		fmt.Fprintf(os.Stderr, "  pushed by the job: %s (tp rc %s) %s\n", st.Push.Outcome, st.Push.TPRC, st.Push.Message)
	}
	fmt.Fprintf(os.Stderr, "  outcome: %s", st.Outcome)
	if st.JobCount != "" {
		fmt.Fprintf(os.Stderr, " (job %s %s, status %q)", st.Job, st.JobCount, st.JobStatus)
	}
	fmt.Fprintf(os.Stderr, ", in buffer: %t\n", st.InBuffer)
	for _, l := range st.JobLog {
		fmt.Fprintln(os.Stderr, "    "+l)
	}
	if st.Note != "" {
		fmt.Fprintln(os.Stderr, "  "+st.Note)
	}
}

var transportStatusCmd = &cobra.Command{
	Use:   "status <REQUEST> [--job <NUMBER>]",
	Short: "Say whether an uploaded request reached the import queue (read-only; needs ZADT_VSP)",
	Long: `Report the outcome of vsp transport upload's add to the import queue:
queued (the buffer file DIR_TRANS/buffer/<SID> has the request and the job is
done), pending (job ZVSP_TRANSPORT_BUFFER has not finished), job_failed (the job
ended and the buffer does not have it), or unknown. Without --job, only the
buffer is read. Read-only: no tp, nothing written. Requires --enable-transports.

  SAP_ENABLE_TRANSPORTS=true vsp -s qassys transport status TR-EXAMPLE --job 12345678`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		job, _ := cmd.Flags().GetString("job")
		client, err := createADTClientFor(cmd)
		if err != nil {
			return err
		}
		if err = client.CheckTransportBufferRead(args[0], "TransportAddStatus"); err != nil {
			return err
		}
		ws, closeWS, err := transportServiceWS(client)
		if err != nil {
			return err
		}
		defer closeWS()
		st, err := client.TransportAddStatus(context.Background(), ws, args[0], job)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(st)
		}
		fmt.Fprintf(os.Stderr, "%s on %s\n", st.Request, st.System)
		printAddStatus(st)
		return nil
	},
}

var transportBufferCmd = &cobra.Command{
	Use:   "buffer [REQUEST]",
	Short: "Show the connected system's import buffer, or one request in it (needs ZADT_VSP)",
	Long: `Read the import buffer (import queue) of the connected system: the buffer
file DIR_TRANS/buffer/<SID>, read through SAP's file layer -- no tp, no job,
nothing written. Allowed under --read-only. Requires --enable-transports.

  SAP_ENABLE_TRANSPORTS=true vsp -s qassys transport buffer
  SAP_ENABLE_TRANSPORTS=true vsp -s qassys transport buffer TR-EXAMPLE`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		request := ""
		if len(args) == 1 {
			request = args[0]
		}
		client, err := createADTClientFor(cmd)
		if err != nil {
			return err
		}
		if err = client.CheckTransportBufferRead(request, "TransportBuffer"); err != nil {
			return err
		}
		ws, closeWS, err := transportServiceWS(client)
		if err != nil {
			return err
		}
		defer closeWS()
		res, err := client.TransportBuffer(context.Background(), ws, request)
		if err != nil {
			return err
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return printJSON(res)
		}
		fmt.Printf("Import buffer of %s: %d entr", res.System, res.Total)
		if res.Total == 1 {
			fmt.Println("y")
		} else {
			fmt.Println("ies")
		}
		for _, e := range res.Entries {
			fmt.Printf("  %-20s client %-3s type %-1s owner %-12s umodes %-10s rc %s\n",
				e.Request, e.Client, e.Function, e.Owner, e.UModes, e.ReturnCode)
		}
		if res.Truncated {
			fmt.Println("  ... (more not shown)")
		}
		return nil
	},
}

var transportDownloadCmd = &cobra.Command{
	Use:   "download <REQUEST> [-o DIR]",
	Short: "Copy a released request's cofile and data file out of DIR_TRANS (read-only; needs ZADT_VSP)",
	Long: `Read K<nr>.<SID> from DIR_TRANS/cofiles and R<nr>.<SID> from DIR_TRANS/data
of the connected system and write them into a local directory. Nothing on the
system changes. Existing local files are not overwritten. A data file can
carry table contents, so this is treated as a sensitive read: it requires
--enable-transports and is refused under --read-only.

  SAP_ENABLE_TRANSPORTS=true vsp -s devsys transport download TR-EXAMPLE -o ./out`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("output")
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		if cliReadOnly(params) {
			return fmt.Errorf("operation 'DownloadTransportFiles' is blocked: read-only mode enabled (read_only in .vsp.json, or SAP_READ_ONLY); a data file can carry table contents")
		}
		client, err := createADTClientFor(cmd)
		if err != nil {
			return err
		}
		if err = client.CheckTransportDownload(args[0]); err != nil {
			return err
		}
		cofileName, dataName, err := adt.TransportFileNamesForRequest(args[0])
		if err != nil {
			return err
		}
		for _, n := range []string{cofileName, dataName} {
			if _, statErr := os.Lstat(filepath.Join(dir, n)); statErr == nil {
				return fmt.Errorf("%s exists; not overwritten", filepath.Join(dir, n))
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
		}
		ws, closeWS, err := transportServiceWS(client)
		if err != nil {
			return err
		}
		defer closeWS()
		files, err := client.DownloadTransportFiles(context.Background(), ws, args[0])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, f := range []struct {
			name string
			data []byte
		}{{files.CofileName, files.Cofile}, {files.DataName, files.Data}} {
			fh, err := os.OpenFile(filepath.Join(dir, f.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			if _, err := fh.Write(f.data); err != nil {
				fh.Close()
				return err
			}
			if err := fh.Close(); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s (%d bytes)\n", filepath.Join(dir, f.name), len(f.data))
		}
		return nil
	},
}

// transportServiceWS is a WebSocket to ZADT_VSP on the connected system,
// opened only after every gate has passed. It authenticates as the
// profile's HTTP client does: the session that client holds at this moment --
// from a cookie_file, a cookie_string or single sign-on, including the last
// refresh an HTTP call made -- and the password otherwise. Building the
// WebSocket does not itself re-authenticate.
func transportServiceWS(client *adt.Client) (adt.TransportService, func(), error) {
	ws := client.NewDebugWebSocketClient()
	if err := ws.Connect(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("ZADT_VSP (WebSocket) is not reachable: %w -- it needs ZADT_VSP with ZCL_VSP_TRANSPORT_SERVICE (vsp install zadt-vsp)", err)
	}
	return ws, func() { ws.Close() }, nil
}

func init() {
	transportUploadCmd.Flags().String("cofile", "", "The cofile, K<6 digits>.<SID>")
	transportUploadCmd.Flags().String("datafile", "", "The data file, R<6 digits>.<SID>")
	transportUploadCmd.Flags().Bool("json", false, "Emit JSON")
	transportUploadCmd.Flags().Duration("wait", 60*time.Second, "How long to wait for the add's outcome (0: return once the job is released)")
	transportStatusCmd.Flags().String("job", "", "The job number vsp transport upload reported")
	transportStatusCmd.Flags().Bool("json", false, "Emit JSON")
	transportBufferCmd.Flags().Bool("json", false, "Emit JSON")
	transportDownloadCmd.Flags().StringP("output", "o", ".", "Directory to write the two files into")
	transportCmd.AddCommand(transportUploadCmd, transportStatusCmd, transportBufferCmd, transportDownloadCmd)
}
