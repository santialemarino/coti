// Package openai adapts OpenAI's embedding and transcription endpoints to the domain Embedder and
// Transcriber ports. Anthropic exposes neither, which is why a second provider exists at all.
//
// The two endpoints are called over net/http rather than through a provider SDK: the retry policy,
// the timeouts and the usage log are shared with the other adapter and belong to internal/ai, so an
// SDK would contribute request signing and little else.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/ai"
)

// maxErrorBody caps how much of a failed response is read into the error, so a provider answering
// with an HTML page does not put it all in the log.
const maxErrorBody = 2 << 10

// post sends body as JSON to path and decodes the reply. A transport failure, a rate limit or a
// provider-side fault comes back marked for another attempt.
func post(ctx context.Context, client *http.Client, baseURL, apiKey, path string, body, reply any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	request, err := newRequest(ctx, baseURL, apiKey, path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	return send(client, request, reply)
}

// newRequest builds an authenticated request against the configured base URL.
func newRequest(ctx context.Context, baseURL, apiKey, path, contentType string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", contentType)
	return request, nil
}

// send performs request and decodes a successful reply, classifying anything else.
func send(client *http.Client, request *http.Request, reply any) error {
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// The caller walked away; that is not an outage to report as one.
			return err
		}
		// No response at all: a dial, TLS or read failure, all worth another attempt.
		return ai.Retryable(fmt.Errorf("call %s: %w", request.URL.Path, err))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return statusError(request.URL.Path, response)
	}
	if err := json.NewDecoder(response.Body).Decode(reply); err != nil {
		return ai.Retryable(fmt.Errorf("decode %s response: %w", request.URL.Path, err))
	}
	return nil
}

// statusError turns a non-200 into an error of the right kind, using the one shared definition of
// which statuses another attempt could clear. A rejected request is our fault and final: repeating
// it would only spend the allowance.
func statusError(path string, response *http.Response) error {
	detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	err := fmt.Errorf("call %s: provider answered %d: %s", path, response.StatusCode,
		strings.TrimSpace(string(detail)))

	if !ai.RetryableStatus(response.StatusCode) {
		return ai.Rejected(err)
	}
	// The provider's own window beats our ladder, which is measured in single seconds.
	if after := ai.RetryAfter(response.Header); after > 0 {
		return ai.RetryableAfter(err, after)
	}
	return ai.Retryable(err)
}

var errEmptyReply = errors.New("provider returned an empty reply")
