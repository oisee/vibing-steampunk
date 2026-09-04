package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// Spool requests and background jobs, for the post-mortem that starts with
// "the night job failed": which jobs, which steps, what they printed, what
// the log said. Listing and spool content come over free SQL; the job log,
// and spool a system keeps in files, come over RFC through XBP.

const (
	noteSpoolInFiles = "This spool request's content is kept in a file on the application server, not in TST03; pass via=\"rfc\" to read it through XBP."
	noteJobLogViaRFC = "Job logs are TemSe objects most systems keep in files, so this is read over RFC through XBP; the system needs a reachable gateway."
)

type spoolListResult struct {
	Requests []adt.SpoolRequest `json:"requests"`
	Count    int                `json:"count"`
	Notes    []string           `json:"notes,omitempty"`
}

func (s *Server) handleSpoolList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	filter := adt.SpoolFilter{
		Owner:   firstString(args, "user", "owner"),
		Title:   firstString(args, "title"),
		Program: firstString(args, "program"),
		Job:     firstString(args, "job", "job_name"),
	}
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		filter.Limit = int(n)
	}
	var err error
	if filter.From, err = boundFrom(args, false, "from", "since", "date_from"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if filter.To, err = boundFrom(args, true, "to", "until", "date_to"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	reqs, err := s.adtClient.SpoolRequests(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list spool requests: %v", err)), nil
	}
	if reqs == nil {
		reqs = []adt.SpoolRequest{}
	}
	var notes []string
	for _, r := range reqs {
		if r.Storage != "" && r.Storage != "D" {
			notes = []string{noteSpoolInFiles}
			break
		}
	}
	return newToolResultJSON(spoolListResult{Requests: reqs, Count: len(reqs), Notes: notes}), nil
}

type spoolReadResult struct {
	Content *adt.SpoolContent `json:"content,omitempty"`
	Number  int               `json:"number"`
	Via     string            `json:"via"`
	Lines   []string          `json:"lines,omitempty"`
	Notes   []string          `json:"notes,omitempty"`
}

func (s *Server) handleSpoolRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	number := 0
	if n, ok := firstNumber(args, "number", "spool", "rqident"); ok {
		number = int(n)
	} else if v := firstString(args, "number", "spool", "rqident"); v != "" {
		number, _ = strconv.Atoi(strings.TrimSpace(v))
	}
	if number <= 0 {
		return newToolResultError("spool_read needs number: the spool request number"), nil
	}
	via := strings.ToLower(firstString(args, "via"))
	if via != "rfc" {
		content, err := s.adtClient.Spool(ctx, number)
		if err == nil {
			return newToolResultJSON(spoolReadResult{Content: content, Number: number, Via: "adt"}), nil
		}
		if content == nil || content.Request.Storage == "" || content.Request.Storage == "D" || via == "adt" {
			return newToolResultError(fmt.Sprintf("Failed to read spool request %d: %v", number, err)), nil
		}
	}
	c, err := s.dialRFC(ctx, args)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read spool request %d over RFC: %v", number, err)), nil
	}
	defer c.Close(ctx)
	lines, err := saprfc.ReadSpoolRequest(ctx, c, number)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read spool request %d over RFC: %v", number, err)), nil
	}
	return newToolResultJSON(spoolReadResult{Number: number, Via: "rfc", Lines: lines}), nil
}

type jobListResult struct {
	Jobs  []adt.Job `json:"jobs"`
	Count int       `json:"count"`
}

func (s *Server) handleJobList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	filter := adt.JobFilter{
		Name:    firstString(args, "name", "job", "job_name"),
		User:    firstString(args, "user"),
		Status:  firstString(args, "status"),
		Program: firstString(args, "program"),
	}
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		filter.Limit = int(n)
	}
	var err error
	if filter.From, err = boundFrom(args, false, "from", "since", "date_from"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if filter.To, err = boundFrom(args, true, "to", "until", "date_to"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	jobs, err := s.adtClient.Jobs(ctx, filter)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
	}
	if jobs == nil {
		jobs = []adt.Job{}
	}
	return newToolResultJSON(jobListResult{Jobs: jobs, Count: len(jobs)}), nil
}

type jobLogResult struct {
	Job     string               `json:"job"`
	Count   string               `json:"count"`
	Entries []saprfc.JobLogEntry `json:"entries"`
	Notes   []string             `json:"notes,omitempty"`
}

func (s *Server) handleJobLog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name := strings.ToUpper(firstString(args, "job", "name", "job_name"))
	count := firstString(args, "count", "job_count", "jobcount")
	if n, ok := firstNumber(args, "count", "job_count", "jobcount"); ok && count == "" {
		count = fmt.Sprintf("%08d", int(n))
	}
	if name == "" || count == "" {
		return newToolResultError("job_log needs job (the name) and count (the job count from job_list)"), nil
	}
	c, err := s.dialRFC(ctx, args)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read the log of %s/%s: %v", name, count, err)), nil
	}
	defer c.Close(ctx)
	entries, err := saprfc.ReadJobLog(ctx, c, name, count)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read the log of %s/%s: %v", name, count, err)), nil
	}
	return newToolResultJSON(jobLogResult{Job: name, Count: count, Entries: entries, Notes: []string{noteJobLogViaRFC}}), nil
}
