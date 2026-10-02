package adt

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Import of an abapGit offline zip into a package, in one call, and the
// removal of what such an import left: git_import_zip and git_delete_objects.
//
// The zip goes to ZADT_VSP's git domain (ZCL_VSP_GIT_SERVICE) in chunks with
// a SHA-256, and abapGit's own deserialize runs on the system -- as background
// job ZVSP_GIT_IMPORT, because it activates and may run for minutes, which an
// ABAP Push Channel is not the place for. Everything this side can decide is
// decided before a byte is sent: read-only, the operation filter, the package
// whitelist for the target and for every package the zip maps to (read from
// its .abapgit.xml and folders), and the transport for a transportable
// package. ZCL_VSP_GIT_SERVICE checks again that every file maps to one of the
// packages checked here, and applies abapGit's deserialize checks: nothing is
// overwritten without overwrite, and conflicts or unmet requirements refuse.

const (
	// GitZipMaxBytes caps the zip.
	GitZipMaxBytes = 20 << 20
	// gitZipMaxEntries caps the files in it, and gitZipMaxUnzipped their
	// declared uncompressed size (200 MB). ZCL_VSP_GIT_SERVICE checks the
	// same limits from the zip's directory before abapGit decompresses it.
	gitZipMaxEntries  = 50000
	gitZipMaxUnzipped = 200 << 20
	// gitDotAbapgitMaxBytes caps .abapgit.xml, the one file read here.
	gitDotAbapgitMaxBytes = 1 << 20
	// gitUploadChunk is the payload of one WebSocket message, before base64.
	gitUploadChunk = 256 << 10

	gitDomain        = "git"
	gitImportJobName = "ZVSP_GIT_IMPORT"
)

// Folder logics of .abapgit.xml.
const (
	GitFolderLogicPrefix = "PREFIX"
	GitFolderLogicFull   = "FULL"
	GitFolderLogicMixed  = "MIXED"
)

var (
	gitPackageRe = regexp.MustCompile(`^[A-Z0-9_$/]{1,30}$`)
	gitObjTypeRe = regexp.MustCompile(`^[A-Z0-9]{4}$`)
	gitJobRe     = regexp.MustCompile(`^[0-9]{8}$`)
)

// GitZipObject is an object an abapGit zip carries, as its file names say.
type GitZipObject struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Package string `json:"package"`
}

// GitZipPlan is what a zip would touch, read from the zip alone: the package
// of each folder under the starting folder, by the zip's folder logic, and
// the objects its file names name. Nothing on the system is consulted.
type GitZipPlan struct {
	Package        string `json:"package"`
	StartingFolder string `json:"startingFolder"`
	FolderLogic    string `json:"folderLogic"`
	MainLanguage   string `json:"mainLanguage,omitempty"`
	// Packages are every package a file of the zip maps to, the target
	// first. The import is refused on the system if any file maps elsewhere.
	Packages []string       `json:"packages"`
	Objects  []GitZipObject `json:"objects"`
	Size     int            `json:"size"`
	SHA256   string         `json:"sha256"`
}

type dotAbapgit struct {
	Data struct {
		MasterLanguage string `xml:"MASTER_LANGUAGE"`
		StartingFolder string `xml:"STARTING_FOLDER"`
		FolderLogic    string `xml:"FOLDER_LOGIC"`
	} `xml:"values>DATA"`
}

// NormalizeGitPackage upper-cases and checks a package name.
func NormalizeGitPackage(pkg string) (string, error) {
	p := strings.ToUpper(strings.TrimSpace(pkg))
	if p == "" {
		return "", errors.New("package is required")
	}
	if !gitPackageRe.MatchString(p) {
		return "", fmt.Errorf("package %q is not a package name (A-Z, 0-9, _, $, /; at most 30)", pkg)
	}
	return p, nil
}

// AnalyzeGitZip reads an abapGit offline zip for import into pkg: its
// .abapgit.xml (required, at the root), and the folder and file names of
// everything else. It does no I/O beyond data.
func AnalyzeGitZip(data []byte, pkg string) (*GitZipPlan, error) {
	target, err := NormalizeGitPackage(pkg)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("the zip is empty")
	}
	if len(data) > GitZipMaxBytes {
		return nil, fmt.Errorf("the zip is %d bytes, over the %d-byte (20 MB) limit", len(data), GitZipMaxBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a zip: %w", err)
	}
	if len(zr.File) > gitZipMaxEntries {
		return nil, fmt.Errorf("the zip has %d entries, over the %d limit", len(zr.File), gitZipMaxEntries)
	}
	// Checked before each addition, so a declared size near 2^64 cannot
	// wrap the sum back under the limit.
	var unzipped uint64
	for _, f := range zr.File {
		if f.UncompressedSize64 > gitZipMaxUnzipped-unzipped {
			return nil, fmt.Errorf("the zip unpacks to more than %d bytes (200 MB), the limit", gitZipMaxUnzipped)
		}
		unzipped += f.UncompressedSize64
	}

	var dot *zip.File
	type entry struct{ dir, file string }
	var entries []entry
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasSuffix(name, "/") {
			continue // a directory entry
		}
		clean := path.Clean("/" + name)
		if clean != "/"+strings.TrimPrefix(name, "./") || strings.Contains(name, "..") {
			return nil, fmt.Errorf("zip entry %q is not a plain relative path", f.Name)
		}
		dir, file := path.Split(clean)
		if dir == "/" && file == ".abapgit.xml" {
			if dot != nil {
				return nil, errors.New("the zip has more than one .abapgit.xml at its root: which one abapGit would read is not defined")
			}
			dot = f
			continue
		}
		entries = append(entries, entry{dir, file})
	}
	if dot == nil {
		return nil, errors.New("the zip has no .abapgit.xml at its root: it is not an abapGit offline zip")
	}
	rc, err := dot.Open()
	if err != nil {
		return nil, fmt.Errorf(".abapgit.xml: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(rc, gitDotAbapgitMaxBytes+1))
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf(".abapgit.xml: %w", err)
	}
	if len(raw) > gitDotAbapgitMaxBytes {
		return nil, errors.New(".abapgit.xml is over 1 MB")
	}
	var d dotAbapgit
	if err := xml.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf(".abapgit.xml is not readable: %w", err)
	}

	plan := &GitZipPlan{
		Package:        target,
		StartingFolder: strings.TrimSpace(d.Data.StartingFolder),
		FolderLogic:    strings.ToUpper(strings.TrimSpace(d.Data.FolderLogic)),
		MainLanguage:   strings.TrimSpace(d.Data.MasterLanguage),
		Size:           len(data),
		SHA256:         sha256Hex(data),
	}
	// abapGit's defaults (zcl_abapgit_dot_abapgit=>build_default).
	if plan.StartingFolder == "" {
		plan.StartingFolder = "/src/"
	}
	if !strings.HasPrefix(plan.StartingFolder, "/") || !strings.HasSuffix(plan.StartingFolder, "/") {
		return nil, fmt.Errorf(".abapgit.xml: starting folder %q is not /<folder>/", plan.StartingFolder)
	}
	if plan.FolderLogic == "" {
		plan.FolderLogic = GitFolderLogicPrefix
	}
	switch plan.FolderLogic {
	case GitFolderLogicPrefix, GitFolderLogicFull, GitFolderLogicMixed:
	default:
		return nil, fmt.Errorf(".abapgit.xml: folder logic %q is not PREFIX, FULL or MIXED", plan.FolderLogic)
	}

	packages := map[string]bool{target: true}
	plan.Packages = []string{target}
	seenObj := map[string]bool{}
	for _, e := range entries {
		if !strings.HasPrefix(e.dir, plan.StartingFolder) {
			continue // abapGit reads no object outside the starting folder
		}
		chain, err := gitFolderPackages(target, plan.FolderLogic, strings.TrimPrefix(e.dir, plan.StartingFolder))
		if err != nil {
			return nil, fmt.Errorf("%s%s: %w", e.dir, e.file, err)
		}
		for _, p := range chain {
			if !packages[p] {
				packages[p] = true
				plan.Packages = append(plan.Packages, p)
			}
		}
		folderPkg := target
		if len(chain) > 0 {
			folderPkg = chain[len(chain)-1]
		}
		obj, ok := gitObjectFromFile(e.file, folderPkg)
		if !ok {
			continue
		}
		key := obj.Type + " " + obj.Name
		if !seenObj[key] {
			seenObj[key] = true
			plan.Objects = append(plan.Objects, obj)
		}
	}
	sort.Strings(plan.Packages[1:])
	sort.Slice(plan.Objects, func(i, j int) bool {
		if plan.Objects[i].Type != plan.Objects[j].Type {
			return plan.Objects[i].Type < plan.Objects[j].Type
		}
		return plan.Objects[i].Name < plan.Objects[j].Name
	})
	return plan, nil
}

