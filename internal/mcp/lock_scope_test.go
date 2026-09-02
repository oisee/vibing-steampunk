package mcp

import (
	"context"
	"testing"
)

// TestFocusedModeHasNothingThatSpendsALockHandle is the regression guard for the
// defect behind half of #169: focused mode whitelisted LockObject and
// UnlockObject while whitelisting nothing that accepts a lock handle. An agent
// in that mode could take a lock and had no way to spend it, so the only
// reachable outcome was a stranded ENQUEUE cleared by hand in SM12.
//
// The rule this pins is the general one, not the two names: if a tool that
// consumes a handle is ever added to focused mode, LockObject may come back
// with it. Until then it may not.
func TestFocusedModeHasNothingThatSpendsALockHandle(t *testing.T) {
	focused := focusedToolSet()

	spenders := []string{
		"UpdateSource", "DeleteObject", "CreateTestInclude",
		"UpdateClassInclude", "WriteMessageClassTexts", "WriteDataElementLabels",
	}
	var reachable []string
	for _, name := range spenders {
		if focused[name] {
			reachable = append(reachable, name)
		}
	}

	handleTakers := []string{"LockObject", "UnlockObject"}
	var exposed []string
	for _, name := range handleTakers {
		if focused[name] {
			exposed = append(exposed, name)
		}
	}

	if len(exposed) > 0 && len(reachable) == 0 {
		t.Fatalf("focused mode exposes %v but whitelists nothing that accepts a lock handle; "+
			"an agent can only strand locks. Either drop them or add a consumer.", exposed)
	}
}

// TestLockScopeHelpersHaveTheShapeHandlersRelyOn is a compile-time contract
// check: both helpers must keep the signature the handlers call them through,
// so a refactor cannot quietly change how a self-taken lock is released.
func TestLockScopeHelpersHaveTheShapeHandlersRelyOn(t *testing.T) {
	// A nil adtClient would panic before reaching the release, so this test
	// documents the contract rather than driving a server; the behavioural
	// proof lives in pkg/adt's session-affinity tests. Kept as a compile-time
	// assertion that the helper exists with the shape the handlers rely on.
	type lockScope func(context.Context, string, string, func(string) error) error
	var _ lockScope = (*Server)(nil).withObjectLock
	var _ lockScope = (*Server)(nil).withObjectLockConsumed
}

// TestSelfLockingToolsDoNotDemandAHandle checks the half of #169 that actually
// changes what a model does. Making the handlers self-locking is invisible if
// the published schema still says lock_handle is required — the client demands
// it, the agent calls LockObject to get one, and the cross-call window is back.
func TestSelfLockingToolsDoNotDemandAHandle(t *testing.T) {
	s := serverForMode(t, "expert")
	tools := s.mcpServer.ListTools()

	selfLocking := []string{"UpdateSource", "DeleteObject", "CreateTestInclude", "UpdateClassInclude"}
	for _, name := range selfLocking {
		tool, ok := tools[name]
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		for _, req := range tool.Tool.InputSchema.Required {
			if req == "lock_handle" {
				t.Errorf("%s still declares lock_handle as required; the handler locks internally, "+
					"so the schema forces an agent back into the cross-call window (#169)", name)
			}
		}
	}

	// UnlockObject is the one place a handle is genuinely required: there is
	// nothing to release without it.
	if tool, ok := tools["UnlockObject"]; ok {
		found := false
		for _, req := range tool.Tool.InputSchema.Required {
			if req == "lock_handle" {
				found = true
			}
		}
		if !found {
			t.Error("UnlockObject must keep lock_handle required — it has nothing to release without one")
		}
	}
}
