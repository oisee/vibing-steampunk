// Package adt provides refactoring tools using the correct ADT REST API.
// Reference: abap-adt-api/src/api/refactor.ts by marcellourbani.
//
// The ADT refactoring API uses a 3-step flow:
//   1. Evaluate — check if refactoring is possible, get affected objects
//   2. Preview — get the changes that would be made
//   3. Execute — apply the changes
//
// All steps use POST /sap/bc/adt/refactorings with ?step= and ?rel= params.
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Types ---

// RenameEvaluation is the response from step=evaluate for rename.
type RenameEvaluation struct {
	XMLName         xml.Name              `xml:"renameRefactoring"`
	OldName         string                `xml:"oldName"`
	NewName         string                `xml:"newName"`
	Refactoring     GenericRefactoring    `xml:"genericRefactoring"`
}

// GenericRefactoring wraps common refactoring metadata.
type GenericRefactoring struct {
	XMLName                   xml.Name          `xml:"genericRefactoring"`
	Title                     string            `xml:"title"`
	AdtObjectURI              string            `xml:"adtObjectUri"`
	AffectedObjects           []AffectedObject  `xml:"affectedObjects>affectedObject"`
	Transport                 string            `xml:"transport"`
	IgnoreSyntaxErrorsAllowed bool              `xml:"ignoreSyntaxErrorsAllowed"`
	IgnoreSyntaxErrors        bool              `xml:"ignoreSyntaxErrors"`
	TextReplaceDeltas         []TextReplaceDelta `xml:"textReplaceDeltas>textReplaceDelta,omitempty"`
}

// AffectedObject is an object affected by a refactoring.
type AffectedObject struct {
	Name      string `xml:"name,attr"`
	ParentURI string `xml:"parentUri,attr"`
	Type      string `xml:"type,attr"`
	URI       string `xml:"uri,attr"`
}

// TextReplaceDelta describes a single text replacement in a source.
type TextReplaceDelta struct {
	RangeFragment string `xml:"rangeFragment"`
	ContentOld    string `xml:"contentOld"`
	ContentNew    string `xml:"contentNew"`
}

// RenameResult wraps the full rename evaluation or preview result.
type RenameResult struct {
	OldName         string            `json:"oldName"`
	NewName         string            `json:"newName"`
	Title           string            `json:"title"`
	AffectedObjects []AffectedObject  `json:"affectedObjects"`
	Transport       string            `json:"transport,omitempty"`
	Deltas          []TextReplaceDelta `json:"deltas,omitempty"`
	Step            string            `json:"step"`
}

// QuickFixProposal is a single fix suggestion from the ADT quickfix API.
type QuickFixProposal struct {
	ID          string `xml:"id" json:"id"`
	Description string `xml:"description" json:"description"`
	URI         string `xml:"uri,attr" json:"uri"`
}

// QuickFixEvaluation is the response from /quickfixes/evaluation.
type QuickFixEvaluation struct {
	XMLName   xml.Name           `xml:"evaluationResults"`
	Proposals []QuickFixProposal `xml:"evaluationResult"`
}

// QuickFixResult wraps the evaluation response for JSON output.
type QuickFixResult struct {
	ObjectURI string             `json:"objectUri"`
	Proposals []QuickFixProposal `json:"proposals"`
	Count     int                `json:"count"`
}

// --- Constants ---

const (
	refactoringEndpoint = "/sap/bc/adt/refactorings"
	quickfixEndpoint    = "/sap/bc/adt/quickfixes/evaluation"

	relRename        = "http://www.sap.com/adt/relations/refactoring/rename"
	relExtractMethod = "http://www.sap.com/adt/relations/refactoring/extractmethod"
)

// --- Rename Refactoring ---

