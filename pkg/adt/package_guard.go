package adt

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// checkSafety checks if an operation is allowed by the safety configuration.
func (c *Client) checkSafety(op OperationType, opName string) error {
	return c.config.Safety.CheckOperation(op, opName)
}

// checkPackageSafety checks if operations on a package are allowed.
func (c *Client) checkPackageSafety(pkg string) error {
	return c.config.Safety.CheckPackage(pkg)
}

// checkObjectPackageSafety resolves the package for an existing object and
// validates it against the configured package whitelist. inLockSession sends
// the lookup in the session of the lock the calling write already holds (see
// getObjectPackage).
func (c *Client) checkObjectPackageSafety(ctx context.Context, objectURL string, inLockSession bool) error {
	if len(c.config.Safety.AllowedPackages) == 0 {
		return nil
	}

	pkg, err := c.getObjectPackage(ctx, objectURL, inLockSession)
	if err != nil {
		return fmt.Errorf("resolving package for %s: %w", normalizeObjectURLForPackageCheck(objectURL), err)
	}

	return c.checkPackageSafety(pkg)
}

// CheckObjectPackageByName checks the package an existing object is in
// against the configured package whitelist, for a caller that knows the
// object by its TADIR type and name rather than by an ADT URL (MoveObject,
// which reassigns the package through ZADT_VSP). It resolves the package the
// way checkObjectPackageSafety does, through the repository search, and fails
// closed when no hit of that type and name carries a package. Without a
// whitelist it does nothing and sends nothing.
func (c *Client) CheckObjectPackageByName(ctx context.Context, objectType, name string) error {
	if len(c.config.Safety.AllowedPackages) == 0 {
		return nil
	}
	objectType = strings.ToUpper(strings.TrimSpace(objectType))
	name = strings.ToUpper(strings.TrimSpace(name))
	results, err := c.SearchObjectByType(ctx, name, objectType, 50)
	if err != nil {
		return fmt.Errorf("resolving package for %s %s: %w", objectType, name, err)
	}
	for _, r := range results {
		kind, _, _ := strings.Cut(strings.ToUpper(r.Type), "/")
		if strings.EqualFold(r.Name, name) && kind == objectType && r.PackageName != "" {
			return c.checkPackageSafety(r.PackageName)
		}
	}
	return fmt.Errorf("resolving package for %s %s: package metadata not found", objectType, name)
}

// checkTransportableEdit checks if editing objects that require transports is allowed.
func (c *Client) checkTransportableEdit(transport, opName string) error {
	return c.config.Safety.CheckTransportableEdit(transport, opName)
}

func (c *Client) getObjectPackage(ctx context.Context, objectURL string, inLockSession bool) (string, error) {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	objectName, err := objectNameFromURL(normalized)
	if err != nil {
		return "", err
	}

	// inLockSession is set when the write that runs this lookup carries a
	// caller-supplied lock handle (MutationContext.LockHandle): its gate then
	// runs after the LOCK, and the lookup is sent stateful so it joins that
	// lock's context instead of relying on stateless-request isolation to leave
	// the context alone. It follows the handle, not the age of any lock record,
	// so neither a long-held lock nor a stale unrelated record changes it.
	results, err := c.searchObjectByType(ctx, objectName, "", 20, inLockSession)
	if err != nil {
		return "", err
	}

	canonicalURL := canonicalizeObjectURL(normalized)
	for _, result := range results {
		if result.PackageName == "" {
			continue
		}
		if canonicalizeObjectURL(result.URI) == canonicalURL {
			return result.PackageName, nil
		}
	}

	return "", fmt.Errorf("package metadata not found")
}

func normalizeObjectURLForPackageCheck(objectURL string) string {
	normalized := strings.TrimSuffix(objectURL, "/")

	if strings.HasSuffix(normalized, "/source/main") {
		normalized = strings.TrimSuffix(normalized, "/source/main")
	}

	// Strip /includes/... only for class sub-resources (e.g. /oo/classes/ZCL_FOO/includes/locals_def).
	// Program includes use /programs/includes/NAME where /includes/ is the collection path — don't strip.
	if idx := strings.Index(normalized, "/includes/"); idx >= 0 {
		prefix := normalized[:idx]
		if !strings.HasSuffix(prefix, "/programs") {
			return prefix
		}
	}

	return normalized
}

func canonicalizeObjectURL(objectURL string) string {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	if decoded, err := url.PathUnescape(normalized); err == nil {
		normalized = decoded
	}
	return strings.ToLower(strings.TrimSuffix(normalized, "/"))
}

func objectNameFromURL(objectURL string) (string, error) {
	normalized := normalizeObjectURLForPackageCheck(objectURL)
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid object URL")
	}

	name, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		return "", fmt.Errorf("decoding object name: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("invalid object URL")
	}

	return strings.ToUpper(name), nil
}

// AllowPackageTemporarily adds a package to the allowed list for the duration of
// an install/bootstrap operation. Returns a cleanup function that removes it.
// This is used by install tools (InstallZADTVSP, InstallAbapGit) which are
// self-contained bootstrap operations that should not be blocked by
// SAP_ALLOWED_PACKAGES restrictions.
func (c *Client) AllowPackageTemporarily(pkg string) func() {
	// If no package restrictions are configured, nothing to do
	if len(c.config.Safety.AllowedPackages) == 0 {
		return func() {}
	}

	// If already allowed, nothing to do
	if c.config.Safety.IsPackageAllowed(pkg) {
		return func() {}
	}

	// Add to allowed packages
	c.config.Safety.AllowedPackages = append(c.config.Safety.AllowedPackages, pkg)

	// Return cleanup function
	return func() {
		// Remove the temporarily added package
		for i, p := range c.config.Safety.AllowedPackages {
			if strings.EqualFold(p, pkg) {
				c.config.Safety.AllowedPackages = append(
					c.config.Safety.AllowedPackages[:i],
					c.config.Safety.AllowedPackages[i+1:]...,
				)
				return
			}
		}
	}
}
