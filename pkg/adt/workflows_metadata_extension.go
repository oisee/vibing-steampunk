package adt

import (
	"context"
	"fmt"
	"strings"
)

// WriteMetadataExtensionOptions configures WriteMetadataExtension behavior.
type WriteMetadataExtensionOptions struct {
	Mode        WriteSourceMode // update, create, upsert (default: upsert)
	Description string          // Object description (for create)
	Package     string          // Package name (for create)
	Transport   string          // Transport request number
}

// WriteMetadataExtensionResult represents the result of a metadata extension write.
type WriteMetadataExtensionResult struct {
	Success      bool                `json:"success"`
	ObjectType   string              `json:"objectType"`
	ObjectName   string              `json:"objectName"`
	ObjectURL    string              `json:"objectUrl"`
	Mode         string              `json:"mode"` // "created" or "updated"
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// WriteMetadataExtension creates or updates a CDS metadata extension (DDLX/EX).
func (c *Client) WriteMetadataExtension(ctx context.Context, name, source string, opts *WriteMetadataExtensionOptions) (*WriteMetadataExtensionResult, error) {
	if err := c.checkSafety(OpWorkflow, "WriteMetadataExtension"); err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &WriteMetadataExtensionOptions{Mode: WriteModeUpsert}
	}
	if opts.Mode == "" {
		opts.Mode = WriteModeUpsert
	}

	if err := c.checkTransportableEdit(opts.Transport, "WriteMetadataExtension"); err != nil {
		return nil, err
	}

	name = strings.ToUpper(name)
	result := &WriteMetadataExtensionResult{
		ObjectType: "DDLX",
		ObjectName: name,
	}

	_, err := c.GetDDLX(ctx, name)
	objectExists := err == nil

	actualMode := opts.Mode
	if opts.Mode == WriteModeUpsert {
		if objectExists {
			actualMode = WriteModeUpdate
		} else {
			actualMode = WriteModeCreate
		}
	}

	if actualMode == WriteModeCreate && objectExists {
		result.Message = fmt.Sprintf("Metadata extension %s already exists (use mode=update or mode=upsert)", name)
		return result, nil
	}
	if actualMode == WriteModeUpdate && !objectExists {
		result.Message = fmt.Sprintf("Metadata extension %s does not exist (use mode=create or mode=upsert)", name)
		return result, nil
	}

	objectURL := GetObjectURL(ObjectTypeDDLX, name, "")
	sourceURL := objectURL + "/source/main"
	result.ObjectURL = objectURL

	if actualMode == WriteModeCreate {
		result.Mode = "created"
		if opts.Package == "" {
			result.Message = "package is required for creating new metadata extensions"
			return result, nil
		}
		if opts.Description == "" {
			result.Message = "description is required for creating new metadata extensions"
			return result, nil
		}

		err := c.CreateObject(ctx, CreateObjectOptions{
			ObjectType:     ObjectTypeDDLX,
			Name:           name,
			Description:    opts.Description,
			PackageName:    opts.Package,
			Transport:      opts.Transport,
			MasterLanguage: c.config.Language,
		})
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create metadata extension: %v", err)
			return result, nil
		}
		// CreateObject gated the package it was given; the writes under the
		// lock must not resolve it again (issue #91).
		ctx = withMutationPackageChecked(ctx, objectURL)
	} else {
		result.Mode = "updated"
		// Resolve and approve the package before the LOCK, not under it.
		gated, err := c.gateAndMark(ctx, MutationContext{
			Op:        OpUpdate,
			OpName:    "WriteMetadataExtension",
			ObjectURL: objectURL,
			Transport: opts.Transport,
		})
		if err != nil {
			return nil, err
		}
		ctx = gated
	}

	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock metadata extension: %v", err)
		return result, nil
	}

	defer func() {
		if !result.Success {
			_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, opts.Transport)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to update metadata extension source: %v", err)
		return result, nil
	}

	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock metadata extension: %v", err)
		return result, nil
	}

	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate metadata extension: %v", err)
		result.Activation = activation
		return result, nil
	}

	result.Activation = activation
	if activation.Success {
		result.Success = true
		switch result.Mode {
		case "created":
			result.Message = "Metadata extension created and activated successfully"
		default:
			result.Message = "Metadata extension updated and activated successfully"
		}
	} else {
		result.Message = "Activation failed - check activation messages"
	}

	return result, nil
}
