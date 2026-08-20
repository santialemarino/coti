package http

import (
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/storage"
)

// The route is mounted by trimming apiPrefix off the link path, which silently yields
// /v1/v1/files if the two ever stop agreeing.
func TestLinkPathIsMountedUnderTheAPIPrefix(t *testing.T) {
	t.Parallel()

	if !strings.HasPrefix(storage.LinkPath, apiPrefix+"/") {
		t.Fatalf("storage.LinkPath = %q, want it under %q", storage.LinkPath, apiPrefix)
	}
}
