package adt

import "context"

// CheckMutation runs the mutation gate for m: the operation type, the
// package (resolved from m.ObjectURL for an existing object, or m.Package as
// given for a create), and the transportable-edit policy. It is the gate every
// built-in write runs, exported for code outside this package -- an extension
// (pkg/mcpext) driving a workflow of its own -- so that it applies the same
// rules instead of rebuilding them from SafetyConfig's parts.
func (c *Client) CheckMutation(ctx context.Context, m MutationContext) error {
	return c.checkMutation(ctx, m)
}

// PrepareMutation is CheckMutation for a workflow that then locks m.ObjectURL
// and writes under the lock: when the gate passes, the returned context
// records that this object's package was checked, so the writes under the lock
// do not look it up again -- a stateless request that would retire the lock's
// session (issue #91). The mark is set only by a gate that passed, and only for
// m.ObjectURL; use the returned context for the whole lock window.
// PrepareSourceUpdate and PrepareDelete are the same for one operation each.
func (c *Client) PrepareMutation(ctx context.Context, m MutationContext) (context.Context, error) {
	return c.gateAndMark(ctx, m)
}
