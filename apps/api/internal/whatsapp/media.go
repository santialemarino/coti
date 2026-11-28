package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// MetaMediaFetcher downloads the files clients send a branch number. Meta hands out a media ID
// with the message; the download link it resolves to expires within minutes, so it is asked for
// only when the file is about to be read.
type MetaMediaFetcher struct {
	client     *http.Client
	baseURL    string
	apiVersion string
}

// NewMetaMediaFetcher builds a MetaMediaFetcher against the Graph API version given.
func NewMetaMediaFetcher(client *http.Client, apiVersion string) *MetaMediaFetcher {
	return newMetaMediaFetcher(client, graphAPIBaseURL, apiVersion)
}

func newMetaMediaFetcher(client *http.Client, baseURL, apiVersion string) *MetaMediaFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &MetaMediaFetcher{client: client, baseURL: strings.TrimRight(baseURL, "/"),
		apiVersion: apiVersion}
}

// FetchMedia downloads one file by its media ID, refusing it once it passes maxBytes.
func (f *MetaMediaFetcher) FetchMedia(ctx context.Context, accessToken, mediaID string,
	maxBytes int64,
) (*domain.FetchedMedia, error) {
	endpoint := f.baseURL + "/" + f.apiVersion + "/" + url.PathEscape(mediaID)
	var described struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
	}
	if err := f.get(ctx, endpoint, accessToken, func(body io.Reader) error {
		return json.NewDecoder(body).Decode(&described)
	}); err != nil {
		return nil, err
	}
	if strings.TrimSpace(described.URL) == "" {
		return nil, fmt.Errorf("WhatsApp media %s has no download URL", mediaID)
	}
	if described.FileSize > maxBytes {
		return nil, fmt.Errorf("%w: WhatsApp media is %d bytes, over the %d limit",
			domain.ErrInvalidInput, described.FileSize, maxBytes)
	}
	var data []byte
	if err := f.get(ctx, described.URL, accessToken, func(body io.Reader) error {
		var readErr error
		data, readErr = io.ReadAll(io.LimitReader(body, maxBytes+1))
		// A download cut short says nothing about the file, so it is worth reading again.
		if readErr != nil {
			return fmt.Errorf("%w: WhatsApp media download: %v", domain.ErrMediaUnavailable, readErr)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: WhatsApp media is over the %d byte limit",
			domain.ErrInvalidInput, maxBytes)
	}
	return &domain.FetchedMedia{MediaType: described.MimeType, Data: data}, nil
}

func (f *MetaMediaFetcher) get(ctx context.Context, endpoint, accessToken string,
	read func(io.Reader) error,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: WhatsApp Cloud API media request: %v", domain.ErrMediaUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusInternalServerError ||
		response.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %v", domain.ErrMediaUnavailable, readMetaResponseError(response))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return readMetaResponseError(response)
	}
	return read(response.Body)
}

var _ domain.WhatsAppMediaFetcher = (*MetaMediaFetcher)(nil)
