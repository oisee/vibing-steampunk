package vsp

// Reading many objects' sources, concurrently and reproducibly.
//
// Every package-wide scan in this CLI is dominated by round trips: 167 objects
// took 18.8 seconds against a live 7.58 system, and the parse of all of them is
// milliseconds. Fetching them one at a time is the whole of the cost, and
// fetching them six at a time removes eleven twelfths of it — measured on
// `boundaries`, 18.8 s to 1.6 s, with byte-identical output.
//
// This exists as one function rather than as the same twenty lines in four
// scans. That is not tidiness: a concurrency bug written four times is fixed
// three times, and this codebase spent the week on what happens when two routes
// answer the same question.
//
// The rule that makes it safe to use anywhere:
//
//	**Results come back in the order asked for, never in the order answered.**
//
// A graph assembled in whatever order the network replied differs from run to
// run, and a report that changes without the system changing cannot be diffed
// and should not be trusted. Concurrency belongs to the fetching and must not
// reach the output.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// sourceFetchWorkers is how many reads are in flight at once.
//
// Six, matching what the transport audit settled on independently. More stops
// helping — the far side serialises — and it starts being the kind of load a
// basis administrator asks about, which is a bad way for a read-only analysis
// command to introduce itself.
const sourceFetchWorkers = 6

// sourceRef names one object to read, and optionally which part of it.
//
// Section matters because a class is not one document. A call recorded against
// CL_FOO=========CCAU is in that class's *test* include, and CL_FOO's main
// source does not contain it — reading main answers cleanly, finds nothing, and
// reports "0 of 15" as though that were the answer. Four sections have an
// address of their own; everything else lives in the main source and must not
// be given a path by pattern, because ADT answers 404 to an invented one and
// the object is then filed as unreadable.
type sourceRef struct {
	Type string
	Name string
	// Section is a class include suffix — CCAU, CCIMP — or empty for the
	// object's main source.
	Section string
}

// Address returns a stable identity for what this ref will read, so a caller
// can dedupe by document rather than by object. A class that calls the target
// from both its main source and its test include is two reads, not one, and
// keeping only the first drops half the answer.
func (r sourceRef) Address() string {
	inc, own := adt.ClassIncludeForSection(r.Section)
	if r.Type == "CLAS" && own {
		return "CLAS " + r.Name + " " + string(inc)
	}
	return r.Type + " " + r.Name
}

// sourceResult is what came back for one ref, in the position it was asked in.
type sourceResult struct {
	Ref    sourceRef
	Source string
	Err    error
}

// fetchSources reads every ref concurrently and returns the results in input
// order.
//
// Errors are carried per result rather than returned, because a scan over a
// package must not stop at the first object it cannot read — and, more to the
// point, must be able to say afterwards which ones those were. A caller that
// drops them silently is back to reporting a clean verdict over a package it
// read in part.
func fetchSources(ctx context.Context, client *adt.Client, refs []sourceRef, label string) []sourceResult {
	results := make([]sourceResult, len(refs))
	if len(refs) == 0 {
		return results
	}

	var wg sync.WaitGroup
	var done int64
	sem := make(chan struct{}, sourceFetchWorkers)

	for i, ref := range refs {
		wg.Add(1)
		go func(i int, ref sourceRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var src string
			var err error
			if inc, own := adt.ClassIncludeForSection(ref.Section); own && ref.Type == "CLAS" {
				src, err = client.GetClassInclude(ctx, ref.Name, inc)
			} else {
				src, err = client.GetSource(ctx, ref.Type, ref.Name, nil)
			}
			results[i] = sourceResult{Ref: ref, Source: src, Err: err}

			n := atomic.AddInt64(&done, 1)
			if label != "" {
				fmt.Fprintf(os.Stderr, "\r  %s [%d/%d] %s %-40s", label, n, len(refs), ref.Type, ref.Name)
			}
		}(i, ref)
	}
	wg.Wait()
	if label != "" {
		fmt.Fprintln(os.Stderr)
	}
	return results
}

// unreadable turns the failures into the form every report in this CLI uses to
// say what it could not look at.
//
// Kept beside the fetch so the two cannot drift: a caller that reads the
// results and forgets the failures produces exactly the report this codebase
// has spent the week removing.
func unreadable(results []sourceResult) []adt.Unsearched {
	var missed []adt.Unsearched
	for _, r := range results {
		if r.Err != nil {
			missed = append(missed, adt.Unsearched{
				Object: r.Ref.Type + " " + r.Ref.Name,
				Reason: r.Err.Error(),
			})
		}
	}
	return missed
}

// healthScanCap bounds the boundary signal inside `vsp health`.
//
// It was 50 while reading was serial — fifty round trips is about six seconds,
// and a health signal nobody waits for is a health signal nobody runs. With six
// concurrent reads a 222-object package takes 1.6 seconds, so the cap moves to
// where it stops shaping the answer on any package a person is likely to point
// this at.
//
// It is not removed. A cap that is never reached costs nothing, and it is the
// only thing between this command and a customer package of four thousand
// objects. When it is reached the report says so with the number, because a
// bounded sweep that does not say it was bounded is the clean verdict over a
// package read in part.
const healthScanCap = 400
