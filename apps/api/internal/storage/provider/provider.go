// Package provider binds the domain.ObjectStorage port to the adapter selected for it. It sits
// above the adapters so every command that stores a file makes the same choice from the same
// settings.
package provider

import (
	"fmt"
	"log/slog"
	"net/url"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

// Set is the storage adapter bound at startup. Local is the same adapter as Storage whenever
// the local one is selected, and nil otherwise: the API serves the links it signs itself, and
// there is nothing to serve when the bucket signs its own.
type Set struct {
	Storage domain.ObjectStorage
	Local   *storage.LocalStorage
}

// Bind selects the adapter behind the port. config.Load has already refused a provider with no
// adapter and a selected provider missing its credentials, so a failure here is the adapter
// itself declining to start.
func Bind(cfg config.StorageConfig, log *slog.Logger) (Set, error) {
	switch cfg.Provider {
	case config.StorageProviderLocal:
		settings := cfg.Local()
		base, err := url.Parse(settings.APIBaseURL)
		if err != nil {
			return Set{}, fmt.Errorf("parse STORAGE_LOCAL_API_BASE_URL: %w", err)
		}
		local, err := storage.NewLocalStorage(
			settings.Dir, base, storage.NewURLSigner([]byte(settings.SigningSecret), nil))
		if err != nil {
			return Set{}, err
		}
		log.Warn("object storage is on the filesystem: files live with the process, not in a bucket",
			slog.String("dir", settings.Dir))
		return Set{Storage: local, Local: local}, nil
	case config.StorageProviderSpaces:
		return Set{Storage: storage.NewSpacesStorage(cfg.Spaces())}, nil
	default:
		return Set{}, fmt.Errorf("no object storage adapter for provider %q", cfg.Provider)
	}
}

// Describe records which adapter ended up behind the port, so a deployment can be read back
// from its own startup log instead of from the environment it was given.
func (s Set) Describe(log *slog.Logger) {
	log.Info("object storage bound", slog.String("adapter", fmt.Sprintf("%T", s.Storage)))
}
