// Package install contains fail-closed primitives shared by CLI and MCP
// installers.
package install

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Client is the subset of the ADT client required by installers.
type Client interface {
	PackageExists(context.Context, string) (bool, error)
	CreateObject(context.Context, adt.CreateObjectOptions) error
	WriteSource(context.Context, string, string, string, *adt.WriteSourceOptions) (*adt.WriteSourceResult, error)
	GetSource(context.Context, string, string, *adt.GetSourceOptions) (string, error)
}

// EnsurePackage proves that a package exists, creating and re-probing it only
// after a definite missing result. A create error is tolerated only if a fresh
// probe proves that the package now exists, as can happen with a concurrent
// create.
func EnsurePackage(ctx context.Context, client Client, name, description string) (created bool, err error) {
	exists, err := client.PackageExists(ctx, name)
	if err != nil {
		return false, fmt.Errorf("package existence check was inconclusive: %w", err)
	}
	if exists {
		return false, nil
	}

	createErr := client.CreateObject(ctx, adt.CreateObjectOptions{
		ObjectType:  adt.ObjectTypePackage,
		Name:        name,
		Description: description,
	})
	exists, verifyErr := client.PackageExists(ctx, name)
	if verifyErr != nil {
		if createErr != nil {
			return false, fmt.Errorf("package create failed (%v) and verification was inconclusive: %w", createErr, verifyErr)
		}
		return false, fmt.Errorf("package creation could not be verified: %w", verifyErr)
	}
	if !exists {
		if createErr != nil {
			return false, fmt.Errorf("package create failed and package is still absent: %w", createErr)
		}
		return false, fmt.Errorf("package creation returned success but package is still absent")
	}
	if createErr != nil {
		return false, nil
	}
	return true, nil
}

// DeploySource writes one object, validates the structured workflow result,
// and proves the object can be read back before reporting success.
func DeploySource(ctx context.Context, client Client, objectType, name, source string, opts *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
	result, err := client.WriteSource(ctx, objectType, name, source, opts)
	if err != nil {
		return result, err
	}
	if resultErr := adt.WriteSourceResultError(result); resultErr != nil {
		return result, resultErr
	}
	for _, syntax := range result.SyntaxErrors {
		severity := strings.ToUpper(strings.TrimSpace(syntax.Severity))
		if severity == "E" || severity == "A" || severity == "X" {
			return result, fmt.Errorf("WriteSource reported a syntax error")
		}
	}
	if result.Activation != nil {
		if activationErr := adt.ActivationResultError(result.Activation); activationErr != nil {
			return result, fmt.Errorf("WriteSource reported %w", activationErr)
		}
	}
	readBack, err := client.GetSource(ctx, objectType, name, nil)
	if err != nil {
		return result, fmt.Errorf("source read-back failed: %w", err)
	}
	if strings.TrimSpace(readBack) == "" {
		return result, fmt.Errorf("source read-back was empty")
	}
	return result, nil
}
