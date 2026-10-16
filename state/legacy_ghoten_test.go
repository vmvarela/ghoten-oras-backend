package state

import (
	"strings"
	"testing"
)

func TestWorkspaceLayout_DefaultRemainsHashed(t *testing.T) {
	client := &Client{config: Config{}}
	ws := newWorkspaceClient(client, "default")

	if !strings.HasPrefix(ws.stateTag, stateTagPrefix) {
		t.Fatalf("state tag %q does not have expected prefix", ws.stateTag)
	}
	id := strings.TrimPrefix(ws.stateTag, stateTagPrefix)
	if len(id) != 64 {
		t.Fatalf("default layout must use full SHA-256 workspace IDs, got %q", id)
	}
}

func TestWorkspaceLayout_LegacyGhotenPreservesExistingTags(t *testing.T) {
	client := &Client{config: Config{WorkspaceLayout: WorkspaceLayoutLegacyGhoten}}

	t.Run("literal OCI tag", func(t *testing.T) {
		ws := newWorkspaceClient(client, "default")
		if ws.stateTag != "state-default" {
			t.Fatalf("state tag = %q, want state-default", ws.stateTag)
		}
		if ws.lockTag != "locked-default" {
			t.Fatalf("lock tag = %q, want locked-default", ws.lockTag)
		}
		if ws.versionTagBase != "stver-default" {
			t.Fatalf("version tag base = %q, want stver-default", ws.versionTagBase)
		}
	})

	t.Run("invalid OCI tag", func(t *testing.T) {
		ws := newWorkspaceClient(client, "my workspace")
		id := strings.TrimPrefix(ws.stateTag, stateTagPrefix)
		if !strings.HasPrefix(id, "ws-") || len(id) != len("ws-")+16 {
			t.Fatalf("legacy hash fallback = %q, want ws- plus 16 hex chars", id)
		}
	})
}
