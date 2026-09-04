package adt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Background jobs are two tables: TBTCO, one row per job with its status
// and times, and TBTCP, one row per step with the program, the variant and
// the spool the step wrote. The job log is a TemSe object too, but one that
// most systems keep in files, so it is read over RFC (pkg/saprfc) and not
// here.

// Job is one TBTCO row with its steps.
type Job struct {
	Name   string `json:"name"`
	Count  string `json:"count"`
	Status string `json:"status"`
	// StatusText spells the one-letter status out.
	StatusText string    `json:"statusText"`
	Scheduled  time.Time `json:"scheduled,omitempty"`
	Released   time.Time `json:"released,omitempty"`
	Started    time.Time `json:"started,omitempty"`
	Ended      time.Time `json:"ended,omitempty"`
	// Duration is Ended minus Started for a job that ran.
	Duration string `json:"duration,omitempty"`
	User     string `json:"user,omitempty"`
	Server   string `json:"server,omitempty"`
	Periodic bool   `json:"periodic,omitempty"`
	Class    string `json:"class,omitempty"`
	// Log is the TemSe name of the job log.
	Log   string    `json:"log,omitempty"`
	Steps []JobStep `json:"steps,omitempty"`
}

// JobStep is one TBTCP row.
type JobStep struct {
	Step    int    `json:"step"`
	Program string `json:"program,omitempty"`
	Variant string `json:"variant,omitempty"`
	User    string `json:"user,omitempty"`
	Lang    string `json:"language,omitempty"`
	Status  string `json:"status,omitempty"`
	// Spool is the number of the spool request the step wrote, 0 for none.
	Spool int `json:"spool,omitempty"`
	// External is an external command or program step.
	External string `json:"external,omitempty"`
}

var jobStatusText = map[string]string{
	"P": "scheduled", "S": "released", "Y": "ready", "R": "active", "F": "finished", "A": "cancelled", "Z": "put active",
}

// JobFilter narrows a listing.
type JobFilter struct {
	Name    string // exact, or a LIKE pattern with * wildcards
	User    string
	Status  string // one letter, or several
	Program string // only jobs with a step running this program
	From    time.Time
	To      time.Time
	Limit   int
}