// gitFolderPackages is the package of each folder level of rel (a path below
// the starting folder, "" or "a/b/"), as zcl_abapgit_folder_logic's
// path_to_package derives them.
func gitFolderPackages(top, logic, rel string) ([]string, error) {
	var out []string
	cur := top
	for _, seg := range strings.Split(strings.Trim(rel, "/"), "/") {
		if seg == "" {
			continue
		}
		var name string
		switch logic {
		case GitFolderLogicFull:
			name = strings.ReplaceAll(seg, "#", "/")
			if strings.HasPrefix(top, "$") {
				name = "$" + name
			}
		case GitFolderLogicPrefix:
			name = cur + "_" + seg
		case GitFolderLogicMixed:
			name = top + "_" + seg
		}
		name = strings.ToUpper(name)
		if len(name) > 30 {
			return nil, fmt.Errorf("package %s exceeds the 30-character limit", name)
		}
		if !gitPackageRe.MatchString(name) {
			return nil, fmt.Errorf("folder %q maps to %q, which is not a package name", seg, name)
		}
		out = append(out, name)
		cur = name
	}
	return out, nil
}

// gitObjectFromFile reads the object an abapGit file name names:
// <name>.<type>[.<extra>].<ext>, the name with '#' for '/' and %-escapes.
// package.devc.xml is the folder's package.
func gitObjectFromFile(file, folderPkg string) (GitZipObject, bool) {
	parts := strings.Split(file, ".")
	if len(parts) < 3 {
		return GitZipObject{}, false
	}
	typ := strings.ToUpper(parts[1])
	if !gitObjTypeRe.MatchString(typ) {
		return GitZipObject{}, false
	}
	if typ == "DEVC" && strings.EqualFold(parts[0], "package") {
		return GitZipObject{Type: "DEVC", Name: folderPkg, Package: folderPkg}, true
	}
	name, err := url.PathUnescape(strings.ReplaceAll(parts[0], "#", "/"))
	if err != nil || name == "" {
		return GitZipObject{}, false
	}
	return GitZipObject{Type: typ, Name: strings.ToUpper(name), Package: folderPkg}, true
}

// ReadGitZip reads a zip from a local path: a regular file, not a link,
// within the size limit.
func ReadGitZip(p string) ([]byte, error) {
	if strings.TrimSpace(p) == "" {
		return nil, errors.New("file_path is required")
	}
	return readRegularFile(p, GitZipMaxBytes)
}

// DecodeGitZipBase64 decodes base64 zip content, refusing more than the limit
// before decoding.
func DecodeGitZipBase64(b64 string) ([]byte, error) {
	b64 = strings.TrimSpace(b64)
	if base64.StdEncoding.DecodedLen(len(b64)) > GitZipMaxBytes+3 {
		return nil, fmt.Errorf("over the %d-byte limit", GitZipMaxBytes)
	}
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("not base64: %w", err)
	}
	return b, nil
}

// --- gates -----------------------------------------------------------------

// GitImportOptions is what an import is asked to do besides the zip.
type GitImportOptions struct {
	Package string
	// RepoName names the offline repository created for the package; the
	// package name when empty. An existing repository keeps its name.
	RepoName  string
	Overwrite bool
	// Transport is the request for a transportable package; chosen as for
	// any write when empty (TransportChoice), and required then.
	Transport string
}

func isLocalPackage(pkg string) bool { return strings.HasPrefix(pkg, "$") }

// CheckGitImportPolicy runs the checks an import into pkg needs before the
// zip is even read: not read-only, create/update/activate allowed -- and
// delete too with overwrite, since abapGit's delete_add deletes an object and
// creates it again --, the target in the package whitelist, and for a
// transportable target the opt-in to transportable edits, with the
// transport whitelist on a named request. It does no I/O.
func (c *Client) CheckGitImportPolicy(pkg, transport string, overwrite bool) error {
	const op = "GitImportZip"
	if c.config.Safety.ReadOnly {
		return fmt.Errorf("operation '%s' is blocked: read-only mode enabled (an import creates and changes objects)", op)
	}
	ops := []OperationType{OpCreate, OpUpdate, OpActivate}
	if overwrite {
		ops = append(ops, OpDelete)
	}
	for _, o := range ops {
		if err := c.checkSafety(o, op); err != nil {
			return err
		}
	}
	p, err := NormalizeGitPackage(pkg)
	if err != nil {
		return err
	}
	if err := c.checkPackageSafety(p); err != nil {
		return err
	}
	transport = strings.ToUpper(strings.TrimSpace(transport))
	if transport != "" && !requestRe.MatchString(transport) {
		return fmt.Errorf("transport %q is not <SID>K<6 digits>", transport)
	}
	if isLocalPackage(p) {
		if transport != "" {
			return fmt.Errorf("package %s is local ($): it takes no transport; leave transport empty", p)
		}
		return nil
	}
	if !c.config.Safety.AllowTransportableEdits {
		return fmt.Errorf("operation '%s' into %s is blocked: %s is not a local ($) package, and editing transportable objects is disabled "+
			"(--allow-transportable-edits or SAP_ALLOW_TRANSPORTABLE_EDITS=true, plus a transport)", op, p, p)
	}
	// No transport named, and none may be chosen: refused here, before the
	// zip is read or ZADT_VSP dialled, not later in resolveGitImportTransport.
	if transport == "" && c.config.Safety.TransportChoice == "off" {
		return fmt.Errorf("operation '%s' into %s is blocked: %s is transportable, no transport was named, and transport choice is off: name the transport", op, p, p)
	}
	return c.checkTransportableEdit(transport, op)
}

// CheckGitImportPlan checks every package the zip maps to against the
// package whitelist. It does no I/O.
func (c *Client) CheckGitImportPlan(plan *GitZipPlan) error {
	if plan == nil {
		return errors.New("no zip")
	}
	for _, p := range plan.Packages {
		if err := c.checkPackageSafety(p); err != nil {
			return fmt.Errorf("the zip maps files to package %s (%s folder logic): %w", p, plan.FolderLogic, err)
		}
		if isLocalPackage(p) != isLocalPackage(plan.Package) {
			return fmt.Errorf("the zip maps files to package %s, and %s is %s: abapGit refuses a mix of local and transportable packages",
				p, plan.Package, map[bool]string{true: "local", false: "transportable"}[isLocalPackage(plan.Package)])
		}
	}
	return nil
}

// resolveGitImportTransport is the request an import into a transportable
// package goes under: the one named, else the one main's transport choice
// picks for the package (an open request that already holds it, the only
// one, the newest; created only with --enable-transports). Gated like any
// transportable edit.
func (c *Client) resolveGitImportTransport(ctx context.Context, pkg, transport string) (string, string, error) {
	const op = "GitImportZip"
	if isLocalPackage(pkg) {
		return "", "", nil
	}
	if transport != "" {
		return transport, "", c.checkTransportableEdit(transport, op)
	}
	if c.config.Safety.TransportChoice == "off" {
		return "", "", fmt.Errorf("package %s is transportable: name the transport", pkg)
	}
	choice, err := c.chooseTransport(ctx, GetObjectURL(ObjectTypePackage, pkg, ""), pkg, "", "vsp git import-zip "+pkg)
	if err != nil {
		return "", "", fmt.Errorf("package %s is transportable and no transport was named; choosing one failed: %w", pkg, err)
	}
	if choice.Transport == "" {
		if choice.Reason == "" {
			// The check says the package records nothing.
			return "", "the package records no changes", nil
		}
		return "", "", fmt.Errorf("package %s is transportable and no transport was named: %s", pkg, choice.Reason)
	}
	if err := c.checkTransportableEdit(choice.Transport, op); err != nil {
		return "", "", err
	}
	return choice.Transport, choice.Reason, nil
}

// --- the ZADT_VSP git domain -----------------------------------------------

// GitService is the part of ZADT_VSP's WebSocket client the git domain needs.
type GitService interface {
	SendDomainRequest(ctx context.Context, domain, action string, params map[string]any, timeout time.Duration) (*WSResponse, error)
}

// GitServiceError is a refusal or failure reported by ZCL_VSP_GIT_SERVICE.
type GitServiceError struct {
	Action, Code, Message string
}

func (e *GitServiceError) Error() string {
	return fmt.Sprintf("git.%s: %s: %s", e.Action, e.Code, e.Message)
}

