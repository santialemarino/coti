package http

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
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

// mountedPaths lists what NewRouter registered, so a route's absence is asserted on the router
// rather than inferred from the branch that was supposed to skip it.
func mountedPaths(t *testing.T, h Handlers) []string {
	t.Helper()
	router := NewRouter(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		h, Auth{}, RateLimit{})

	paths := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		paths = append(paths, route.Path)
	}
	return paths
}

// A bucket serves the links it signs, so mounting this route beside one would answer for objects
// the API does not hold — through a handler that is nil.
func TestFileRouteIsMountedOnlyForTheAdapterThatServesIt(t *testing.T) {
	t.Parallel()
	const route = storage.LinkPath + "/*key"

	if got := mountedPaths(t, Handlers{File: &handler.FileHandler{}}); !slices.Contains(got, route) {
		t.Errorf("routes = %v, want %s mounted when a local adapter is bound", got, route)
	}
	if got := mountedPaths(t, Handlers{}); slices.Contains(got, route) {
		t.Errorf("routes = %v, want no %s when no local adapter is bound", got, route)
	}
}