// RenameEvaluate checks if a rename refactoring is possible at the given position.
// objectURI should include fragment: /sap/bc/adt/oo/classes/zcl_foo#start=10,5;end=10,15
func (c *Client) RenameEvaluate(ctx context.Context, objectURI string) (*RenameResult, error) {
	if err := c.checkSafety(OpRead, "RenameEvaluate"); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("step", "evaluate")
	q.Set("rel", relRename)
	q.Set("uri", objectURI)

	resp, err := c.transport.Request(ctx, refactoringEndpoint, &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("rename evaluate failed: %w", err)
	}

	var eval RenameEvaluation
	if err := xml.Unmarshal(resp.Body, &eval); err != nil {
		return nil, fmt.Errorf("parsing rename evaluation: %w", err)
	}

	return &RenameResult{
		OldName:         eval.OldName,
		NewName:         eval.NewName,
		Title:           eval.Refactoring.Title,
		AffectedObjects: eval.Refactoring.AffectedObjects,
		Transport:       eval.Refactoring.Transport,
		Step:            "evaluate",
	}, nil
}

// RenameExecute performs a rename refactoring (evaluate → execute).
// This is a write operation that modifies source code.
func (c *Client) RenameExecute(ctx context.Context, objectURI, newName, transport string) (*RenameResult, error) {
	if err := c.checkSafety(OpUpdate, "RenameExecute"); err != nil {
		return nil, err
	}

	// Step 1: Evaluate
	q := url.Values{}
	q.Set("step", "evaluate")
	q.Set("rel", relRename)
	q.Set("uri", objectURI)

	resp, err := c.transport.Request(ctx, refactoringEndpoint, &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("rename evaluate failed: %w", err)
	}

	var eval RenameEvaluation
	if err := xml.Unmarshal(resp.Body, &eval); err != nil {
		return nil, fmt.Errorf("parsing rename evaluation: %w", err)
	}

	// Build execute request body
	eval.NewName = newName
	if transport != "" {
		eval.Refactoring.Transport = transport
	}

	body, err := xml.MarshalIndent(eval, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("building rename request: %w", err)
	}

	// Step 3: Execute (skip preview for programmatic use)
	q2 := url.Values{}
	q2.Set("step", "execute")

	resp, err = c.transport.Request(ctx, refactoringEndpoint, &RequestOptions{
		Method:      http.MethodPost,
		Query:       q2,
		Body:        body,
		ContentType: "application/xml",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("rename execute failed: %w", err)
	}

	// Parse result to get deltas
	var result RenameEvaluation
	if parseErr := xml.Unmarshal(resp.Body, &result); parseErr != nil {
		// Execute may return non-XML success
		return &RenameResult{
			OldName: eval.OldName,
			NewName: newName,
			Step:    "execute",
		}, nil
	}

	return &RenameResult{
		OldName:         eval.OldName,
		NewName:         newName,
		Title:           result.Refactoring.Title,
		AffectedObjects: result.Refactoring.AffectedObjects,
		Deltas:          result.Refactoring.TextReplaceDeltas,
		Step:            "execute",
	}, nil
}

// --- QuickFix ---

// GetQuickFixProposals returns available quick fix suggestions at the given position.
func (c *Client) GetQuickFixProposals(ctx context.Context, objectURI string, source string) (*QuickFixResult, error) {
	if err := c.checkSafety(OpRead, "GetQuickFixProposals"); err != nil {
		return nil, err
	}

	// Extract base URI and fragment
	baseURI := objectURI
	fragment := ""
	if idx := strings.Index(objectURI, "#"); idx >= 0 {
		baseURI = objectURI[:idx]
		fragment = objectURI[idx+1:]
	}

	q := url.Values{}
	if fragment != "" {
		q.Set("uri", objectURI)
	} else {
		q.Set("uri", baseURI)
	}

	var body []byte
	contentType := "application/*"
	if source != "" {
		body = []byte(source)
		contentType = "text/plain"
	}

	resp, err := c.transport.Request(ctx, quickfixEndpoint, &RequestOptions{
		Method:      http.MethodPost,
		Query:       q,
		Body:        body,
		ContentType: contentType,
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("quickfix evaluation failed: %w", err)
	}

	var eval QuickFixEvaluation
	if err := xml.Unmarshal(resp.Body, &eval); err != nil {
		return nil, fmt.Errorf("parsing quickfix evaluation: %w", err)
	}

	return &QuickFixResult{
		ObjectURI: objectURI,
		Proposals: eval.Proposals,
		Count:     len(eval.Proposals),
	}, nil
}