// Jobs lists background jobs, latest start first, with their steps.
func (c *Client) Jobs(ctx context.Context, filter JobFilter) ([]Job, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	var terms []string
	if v := strings.ToUpper(strings.TrimSpace(filter.Name)); v != "" {
		if strings.ContainsAny(v, "*%") {
			terms = append(terms, "jobname LIKE '"+sqlQuote(strings.ReplaceAll(v, "*", "%"))+"'")
		} else {
			terms = append(terms, "jobname = '"+sqlQuote(v)+"'")
		}
	}
	if v := strings.ToUpper(strings.TrimSpace(filter.User)); v != "" {
		terms = append(terms, "sdluname = '"+sqlQuote(v)+"'")
	}
	if v := strings.ToUpper(strings.TrimSpace(filter.Status)); v != "" {
		var codes []string
		for _, ch := range v {
			if ch != ',' && ch != ' ' {
				codes = append(codes, "'"+string(ch)+"'")
			}
		}
		terms = append(terms, "status IN ( "+strings.Join(codes, ", ")+" )")
	}
	if !filter.From.IsZero() {
		terms = append(terms, "sdldate >= '"+filter.From.Format("20060102")+"'")
	}
	if !filter.To.IsZero() {
		terms = append(terms, "sdldate <= '"+filter.To.Format("20060102")+"'")
	}
	if v := strings.ToUpper(strings.TrimSpace(filter.Program)); v != "" {
		res, err := c.RunQuery(ctx, "SELECT jobname, jobcount FROM tbtcp WHERE progname = '"+sqlQuote(v)+"' ORDER BY sdldate DESCENDING, sdltime DESCENDING", limit*4)
		if err != nil {
			return nil, fmt.Errorf("reading job steps: %w", err)
		}
		if res == nil || len(res.Rows) == 0 {
			return nil, nil
		}
		var keys []string
		for _, row := range res.Rows {
			keys = append(keys, "( jobname = '"+sqlQuote(cell(row, "JOBNAME"))+"' AND jobcount = '"+cell(row, "JOBCOUNT")+"' )")
		}
		terms = append(terms, "(\n"+strings.Join(keys, "\nOR ")+" )")
	}
	query := "SELECT jobname, jobcount, status, sdldate, sdltime, reldate, reltime, strtdate, strttime, enddate, endtime, sdluname, reaxserver, execserver, periodic, jobclass, joblog FROM tbtco"
	if len(terms) > 0 {
		query += " WHERE " + strings.Join(terms, " AND ")
	}
	query += " ORDER BY strtdate DESCENDING, strttime DESCENDING, sdldate DESCENDING, sdltime DESCENDING"
	res, err := c.RunQuery(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("reading jobs: %w", err)
	}
	if res == nil {
		return nil, nil
	}
	var jobs []Job
	var keys []string
	for _, row := range res.Rows {
		j := Job{Name: cell(row, "JOBNAME"), Count: cell(row, "JOBCOUNT"), Status: cell(row, "STATUS"),
			User: cell(row, "SDLUNAME"), Class: cell(row, "JOBCLASS"), Log: cell(row, "JOBLOG"), Periodic: cell(row, "PERIODIC") == "X"}
		j.StatusText = jobStatusText[j.Status]
		j.Server = cell(row, "EXECSERVER")
		if j.Server == "" {
			j.Server = cell(row, "REAXSERVER")
		}
		j.Scheduled = parseSAPStamp(cell(row, "SDLDATE"), cell(row, "SDLTIME"))
		j.Released = parseSAPStamp(cell(row, "RELDATE"), cell(row, "RELTIME"))
		j.Started = parseSAPStamp(cell(row, "STRTDATE"), cell(row, "STRTTIME"))
		j.Ended = parseSAPStamp(cell(row, "ENDDATE"), cell(row, "ENDTIME"))
		if !j.Started.IsZero() && !j.Ended.IsZero() {
			j.Duration = j.Ended.Sub(j.Started).String()
		}
		jobs = append(jobs, j)
		keys = append(keys, "( jobname = '"+sqlQuote(j.Name)+"' AND jobcount = '"+j.Count+"' )")
	}
	if len(jobs) == 0 {
		return jobs, nil
	}
	steps, err := c.RunQuery(ctx, "SELECT jobname, jobcount, stepcount, progname, variant, authcknam, language, status, listident, xpgprog, extcmd FROM tbtcp WHERE (\n"+strings.Join(keys, "\nOR ")+" ) ORDER BY jobname, jobcount, stepcount", len(jobs)*20)
	if err != nil {
		return nil, fmt.Errorf("reading job steps: %w", err)
	}
	if steps != nil {
		index := map[string]int{}
		for i, j := range jobs {
			index[j.Name+"\x00"+j.Count] = i
		}
		for _, row := range steps.Rows {
			i, ok := index[cell(row, "JOBNAME")+"\x00"+cell(row, "JOBCOUNT")]
			if !ok {
				continue
			}
			s := JobStep{Program: cell(row, "PROGNAME"), Variant: cell(row, "VARIANT"), User: cell(row, "AUTHCKNAM"), Lang: cell(row, "LANGUAGE"), Status: cell(row, "STATUS")}
			s.Step, _ = strconv.Atoi(cell(row, "STEPCOUNT"))
			s.Spool, _ = strconv.Atoi(cell(row, "LISTIDENT"))
			if s.Program == "" {
				s.External = strings.TrimSpace(cell(row, "EXTCMD") + " " + cell(row, "XPGPROG"))
			}
			jobs[i].Steps = append(jobs[i].Steps, s)
		}
	}
	return jobs, nil
}