func gitCall(ctx context.Context, ws GitService, action string, params map[string]any, timeout time.Duration, out any) error {
	if ws == nil {
		return errors.New("the abapGit import needs ZADT_VSP (a WebSocket to the system)")
	}
	resp, err := ws.SendDomainRequest(ctx, gitDomain, action, params, timeout)
	if err != nil {
		return fmt.Errorf("git.%s: %w", action, err)
	}
	if !resp.Success {
		if resp.Error != nil {
			if resp.Error.Code == "UNKNOWN_DOMAIN" {
				return errors.New("ZADT_VSP on this system has no git service: abapGit is not installed, or ZCL_VSP_GIT_SERVICE was not deployed " +
					"(vsp install zadt-vsp deploys it where abapGit is installed)")
			}
			if resp.Error.Code == "GIT_ERROR" && strings.HasPrefix(resp.Error.Message, "Unknown action") {
				return fmt.Errorf("ZADT_VSP's git service on this system predates %s: deploy the current ZADT_VSP (vsp install zadt-vsp)", action)
			}
			return &GitServiceError{Action: action, Code: resp.Error.Code, Message: resp.Error.Message}
		}
		return fmt.Errorf("git.%s failed", action)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("git.%s: unreadable answer: %w", action, err)
	}
	return nil
}

// GitImportStarted is an import handed to its background job.
type GitImportStarted struct {
	System    string      `json:"system"`
	Client    string      `json:"client"`
	Package   string      `json:"package"`
	Job       string      `json:"job"`
	JobCount  string      `json:"jobCount"`
	Transport string      `json:"transport,omitempty"`
	Note      string      `json:"note,omitempty"`
	Plan      *GitZipPlan `json:"plan"`
}

type gitBeginAnswer struct {
	AssemblyID string `json:"assembly_id"`
	Package    string `json:"package"`
	System     string `json:"system"`
	Client     string `json:"client"`
}

type gitCommitAnswer struct {
	Status   string `json:"status"`
	Job      string `json:"job"`
	JobCount string `json:"job_count"`
	Package  string `json:"package"`
}

// StartGitImport sends the zip and starts the import job. Order: the gates
// (again: a caller may not have run them), the transport, begin (ZADT_VSP
// checks the parameters and that its job program is there, and answers
// which system and client it is), a check that this is the configured
// client, the chunks, and commit (size and SHA-256, a zip with .abapgit.xml,
// the job). The import's outcome is WaitGitImport's or GitImportStatus's.
func (c *Client) StartGitImport(ctx context.Context, ws GitService, zipData []byte, opts GitImportOptions) (*GitImportStarted, error) {
	if err := c.CheckGitImportPolicy(opts.Package, opts.Transport, opts.Overwrite); err != nil {
		return nil, err
	}
	plan, err := AnalyzeGitZip(zipData, opts.Package)
	if err != nil {
		return nil, err
	}
	if err = c.CheckGitImportPlan(plan); err != nil {
		return nil, err
	}
	repoName := strings.TrimSpace(opts.RepoName)
	if repoName == "" {
		repoName = plan.Package
	}
	if len(repoName) > 60 || strings.ContainsFunc(repoName, func(r rune) bool { return r < 0x20 }) {
		return nil, errors.New("repo_name is at most 60 characters, without control characters")
	}
	transport, reason, err := c.resolveGitImportTransport(ctx, plan.Package, strings.ToUpper(strings.TrimSpace(opts.Transport)))
	if err != nil {
		return nil, err
	}

	// One upload at a time per connection: ZADT_VSP holds one assembly per
	// session, and an MCP server shares its WebSocket between calls.
	unlock := lockGitUpload(ws)
	defer unlock()

	var begin gitBeginAnswer
	err = gitCall(ctx, ws, "import_zip", map[string]any{
		"step":      "begin",
		"size":      len(zipData),
		"sha256":    plan.SHA256,
		"package":   plan.Package,
		"repo_name": repoName,
		"overwrite": fmt.Sprintf("%t", opts.Overwrite),
		"transport": transport,
		"packages":  strings.Join(plan.Packages, ","),
	}, time.Minute, &begin)
	var se *GitServiceError
	switch {
	case errors.As(err, &se):
		// ZADT_VSP answered with a refusal: it holds no upload.
		return nil, err
	case err != nil:
		// No answer (time ran out, the call was cancelled, the connection
		// failed): begin may have been accepted, and its assembly id is
		// lost with the answer -- an upload nobody can abort, which would
		// refuse every later import on this connection (UPLOAD_IN_PROGRESS).
		// A new session holds none, so the connection is reset.
		return nil, fmt.Errorf("%w; nothing was imported%s", err, resetGitConnection(ws))
	}
	// abort discards the upload, bounded in time; when that is not
	// confirmed, the connection is reset instead.
	abort := func() string {
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if aerr := gitCall(actx, ws, "import_zip", map[string]any{"step": "abort", "assembly_id": begin.AssemblyID}, 30*time.Second, nil); aerr != nil {
			return resetGitConnection(ws)
		}
		return ""
	}
	if begin.AssemblyID == "" {
		return nil, fmt.Errorf("git.import_zip: ZADT_VSP answered begin without an assembly id%s", resetGitConnection(ws))
	}
	if !strings.EqualFold(begin.Package, plan.Package) {
		note := abort()
		return nil, fmt.Errorf("ZADT_VSP read the package as %q, not %q; nothing was imported%s", begin.Package, plan.Package, note)
	}
	if !sameClient(begin.Client, c.config.Client) {
		note := abort()
		return nil, fmt.Errorf("ZADT_VSP answered from client %s of %s, but this connection is configured for client %s; nothing was imported%s",
			begin.Client, begin.System, strings.TrimSpace(c.config.Client), note)
	}
	for off := 0; off < len(zipData); off += gitUploadChunk {
		end := min(off+gitUploadChunk, len(zipData))
		if err := gitCall(ctx, ws, "import_zip", map[string]any{
			"step":        "chunk",
			"assembly_id": begin.AssemblyID,
			"offset":      off,
			"chunk_b64":   base64.StdEncoding.EncodeToString(zipData[off:end]),
		}, time.Minute, nil); err != nil {
			note := abort()
			return nil, fmt.Errorf("sending the zip at offset %d: %w; nothing was imported%s", off, err, note)
		}
	}
	var commit gitCommitAnswer
	started := &GitImportStarted{System: begin.System, Client: begin.Client, Package: plan.Package,
		Job: gitImportJobName, Transport: transport, Plan: plan}
	if reason != "" {
		started.Note = "transport: " + reason
	}
	if err := gitCall(ctx, ws, "import_zip", map[string]any{"step": "commit", "assembly_id": begin.AssemblyID}, 2*time.Minute, &commit); err != nil {
		if errors.As(err, &se) {
			// ZADT_VSP answered: the commit was refused (size, SHA-256, the
			// zip, the job), and nothing was started.
			return nil, err
		}
		// No answer: the job may have been scheduled before the
		// connection failed or the time ran out. A commit that ran took
		// the upload with it; one that never arrived left it held, so the
		// connection is reset (that does not touch a scheduled job).
		reset := resetGitConnection(ws)
		owner := ""
		if u := strings.TrimSpace(c.config.Username); u != "" {
			owner = " of user " + strings.ToUpper(u)
		}
		started.Note = strings.TrimSpace(fmt.Sprintf("the import may be running: the commit got no answer (%v). Look for job %s%s in SM37, "+
			"and read its outcome with git_import_status (vsp git import-status <job number>) before importing again.%s %s",
			err, gitImportJobName, owner, reset, started.Note))
		c.InvalidateCache()
		return started, &GitImportUnconfirmedError{Err: err, Started: started}
	}
	started.Job = orDefaultString(commit.Job, gitImportJobName)
	started.JobCount = commit.JobCount
	return started, nil
}

// GitImportUnconfirmedError says the zip was sent and the commit got no
// answer: the import job may have been started, or not. Started holds what
// is known (no job number).
type GitImportUnconfirmedError struct {
	Err     error
	Started *GitImportStarted
}

func (e *GitImportUnconfirmedError) Error() string { return e.Started.Note }
func (e *GitImportUnconfirmedError) Unwrap() error { return e.Err }

// gitConnectionCloser is a git service connection that can be closed: the
// WebSocket client. ZADT_VSP drops an upload with its session.
type gitConnectionCloser interface {
	Close() error
}

// resetGitConnection closes ws when it can be closed, so that an upload
// ZADT_VSP may hold for it goes with the session (the MCP server dials a
// new connection on its next call). It returns what to tell the caller.
func resetGitConnection(ws GitService) string {
	cl, ok := ws.(gitConnectionCloser)
	if !ok {
		return ""
	}
	_ = cl.Close()
	return " (the ZADT_VSP connection was reset, so that no upload stays held on it)"
}

