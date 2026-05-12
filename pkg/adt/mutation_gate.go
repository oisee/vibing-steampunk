package adt

import (
	"context"
	"fmt"
)

// MutationSurface identifies the object surface a mutation targets. Different
// surfaces require different metadata resolution strategies (ADT SearchObject,
// UI5 BSP metadata, etc.). Use SurfaceADT for standard ABAP objects.
type MutationSurface int

const (
	// SurfaceADT is the default ADT object surface (classes, programs,
	// interfaces, data elements, message classes, etc.). Package resolution
	// for existing objects uses SearchObject.
	SurfaceADT MutationSurface = iota

	// SurfaceUI5 is the UI5/BSP filestore surface. Package resolution for
	// existing UI5 apps is not yet implemented — mutations on this surface
	// are blocked when AllowedPackages is configured until app→package
	// resolution lands.
	SurfaceUI5
)

// MutationContext describes a single mutation operation for policy evaluation.
// Callers should build a MutationContext at the top of every mutating public
// method and pass it to checkMutation before performing any side effects.
//
// There are two ways to provide the target package:
//   - For **existing** objects: set ObjectURL and leave Package empty. The
//     gate resolves the package from the object metadata.
//   - For **create** operations: set Package explicitly (and optionally
//     ObjectURL, but it will not be resolved).
type MutationContext struct {
	// Op is the operation type used for safety whitelist/blacklist checks
	// (OpCreate, OpUpdate, OpDelete, OpActivate, OpWorkflow, ...).
	Op OperationType

	// OpName is a human-readable name of the operation, used in error
	// messages ("EditSource", "DeleteObject", ...).
	OpName string

	// ObjectURL is the ADT URL of an existing object being mutated. When
	// AllowedPackages is configured and Package is empty, the gate resolves
	// the object's package via this URL.
	ObjectURL string

	// Package is an explicit target package, used for create operations
	// where the package is a caller-supplied parameter.
	Package string

	// Transport is the transport request number supplied by the caller
	// (empty for local objects).
	Transport string

	// Surface selects the package-resolution strategy. Defaults to SurfaceADT.
	Surface MutationSurface
}

// checkMutation runs all policy checks for a mutation operation in a single
// place. It performs (in order):
//
//  1. Operation-type safety check (read-only, allowed/disallowed ops)
//  2. Package ownership check (resolves from ObjectURL for existing objects,
//     or uses explicit Package for creates)
//  3. Transportable-edit check (when a transport is supplied)
//
// This is the single source of truth for mutation policy. Individual mutators
// should call this at the top of their implementation instead of wiring the
// sub-checks by hand — that avoids the class of bug where one sub-check is
// forgotten and a whole mutation path silently bypasses policy.
func (c *Client) checkMutation(ctx context.Context, m MutationContext) error {
	// 1. Operation type check
	if err := c.checkSafety(m.Op, m.OpName); err != nil {
		return err
	}

	// 2. Package ownership check
	if err := c.checkMutationPackage(ctx, m); err != nil {
		return err
	}

	// 3. Transportable-edit check
	if err := c.checkTransportableEdit(m.Transport, m.OpName); err != nil {
		return err
	}

	return nil
}

// checkMutationPackage validates the target package for a mutation. If no
// package whitelist is configured, the check is a no-op.
func (c *Client) checkMutationPackage(ctx context.Context, m MutationContext) error {
	if len(c.config.Safety.AllowedPackages) == 0 {
		return nil
	}

	// If the caller supplied an explicit package (create path), check it
	// directly.
	if m.Package != "" {
		return c.checkPackageSafety(m.Package)
	}

	// Otherwise resolve the package from the existing object.
	if m.ObjectURL == "" {
		return fmt.Errorf("mutation gate: %s requires either ObjectURL or Package when AllowedPackages is configured", m.OpName)
	}

	switch m.Surface {
	case SurfaceADT:
		return c.checkObjectPackageSafety(ctx, m.ObjectURL)

	case SurfaceUI5:
		// UI5 app→package resolution is not yet implemented. Fail closed
		// when a package whitelist is configured so that UI5 mutations do
		// not silently bypass policy.
		return fmt.Errorf(
			"operation '%s' on UI5 surface is blocked: UI5 app→package resolution not yet implemented, cannot verify package against SAP_ALLOWED_PACKAGES (tracked as follow-up)",
			m.OpName)

	default:
		return fmt.Errorf("mutation gate: unknown surface %d for %s", m.Surface, m.OpName)
	}
}
