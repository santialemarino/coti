package storage

import (
	"fmt"
	"path"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// validateKey enforces the key rules every adapter shares, so an object stored through one is
// reachable through the other. Both refusals are separate checks and neither covers the other:
// a non-canonical key means a bucket and a filesystem disagree about where the object lives,
// and a climbing key is what path.Clean leaves exactly as it found it.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: storage key is empty", domain.ErrInvalidInput)
	}
	if strings.HasSuffix(key, metaSuffix) {
		return fmt.Errorf("%w: storage key may not end in %s", domain.ErrInvalidInput, metaSuffix)
	}
	if path.IsAbs(key) || path.Clean(key) != key {
		return fmt.Errorf("%w: storage key is not a canonical relative path: %s",
			domain.ErrInvalidInput, key)
	}
	if key == ".." || strings.HasPrefix(key, "../") {
		return fmt.Errorf("%w: storage key escapes the base directory: %s",
			domain.ErrInvalidInput, key)
	}
	return nil
}