// gitUploads serializes begin..commit per git service connection. An entry
// lives while anyone holds or waits for its mutex, and goes with the last
// of them, so that a connection the MCP server has replaced is not kept.
var (
	gitUploadsMu sync.Mutex
	gitUploads   = map[GitService]*gitUploadLock{}
)

type gitUploadLock struct {
	mu    sync.Mutex
	users int // holding or waiting; guarded by gitUploadsMu
}

func lockGitUpload(ws GitService) func() {
	if ws == nil {
		return func() {}
	}
	gitUploadsMu.Lock()
	l := gitUploads[ws]
	if l == nil {
		l = &gitUploadLock{}
		gitUploads[ws] = l
	}
	l.users++
	gitUploadsMu.Unlock()

	l.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Unlock()
			gitUploadsMu.Lock()
			l.users--
			if l.users == 0 && gitUploads[ws] == l {
				delete(gitUploads, ws)
			}
			gitUploadsMu.Unlock()
		})
	}
}

func orDefaultString(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// GitLogLine is one error, warning or abort message of abapGit's log.
type GitLogLine struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	ObjType string `json:"objType,omitempty"`
	ObjName string `json:"objName,omitempty"`
}

// GitTadirRow is a TADIR row the import created or changed.
type GitTadirRow struct {
	PgmID    string `json:"pgmid"`
	Object   string `json:"object"`
	ObjName  string `json:"objName"`
	DevClass string `json:"devclass"`
	Created  bool   `json:"created"`
}

// GitDecision is what the import decided for one object abapGit's checks
// listed: add, update, overwrite, delete_add (Y with overwrite), delete
// (always N: a local object not in the zip is kept), or package_kept (always
// N: the target package existed without a repository, and its own entry is
// left as it is).
type GitDecision struct {
	ObjType  string `json:"objType"`
	ObjName  string `json:"objName"`
	DevClass string `json:"devclass,omitempty"`
	Action   string `json:"action"`
	Decision string `json:"decision"`
}

// Outcomes of an import.
const (
	GitImported           = "imported"
	GitImportedWithErrors = "imported_with_errors"
	GitRefused            = "refused"
	GitFailed             = "failed"
)

// GitImportResult is what the import job did.
type GitImportResult struct {
	// Outcome: imported, imported_with_errors (the log has E/A messages),
	// refused (nothing imported) or failed (deserialize stopped partway).
	Outcome     string `json:"outcome"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	System      string `json:"system,omitempty"`
	Client      string `json:"client,omitempty"`
	Package     string `json:"package"`
	RepoKey     string `json:"repoKey,omitempty"`
	RepoName    string `json:"repoName,omitempty"`
	RepoCreated bool   `json:"repoCreated"`
	// PackageCreated: the target package did not exist and the import
	// created it (a local package has no TADIR row of its own).
	PackageCreated bool          `json:"packageCreated"`
	Transport      string        `json:"transport,omitempty"`
	InfoCount      int           `json:"infoCount"`
	Log            []GitLogLine  `json:"log"`
	Tadir          []GitTadirRow `json:"tadir"`
	Decisions      []GitDecision `json:"decisions,omitempty"`
}

type gitResultAnswer struct {
	Outcome     string `json:"outcome"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	System      string `json:"system"`
	Client      string `json:"client"`
	Package     string `json:"package"`
	RepoKey     string `json:"repo_key"`
	RepoName    string `json:"repo_name"`
	RepoCreated bool   `json:"repo_created"`
	PkgCreated  bool   `json:"package_created"`
	Transport   string `json:"transport"`
	InfoCount   int    `json:"info_count"`
	Log         []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		ObjType string `json:"obj_type"`
		ObjName string `json:"obj_name"`
	} `json:"log"`
	Tadir []struct {
		PgmID    string `json:"pgmid"`
		Object   string `json:"object"`
		ObjName  string `json:"obj_name"`
		DevClass string `json:"devclass"`
		Created  bool   `json:"created"`
	} `json:"tadir"`
	Decisions []struct {
		ObjType  string `json:"obj_type"`
		ObjName  string `json:"obj_name"`
		DevClass string `json:"devclass"`
		Action   string `json:"action"`
		Decision string `json:"decision"`
	} `json:"decisions"`
}

func (a *gitResultAnswer) result() *GitImportResult {
	t := strings.TrimSpace
	r := &GitImportResult{Outcome: a.Outcome, Code: a.Code, Message: t(a.Message), System: a.System, Client: a.Client,
		Package: t(a.Package), RepoKey: t(a.RepoKey), RepoName: t(a.RepoName), RepoCreated: a.RepoCreated, PackageCreated: a.PkgCreated,
		Transport: t(a.Transport), InfoCount: a.InfoCount, Log: []GitLogLine{}, Tadir: []GitTadirRow{}, Decisions: []GitDecision{}}
	for _, l := range a.Log {
		r.Log = append(r.Log, GitLogLine{Type: t(l.Type), Text: t(l.Text), ObjType: t(l.ObjType), ObjName: t(l.ObjName)})
	}
	for _, row := range a.Tadir {
		r.Tadir = append(r.Tadir, GitTadirRow{PgmID: t(row.PgmID), Object: t(row.Object), ObjName: t(row.ObjName), DevClass: t(row.DevClass), Created: row.Created})
	}
	for _, d := range a.Decisions {
		r.Decisions = append(r.Decisions, GitDecision{ObjType: t(d.ObjType), ObjName: t(d.ObjName), DevClass: t(d.DevClass), Action: d.Action, Decision: t(d.Decision)})
	}
	return r
}

// Job states of an import, as GitImportStatus reports them.
const (
	GitJobPending = "pending"
	GitJobDone    = "done"
	GitJobFailed  = "failed"
	GitJobUnknown = "unknown"
)

