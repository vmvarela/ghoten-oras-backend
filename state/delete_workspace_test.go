package state

import (
	"context"
	"slices"
	"testing"
)

func TestDeleteWorkspaceRemovesOnlyTargetWorkspace(t *testing.T) {
	ctx := context.Background()
	fake := newFakeORASRepo()
	c := newTestClient(&orasRepositoryClient{inner: fake})
	c.config.WorkspaceLayout = WorkspaceLayoutLegacyGhoten
	c.config.MaxVersions = 2

	if err := c.Put(ctx, "prod", []byte("prod-v1")); err != nil {
		t.Fatal(err)
	}
	c.WaitForRetention()
	if err := c.Put(ctx, "prod", []byte("prod-v2")); err != nil {
		t.Fatal(err)
	}
	c.WaitForRetention()
	if _, err := c.Lock(ctx, "prod", LockInfo{ID: "prod-lock"}); err != nil {
		t.Fatal(err)
	}

	if err := c.Put(ctx, "staging", []byte("staging")); err != nil {
		t.Fatal(err)
	}
	c.WaitForRetention()

	if err := c.DeleteWorkspace(ctx, "prod"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("deleted workspace still has state: %q", got)
	}

	staging, err := c.Get(ctx, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if string(staging) != "staging" {
		t.Fatalf("sibling workspace changed: %q", staging)
	}

	workspaces, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(workspaces, "prod") {
		t.Fatalf("deleted workspace still listed: %v", workspaces)
	}
	if !slices.Contains(workspaces, "staging") {
		t.Fatalf("sibling workspace missing: %v", workspaces)
	}
}