// GitImportStatus is the state of an import job and, once it is done, its
// result.
type GitImportStatus struct {
	Job       string           `json:"job"`
	JobCount  string           `json:"jobCount"`
	JobFound  bool             `json:"jobFound"`
	JobStatus string           `json:"jobStatus,omitempty"`
	State     string           `json:"state"`
	JobLog    []string         `json:"jobLog,omitempty"`
	Result    *GitImportResult `json:"result,omitempty"`
	// Pushed says the job's push reached this connection.
	Pushed bool   `json:"pushed,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Terminal says the state will not change by waiting.
func (s *GitImportStatus) Terminal() bool {
	return s.State == GitJobDone || s.State == GitJobFailed
}

type gitStatusAnswer struct {
	Job       string           `json:"job"`
	JobCount  string           `json:"job_count"`
	JobFound  bool             `json:"job_found"`
	JobStatus string           `json:"job_status"`
	Outcome   string           `json:"outcome"`
	JobLog    []string         `json:"job_log"`
	Result    *gitResultAnswer `json:"result"`
}

// GitImportStatus reads the state of import job jobCount. It changes
// nothing, and is allowed under read-only.
func (c *Client) GitImportStatus(ctx context.Context, ws GitService, jobCount string) (*GitImportStatus, error) {
	jobCount = strings.TrimSpace(jobCount)
	if !gitJobRe.MatchString(jobCount) {
		return nil, fmt.Errorf("job %q is not a job number (8 digits)", jobCount)
	}
	var a gitStatusAnswer
	if err := gitCall(ctx, ws, "import_status", map[string]any{"job": jobCount}, time.Minute, &a); err != nil {
		return nil, err
	}
	st := &GitImportStatus{Job: orDefaultString(a.Job, gitImportJobName), JobCount: orDefaultString(a.JobCount, jobCount),
		JobFound: a.JobFound, JobStatus: strings.TrimSpace(a.JobStatus), State: a.Outcome, JobLog: a.JobLog}
	if a.Result != nil {
		st.Result = a.Result.result()
	}
	// "done" is believed only with a result behind it.
	if st.State == GitJobDone && st.Result == nil {
		st.State = GitJobUnknown
	}
	switch st.State {
	case GitJobDone, GitJobPending, GitJobFailed:
	default:
		st.State = GitJobUnknown
	}
	st.Note = gitStatusNote(st)
	if st.Terminal() {
		// The job wrote through abapGit, not through this client's HTTP
		// requests: whatever the response cache holds may be stale.
		c.InvalidateCache()
	}
	return st, nil
}

func gitStatusNote(st *GitImportStatus) string {
	ref := st.Job + " " + st.JobCount
	switch st.State {
	case GitJobPending:
		return fmt.Sprintf("the import is still running; ask again (git_import_status job=%s), or see job %s in SM37", st.JobCount, ref)
	case GitJobFailed:
		return fmt.Sprintf("job %s ended without a result (cancelled or dumped): see its log, SM37 and ST22; objects may be partly imported", ref)
	case GitJobDone:
		if st.Result != nil {
			switch st.Result.Outcome {
			case GitRefused:
				return "the import was refused: nothing was imported"
			case GitFailed:
				return "the import stopped partway: see the log; objects may be partly imported"
			case GitImportedWithErrors:
				return "the import ran, and abapGit's log has errors"
			}
		}
		return ""
	}
	return fmt.Sprintf("the state of job %s is unknown: see SM37", ref)
}

// GitPusher is a WebSocket client that receives ZADT_VSP's pushes.
type GitPusher interface {
	AwaitPush(ctx context.Context, id string) (*WSResponse, error)
	TakePush(id string) (*WSResponse, bool)
}

func gitPushID(jobCount string) string { return "push:git:" + jobCount }

var gitPollInterval = 3 * time.Second

// WaitGitImport waits until import job jobCount is done or failed, or ctx
// ends; then the last state read is returned, unknown if none was. When ws
// receives pushes, the job's push wakes it early; the state is always
// import_status's.
//
// Only ctx ending returns ctx's error (a --wait that ran out: the job may be
// pending). Anything else that stops the wait is returned as itself, never
// as a timeout: a status read that fails (a refusal, a lost connection), or
// the push channel failing (ErrWebSocketClosed), which also cancels a
// status read in flight.
func (c *Client) WaitGitImport(ctx context.Context, ws GitService, jobCount string) (*GitImportStatus, error) {
	wctx, fail := context.WithCancelCause(ctx)
	defer fail(nil)
	var pushed <-chan struct{}
	if p, ok := ws.(GitPusher); ok {
		ch := make(chan struct{}, 1)
		go func() {
			_, err := p.AwaitPush(wctx, gitPushID(jobCount))
			switch {
			case err == nil:
				ch <- struct{}{}
			case wctx.Err() == nil:
				fail(fmt.Errorf("the push channel failed while waiting for job %s %s: %w", gitImportJobName, jobCount, err))
			}
		}()
		pushed = ch
	}
	// stopped is why the wait cannot go on, nil while it can.
	stopped := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if wctx.Err() != nil {
			return context.Cause(wctx)
		}
		return nil
	}
	var last *GitImportStatus
	got := false
	for {
		st, err := c.GitImportStatus(wctx, ws, jobCount)
		if err == nil {
			st.Pushed = got
			last = st
			if st.Terminal() {
				return st, nil
			}
		}
		// A read cancelled because the wait stopped reports why it stopped.
		if serr := stopped(); serr != nil {
			return gitUnknown(last, jobCount), serr
		}
		if err != nil {
			return gitUnknown(last, jobCount), fmt.Errorf("reading the state of job %s %s: %w", gitImportJobName, jobCount, err)
		}
		select {
		case <-wctx.Done():
			return gitUnknown(last, jobCount), stopped()
		case <-pushed:
			got = true
			pushed = nil
		case <-time.After(gitPollInterval):
		}
	}
}

func gitUnknown(last *GitImportStatus, jobCount string) *GitImportStatus {
	if last == nil {
		last = &GitImportStatus{Job: gitImportJobName, JobCount: jobCount}
	}
	out := *last
	if !out.Terminal() {
		if out.State != GitJobPending {
			out.State = GitJobUnknown
		}
		out.Note = gitStatusNote(&out)
	}
	return &out
}

// --- delete ----------------------------------------------------------------

// GitDeleteItem is one object git_delete_objects is asked to delete. With
// Expect, only while it is still that version (see GitExpect).
type GitDeleteItem struct {
	Type   string     `json:"type"`
	Name   string     `json:"name"`
	Expect *GitExpect `json:"expect,omitempty"`
}

// ParseGitDeleteItems reads the objects to delete: "TYPE NAME" strings
// ("R3TR TYPE NAME" too), or {"type","name"[,"expect"]} objects, as a list
// or one comma-separated string. An object map with any other key, or an
// expect that cannot be checked, is refused: an expectation is never
// dropped.
func ParseGitDeleteItems(raw any) ([]GitDeleteItem, error) {
	var list []any
	switch v := raw.(type) {
	case nil:
		return nil, errors.New("objects is required: the TADIR items to delete, as \"TYPE NAME\"")
	case string:
		for _, s := range strings.Split(v, ",") {
			if strings.TrimSpace(s) != "" {
				list = append(list, s)
			}
		}
	case []any:
		list = v
	case []string:
		for _, s := range v {
			list = append(list, s)
		}
	default:
		return nil, fmt.Errorf("objects must be a list of \"TYPE NAME\" or {\"type\",\"name\"}, not %T", raw)
	}
	var out []GitDeleteItem
	seen := map[string]int{}
	for _, e := range list {
		var it GitDeleteItem
		switch v := e.(type) {
		case string:
			f := strings.Fields(strings.ToUpper(v))
			if len(f) == 3 && f[0] == "R3TR" {
				f = f[1:]
			}
			if len(f) != 2 {
				return nil, fmt.Errorf("object %q is not \"TYPE NAME\"", v)
			}
			it = GitDeleteItem{Type: f[0], Name: f[1]}
		case map[string]any:
			for k := range v {
				if k != "type" && k != "name" && k != "expect" {
					return nil, fmt.Errorf("object %v: unknown key %q (type, name, expect)", v, k)
				}
			}
			t, _ := v["type"].(string)
			n, _ := v["name"].(string)
			it = GitDeleteItem{Type: strings.ToUpper(strings.TrimSpace(t)), Name: strings.ToUpper(strings.TrimSpace(n))}
			if raw, ok := v["expect"]; ok {
				exp, err := ParseGitExpect(it.Type, raw)
				if err != nil {
					return nil, fmt.Errorf("object %s %s: %w", it.Type, it.Name, err)
				}
				it.Expect = exp
			}
		default:
			return nil, fmt.Errorf("object %v is not \"TYPE NAME\" or {\"type\",\"name\"}", e)
		}
		if !gitObjTypeRe.MatchString(it.Type) || it.Name == "" || len(it.Name) > 40 {
			return nil, fmt.Errorf("object %s %s is not a TADIR type and name", it.Type, it.Name)
		}
		k := it.Type + " " + it.Name
		if i, dup := seen[k]; dup {
			// The same object twice: one expectation, or none.
			if !sameGitExpect(out[i].Expect, it.Expect) {
				return nil, fmt.Errorf("object %s is listed twice with different expect", k)
			}
			continue
		}
		seen[k] = len(out)
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, errors.New("objects is empty: name the TADIR items to delete")
	}
	return out, nil
}

// GitObjectURL is the ADT URL an object of a TADIR type is deleted at. The
// second result is false for a type there is no ADT delete for here.
func GitObjectURL(objType, name string) (string, bool) {
	enc := url.PathEscape(strings.ToLower(name))
	// Classic objects in a namespace are addressed in upper case: SAP does
	// not recognize /demo/cl_x (see GetObjectURL); RAP and DDIC sources
	// stay lower case.
	classic := enc
	if strings.HasPrefix(name, "/") {
		classic = url.PathEscape(strings.ToUpper(name))
	}
	switch strings.ToUpper(objType) {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + classic, true
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + classic, true
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + classic, true
	case "FUGR":
		return "/sap/bc/adt/functions/groups/" + classic, true
	case "DEVC":
		return GetObjectURL(ObjectTypePackage, name, ""), true
	case "TABL":
		return "/sap/bc/adt/ddic/tables/" + enc, true
	case "DTEL":
		return "/sap/bc/adt/ddic/dataelements/" + enc, true
	case "DOMA":
		return "/sap/bc/adt/ddic/domains/" + enc, true
	case "TTYP":
		return "/sap/bc/adt/ddic/tabletypes/" + enc, true
	case "DDLS":
		return "/sap/bc/adt/ddic/ddl/sources/" + enc, true
	case "DCLS":
		return "/sap/bc/adt/acm/dcl/sources/" + enc, true
	case "BDEF":
		return "/sap/bc/adt/bo/behaviordefinitions/" + enc, true
	case "SRVD":
		return "/sap/bc/adt/ddic/srvd/sources/" + enc, true
	case "SRVB":
		return "/sap/bc/adt/businessservices/bindings/" + enc, true
	case "MSAG":
		return "/sap/bc/adt/messageclass/" + enc, true
	case "XSLT":
		return "/sap/bc/adt/xslt/transformations/" + enc, true
	}
	return "", false
}

// GitPackageObject is a TADIR row of a package.
type GitPackageObject struct {
	PgmID    string `json:"pgmid"`
	Object   string `json:"object"`
	ObjName  string `json:"objName"`
	DevClass string `json:"devclass"`
}

// GitRepoInfo is the abapGit repository registered for a package.
type GitRepoInfo struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Offline is true only when abapGit said so; State is offline, online
	// or unknown (the row is there, abapGit could not open the repository).
	// Unknown is treated as online: never unregistered.
	Offline bool   `json:"offline"`
	State   string `json:"state"`
}

// Repository states.
const (
	GitRepoOffline = "offline"
	GitRepoOnline  = "online"
	GitRepoUnknown = "unknown"
)

// GitPackageContents is a package's TADIR rows, subpackages and repository.
type GitPackageContents struct {
	Package     string             `json:"package"`
	Exists      bool               `json:"exists"`
	Truncated   bool               `json:"truncated,omitempty"`
	Objects     []GitPackageObject `json:"objects"`
	Subpackages []string           `json:"subpackages"`
	Repo        *GitRepoInfo       `json:"repo,omitempty"`
}

// Has says whether the package's TADIR has the object -- R3TR, devclass
// exactly this package.
func (p *GitPackageContents) Has(objType, name string) bool {
	for _, o := range p.Objects {
		if o.PgmID == "R3TR" && strings.EqualFold(o.Object, objType) && strings.EqualFold(o.ObjName, name) && strings.EqualFold(o.DevClass, p.Package) {
			return true
		}
	}
	return false
}

// Remaining is what keeps the package from being empty: every TADIR row but
// the package's own, and its subpackages.
func (p *GitPackageContents) Remaining() []string {
	var out []string
	for _, o := range p.Objects {
		if o.PgmID == "R3TR" && o.Object == "DEVC" && strings.EqualFold(o.ObjName, p.Package) {
			continue
		}
		out = append(out, strings.TrimSpace(o.PgmID+" "+o.Object+" "+o.ObjName))
	}
	for _, s := range p.Subpackages {
		out = append(out, "subpackage "+s)
	}
	return out
}

type gitPackageAnswer struct {
	Package   string `json:"package"`
	Exists    bool   `json:"exists"`
	Truncated bool   `json:"truncated"`
	Objects   []struct {
		PgmID    string `json:"pgmid"`
		Object   string `json:"object"`
		ObjName  string `json:"obj_name"`
		DevClass string `json:"devclass"`
	} `json:"objects"`
	Subpackages []string `json:"subpackages"`
	Repo        *struct {
		Key     string `json:"key"`
		Name    string `json:"name"`
		Offline bool   `json:"offline"`
		State   string `json:"repo_state"`
	} `json:"repo"`
}

// GitPackageObjects reads a package's TADIR rows, subpackages and abapGit
// repository through ZADT_VSP. It changes nothing.
func (c *Client) GitPackageObjects(ctx context.Context, ws GitService, pkg string) (*GitPackageContents, error) {
	p, err := NormalizeGitPackage(pkg)
	if err != nil {
		return nil, err
	}
	var a gitPackageAnswer
	if err := gitCall(ctx, ws, "package_objects", map[string]any{"package": p}, time.Minute, &a); err != nil {
		return nil, err
	}
	t := strings.TrimSpace
	out := &GitPackageContents{Package: p, Exists: a.Exists, Truncated: a.Truncated, Objects: []GitPackageObject{}, Subpackages: []string{}}
	for _, o := range a.Objects {
		out.Objects = append(out.Objects, GitPackageObject{PgmID: t(o.PgmID), Object: t(o.Object), ObjName: t(o.ObjName), DevClass: t(o.DevClass)})
	}
	for _, s := range a.Subpackages {
		out.Subpackages = append(out.Subpackages, t(s))
	}
	if a.Repo != nil {
		// Fail closed: offline only when the state says so. An answer
		// without a state (an older ZADT_VSP) falls back to its flag.
		st := strings.ToLower(t(a.Repo.State))
		switch {
		case st == GitRepoOffline || st == GitRepoOnline:
		case st == "" && a.Repo.Offline:
			st = GitRepoOffline
		case st == "":
			st = GitRepoOnline
		default:
			st = GitRepoUnknown
		}
		out.Repo = &GitRepoInfo{Key: t(a.Repo.Key), Name: t(a.Repo.Name), Offline: st == GitRepoOffline, State: st}
	}
	return out, nil
}

// GitDeleteOutcome is what happened to one requested object.
type GitDeleteOutcome struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Status string `json:"status"` // deleted, skipped, changed, failed
	Reason string `json:"reason,omitempty"`
	// Observed is the version read under the lock, for an object with an
	// expect: what it is now, when it came back changed.
	Observed *GitExpect `json:"observed,omitempty"`
}

// GitDeleteResult is what git_delete_objects did.
type GitDeleteResult struct {
	Package string             `json:"package"`
	Objects []GitDeleteOutcome `json:"objects"`
	// RepoDeleted says the repository row registered for the package was
	// removed (its objects are not touched by that): only on delete_repo,
	// only an offline repository, only of a package left empty.
	RepoDeleted bool         `json:"repoDeleted"`
	Repo        *GitRepoInfo `json:"repo,omitempty"`
	RepoNote    string       `json:"repoNote,omitempty"`
	// PackageDeleted says the package itself was removed: it was empty.
	PackageDeleted bool     `json:"packageDeleted"`
	PackageNote    string   `json:"packageNote,omitempty"`
	Remaining      []string `json:"remaining,omitempty"`
	// Order is every delete attempt, as "TYPE NAME", in the order they
	// were made (Objects keeps the order given). An object whose first
	// attempt failed in a way worth retrying is tried once more after the
	// others, so it is listed twice.
	Order []string `json:"order"`
}

// GitDeleteRank is where an object of a TADIR type goes in a delete: users
// before what they use, so that deleting one object never changes the
// abapGit serialisation of another still to be checked against its sha256
// (a TABL names its data elements; deleted first, they change it). Code and
// any type not named here first, then RAP (service binding, service
// definition, behaviour definition, access control and metadata extension,
// CDS view), search helps and lock objects, table types, tables and
// structures, data elements, domains. All TABLs share one rank: an append
// structure is not told apart from the table it extends, so it goes before
// it only when it is listed first.
func GitDeleteRank(objType string) int {
	switch strings.ToUpper(strings.TrimSpace(objType)) {
	case "SRVB":
		return 10
	case "SRVD":
		return 11
	case "BDEF":
		return 12
	case "DCLS", "DDLX":
		return 13
	case "DDLS":
		return 14
	case "SHLP", "ENQU":
		return 20
	case "TTYP":
		return 30
	case "TABL":
		return 40
	case "DTEL":
		return 50
	case "DOMA":
		return 60
	}
	return 0
}

// CheckGitDelete runs the checks a delete in pkg needs before any I/O: not
// read-only, deletes allowed, the package in the whitelist, and for a
// transportable package the opt-in and a transport.
func (c *Client) CheckGitDelete(pkg, transport string) error {
	const op = "GitDeleteObjects"
	if c.config.Safety.ReadOnly {
		return fmt.Errorf("operation '%s' is blocked: read-only mode enabled", op)
	}
	if err := c.checkSafety(OpDelete, op); err != nil {
		return err
	}
	p, err := NormalizeGitPackage(pkg)
	if err != nil {
		return err
	}
	if err := c.checkPackageSafety(p); err != nil {
		return err
	}
	transport = strings.ToUpper(strings.TrimSpace(transport))
	if transport != "" && !requestRe.MatchString(transport) {
		return fmt.Errorf("transport %q is not <SID>K<6 digits>", transport)
	}
	if isLocalPackage(p) {
		if transport != "" {
			return fmt.Errorf("package %s is local ($): it takes no transport; leave transport empty", p)
		}
		return nil
	}
	if transport == "" {
		return fmt.Errorf("package %s is transportable: deleting its objects needs a transport (and --allow-transportable-edits)", p)
	}
	return c.checkTransportableEdit(transport, op)
}

// deleteGated deletes one object through DeleteObject's gate: PrepareDelete
// (operation, package whitelist, transportable edit) before the lock, then
// lock, DELETE, and UNLOCK -- after a failed DELETE, and after a successful
// one too. A DELETE does not release the ENQUEUE its LOCK took: on a 7.58
// the TRDIR entry of a deleted program stayed in SM12 for as long as the ADT
// session lived, and abapGit refused to import the program again ("is
// locked"); the UNLOCK after the DELETE releases it. The note says when it
// could not. retry is false when a failed DELETE also left its lock behind:
// the error carries the SM12 advice, and trying again would only hide it.
func (c *Client) deleteGated(ctx context.Context, objectURL, transport string) (note string, retry bool, err error) {
	return c.deleteGatedChecked(ctx, objectURL, transport, nil)
}

// deleteGatedChecked is deleteGated with a check that runs while the lock is
// held, after LOCK and before DELETE: when it returns an error, the object
// is unlocked and not deleted, and that error is returned (retry false for
// a *GitChangedError).
func (c *Client) deleteGatedChecked(ctx context.Context, objectURL, transport string, check func(context.Context) error) (note string, retry bool, err error) {
	gctx, err := c.PrepareDelete(ctx, objectURL, transport)
	if err != nil {
		return "", true, err
	}
	lock, err := c.LockObject(gctx, objectURL, "MODIFY", transport)
	if err != nil {
		return "", true, fmt.Errorf("locking %s: %w", objectURL, err)
	}
	if check != nil {
		if cerr := check(gctx); cerr != nil {
			var changed *GitChangedError
			retry = !errors.As(cerr, &changed)
			if uerr := c.releaseLockAfterFailure(gctx, objectURL, lock.LockHandle); uerr != nil {
				return "", false, fmt.Errorf("%w -- %s", cerr, strandedLockAdvice(objectURL, uerr))
			}
			return "", retry, cerr
		}
	}
	if derr := c.DeleteObject(gctx, objectURL, lock.LockHandle, transport); derr != nil {
		if uerr := c.releaseLockAfterFailure(gctx, objectURL, lock.LockHandle); uerr != nil {
			return "", false, fmt.Errorf("%w -- %s", derr, strandedLockAdvice(objectURL, uerr))
		}
		return "", true, derr
	}
	if deleteReleasesLock(c) {
		// Behind a session-holding proxy DeleteObject has already retired
		// the stateful context, and the ENQUEUE went with it.
		return "", true, nil
	}
	if uerr := c.releaseLockAfterFailure(gctx, objectURL, lock.LockHandle); uerr != nil {
		return "its lock entry may stay in SM12 until the ADT session ends: " + uerr.Error(), true, nil
	}
	return "", true, nil
}

// gitObjectADTTypes are the ADT types a search hit may carry for an object
// of a TADIR type. The main type alone is not enough: FUGR/FF is a function
// module, not its group, and the module's URI must never stand in for the
// group's. A TADIR type not listed here is matched by its main type.
var gitObjectADTTypes = map[string][]string{
	"PROG": {"PROG/P", "PROG/I"},
	"CLAS": {"CLAS/OC"},
	"INTF": {"INTF/OI"},
	"FUGR": {"FUGR/F"},
	"TABL": {"TABL/DT", "TABL/DS"},
	"DTEL": {"DTEL/DE"},
	"DOMA": {"DOMA/DD"},
	"TTYP": {"TTYP/DA"},
	"DDLS": {"DDLS/DF"},
}

// gitHitIsObject says whether a search hit of ADT type adtType is the object
// itself for TADIR type objType, not a part of it or another kind of object.
func gitHitIsObject(objType, adtType string) bool {
	adtType = strings.ToUpper(strings.TrimSpace(adtType))
	if allowed, ok := gitObjectADTTypes[objType]; ok {
		for _, a := range allowed {
			if adtType == a {
				return true
			}
		}
		return false
	}
	main, _, _ := strings.Cut(adtType, "/")
	return main == objType
}

// gitDeleteURL is the ADT URI of the object TADIR lists as type/name in
// pkg. The TADIR type alone does not name the ADT collection -- a PROG may
// be an include (/programs/includes), a TABL a structure or an append
// (/ddic/structures) -- so the object is looked up by its exact name, of
// any ADT type (an ADT type filter would expand PROG to PROG/P and TABL to
// TABL/DT, and miss exactly those), and the hit whose main type is the
// TADIR type gives the URI.
//
// The hit must be in pkg. One in another package means the object moved
// after the package was read: that is an error, and the object is not
// deleted -- the static address would reach it in its new package, and
// DeleteObject's gate checks only the whitelist, not this package. A search
// that fails, comes back incomplete, or returns a hit without a package is
// an error too: the package cannot be confirmed. Only when
// the search finds no object of that type at all is the collection the
// type usually lives in used (a DELETE there of something absent fails).
func (c *Client) gitDeleteURL(ctx context.Context, objType, name, pkg string) (string, error) {
	objType = strings.ToUpper(objType)
	fallback, ok := GitObjectURL(objType, name)
	if !ok {
		return "", fmt.Errorf("no ADT delete for type %s here; delete it in SE80", objType)
	}
	hits, incomplete, err := c.SearchObjectExact(ctx, name, "", 0)
	if err != nil {
		return "", fmt.Errorf("its ADT address could not be looked up, so its package could not be confirmed: %w; not deleted", err)
	}
	if incomplete != "" {
		// The window filled: a hit of this type may be past it, so "none
		// found" cannot pick the fallback.
		return "", fmt.Errorf("the search for its ADT address may be incomplete, so its package could not be confirmed; not deleted")
	}
	found := ""
	for _, h := range hits {
		if !gitHitIsObject(objType, h.Type) {
			continue
		}
		hp := strings.TrimSpace(h.PackageName)
		if hp == "" {
			// No package on the hit: it cannot be shown to be still in pkg.
			return "", fmt.Errorf("the search did not say which package it is in, so that could not be confirmed; not deleted")
		}
		if !strings.EqualFold(hp, pkg) {
			return "", fmt.Errorf("it is in package %s now, not %s (it moved after the package was read); not deleted", strings.ToUpper(hp), pkg)
		}
		u := strings.TrimSpace(h.URI)
		if found == "" && strings.HasPrefix(u, "/sap/bc/adt/") && !strings.ContainsAny(u, "?#") {
			found = u
		}
	}
	if found != "" {
		return found, nil
	}
	return fallback, nil
}

// DeleteGitObjects deletes exactly the given objects of pkg, then -- only
// when deleteRepo is set -- the offline abapGit repository registered for
// pkg, then pkg itself if nothing is left in it and no repository is
// registered for it any more.
//
// An object is deleted only when the package's TADIR has it (R3TR, devclass
// pkg exactly); anything else is skipped and said so. A package is never an
// item: pkg goes last, only empty, and a subpackage never. Every delete goes
// through DeleteObject's gate. When an object cannot be deleted, the
// repository and the package are left alone.
//
// A repository registration (URL, branch, settings) is dropped only on an
// explicit deleteRepo, only for an offline repository, and only once the
// package is empty; ZCL_VSP_GIT_SERVICE checks the same again. An online
// repository is never unregistered: deleteRepo with one is refused before
// anything is deleted.
func (c *Client) DeleteGitObjects(ctx context.Context, ws GitService, pkg string, items []GitDeleteItem, transport string, deleteRepo bool) (*GitDeleteResult, error) {
	return c.DeleteGitObjectsWith(ctx, ws, pkg, items, GitDeleteOptions{Transport: transport, DeleteRepo: deleteRepo})
}

// DeleteGitObjectsWith is DeleteGitObjects with its options, the expected
// repository among them. An item with an Expect is deleted only when, read
// under the ADT lock its DELETE uses, it is still that version; otherwise it
// comes back changed, with what was observed, and is kept, and the
// repository and the package are left alone. With ExpectRepo the
// repository row is dropped only when it is exactly that row.
//
// Objects are deleted users before what they use (GitDeleteRank), in the
// order given within a rank, or exactly in the order given with KeepOrder;
// the result's Order says which. Each expect is read right before its own
// DELETE, so after the deletes ahead of it: a sha256 read before the call is
// of the state before all of them, and deleting a data element before the
// table that uses it would change the table's sha256. Every TABL shares a
// rank: an append structure is not ordered before the table it extends.
func (c *Client) DeleteGitObjectsWith(ctx context.Context, ws GitService, pkg string, items []GitDeleteItem, opts GitDeleteOptions) (*GitDeleteResult, error) {
	transport := strings.ToUpper(strings.TrimSpace(opts.Transport))
	deleteRepo := opts.DeleteRepo
	if err := c.CheckGitDelete(pkg, transport); err != nil {
		return nil, err
	}
	p, _ := NormalizeGitPackage(pkg)
	if len(items) == 0 {
		return nil, errors.New("no objects to delete")
	}
	expectRepo, err := opts.ExpectRepo.normalized(deleteRepo)
	if err != nil {
		return nil, err
	}
	// Types and names in upper case, once, here: every check below compares
	// them exactly (a "devc" item must meet the DEVC exclusion), whatever a
	// direct caller passed. Every expectation is checked here too, before
	// anything is read: one that cannot be checked refuses the call.
	norm := make([]GitDeleteItem, 0, len(items))
	for _, it := range items {
		n := GitDeleteItem{Type: strings.ToUpper(strings.TrimSpace(it.Type)), Name: strings.ToUpper(strings.TrimSpace(it.Name))}
		if it.Expect != nil {
			exp, err := it.Expect.normalized(n.Type)
			if err != nil {
				return nil, fmt.Errorf("object %s %s: %w", n.Type, n.Name, err)
			}
			n.Expect = exp
		}
		norm = append(norm, n)
	}
	items = norm
	contents, err := c.GitPackageObjects(ctx, ws, p)
	if err != nil {
		return nil, err
	}
	if !contents.Exists {
		return nil, fmt.Errorf("package %s does not exist", p)
	}
	if contents.Truncated {
		return nil, fmt.Errorf("package %s has more TADIR rows than one read returns; delete in SE80", p)
	}
	if deleteRepo && contents.Repo != nil && !contents.Repo.Offline {
		what := "online abapGit repository"
		if contents.Repo.State == GitRepoUnknown {
			what = "an abapGit repository abapGit could not open (state unknown, treated as online),"
		}
		return nil, fmt.Errorf("package %s has %s %s %q: vsp never unregisters an online repository (its URL, branch and settings); "+
			"remove it in abapGit if that is wanted, and call again without delete_repo. Nothing was deleted", p, what, contents.Repo.Key, contents.Repo.Name)
	}
	res := &GitDeleteResult{Package: p, Repo: contents.Repo}

	type todo struct {
		i    int
		url  string
		item GitDeleteItem
	}
	var queue []todo
	for _, it := range items {
		o := GitDeleteOutcome{Type: it.Type, Name: it.Name}
		_, ok := GitObjectURL(it.Type, it.Name)
		switch {
		case it.Type == "DEVC":
			o.Status, o.Reason = "skipped", "a package is not deleted as an item: the package itself goes last, and only when empty; a subpackage never"
		case !contents.Has(it.Type, it.Name):
			o.Status, o.Reason = "skipped", fmt.Sprintf("not in package %s (TADIR); nothing outside the package is deleted", p)
		case !ok:
			o.Status, o.Reason = "failed", fmt.Sprintf("no ADT delete for type %s here; delete it in SE80", it.Type)
		default:
			// In the package's TADIR: now its real ADT address, still in
			// this package.
			u, uerr := c.gitDeleteURL(ctx, it.Type, it.Name, p)
			if uerr != nil {
				o.Status, o.Reason = "failed", uerr.Error()
				break
			}
			queue = append(queue, todo{len(res.Objects), u, it})
		}
		res.Objects = append(res.Objects, o)
	}
	// Users before what they use; within a type rank, the order given. Each
	// object's expect is read under its own lock, right before its own
	// DELETE: after the deletes before it in this order.
	if !opts.KeepOrder {
		sort.SliceStable(queue, func(a, b int) bool {
			return GitDeleteRank(queue[a].item.Type) < GitDeleteRank(queue[b].item.Type)
		})
	}
	res.Order = make([]string, 0, len(queue))
	// Two rounds: an object another one uses may go only after it.
	for round := 0; round < 2 && len(queue) > 0; round++ {
		var again []todo
		for _, q := range queue {
			res.Order = append(res.Order, q.item.Type+" "+q.item.Name)
			out := &res.Objects[q.i]
			var check func(context.Context) error
			if q.item.Expect != nil {
				check = func(context.Context) error {
					obs, cerr := c.checkGitExpect(ctx, ws, p, q.item)
					out.Observed = obs
					return cerr
				}
			}
			note, retry, derr := c.deleteGatedChecked(ctx, q.url, transport, check)
			var changed *GitChangedError
			switch {
			case errors.As(derr, &changed):
				out.Status, out.Reason = "changed", derr.Error()
			case derr != nil:
				out.Status, out.Reason = "failed", derr.Error()
				if retry {
					again = append(again, q)
				}
			default:
				out.Status, out.Reason = "deleted", note
			}
		}
		queue = again
	}
	for _, o := range res.Objects {
		if o.Status == "failed" || o.Status == "changed" {
			res.RepoNote = "kept: not every object could be deleted"
			res.PackageNote = "kept: not every object could be deleted"
			if o.Status == "changed" {
				return res, fmt.Errorf("%s %s was kept: %s", o.Type, o.Name, o.Reason)
			}
			return res, fmt.Errorf("%s %s could not be deleted: %s", o.Type, o.Name, o.Reason)
		}
	}

	// What is left decides about the repository and the package.
	after, err := c.GitPackageObjects(ctx, ws, p)
	if err != nil {
		res.RepoNote = "kept: the package's contents could not be read again"
		res.PackageNote = "kept: its contents could not be read again: " + err.Error()
		return res, err
	}
	rest := after.Remaining()
	empty := len(rest) == 0 && !after.Truncated && after.Exists
	res.Remaining = rest
	repo := after.Repo
	switch {
	case repo == nil:
		res.RepoNote = "no abapGit repository is registered for " + p
	case repo.State == GitRepoUnknown:
		res.RepoNote = fmt.Sprintf("kept: abapGit could not open repository %s (state unknown, treated as online); vsp never unregisters one", repo.Key)
	case !repo.Offline:
		res.RepoNote = fmt.Sprintf("kept: %s is an online repository; vsp never unregisters one", repo.Key)
	case !deleteRepo:
		res.RepoNote = fmt.Sprintf("kept: repository %s stays registered (delete_repo: true, or --delete-repo, unregisters an offline repository once the package is empty)", repo.Key)
	case !empty:
		res.RepoNote = fmt.Sprintf("kept: %d object(s) remain in the package; an offline repository is unregistered only from an empty package", len(rest))
	case expectRepo != nil && !expectRepo.matches(repo):
		res.RepoNote = fmt.Sprintf("kept: registered repository is %s %s", repo.Key, repo.Name)
	default:
		var repoAnswer struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		params := map[string]any{"package": p, "key": repo.Key}
		if expectRepo != nil {
			// ZADT_VSP checks the same row again, by key and name, in the
			// step that deletes it.
			params["key"], params["name"] = expectRepo.Key, expectRepo.Name
		}
		err = gitCall(ctx, ws, "delete_repo", params, time.Minute, &repoAnswer)
		var se *GitServiceError
		switch {
		case err != nil && errors.As(err, &se) && (se.Code == "REPO_KEY_MISMATCH" || se.Code == "REPO_NAME_MISMATCH"):
			res.RepoNote = "kept: registered repository is not the expected one: " + se.Message
			res.PackageNote = "kept: an abapGit repository is registered for it"
			return res, nil
		case err == nil:
			res.RepoDeleted = true
			res.Repo = &GitRepoInfo{Key: strings.TrimSpace(repoAnswer.Key), Name: strings.TrimSpace(repoAnswer.Name), Offline: true}
			repo = nil
		case errors.As(err, &se) && se.Code == "REPO_NOT_FOUND":
			res.RepoNote = "no abapGit repository is registered for " + p
			repo = nil
		default:
			res.RepoNote = "not deleted: " + err.Error()
			res.PackageNote = "kept: its repository could not be unregistered"
			return res, err
		}
	}

	// The package, if nothing is left in it and no repository points at it.
	switch {
	case !after.Exists:
		res.PackageNote = "already gone"
		return res, nil
	case !empty:
		res.PackageNote = fmt.Sprintf("kept: %d object(s) remain in it", len(rest))
		return res, nil
	case repo != nil:
		res.PackageNote = fmt.Sprintf("kept: abapGit repository %s is still registered for it", repo.Key)
		return res, nil
	}
	// Once more, just before: a package is never deleted while ZADT_VSP
	// sees a repository row for it, or anything in it.
	last, err := c.GitPackageObjects(ctx, ws, p)
	if err != nil {
		res.PackageNote = "kept: its contents could not be read again: " + err.Error()
		return res, err
	}
	if last.Repo != nil || len(last.Remaining()) > 0 || last.Truncated || !last.Exists {
		res.PackageNote = "kept: it is no longer empty, or an abapGit repository is registered for it"
		if last.Repo != nil {
			res.PackageNote = fmt.Sprintf("kept: abapGit repository %s is registered for it", last.Repo.Key)
		}
		return res, nil
	}
	note, _, err := c.deleteGated(ctx, GetObjectURL(ObjectTypePackage, p, ""), transport)
	if err != nil {
		res.PackageNote = "not deleted: " + err.Error()
		return res, err
	}
	res.PackageDeleted, res.PackageNote = true, note
	return res, nil
}
