package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// defaultObjectContentType is what an object stored without one is served as, rather than a
// blank Content-Type header the browser has to guess from.
const defaultObjectContentType = "application/octet-stream"

// SignedObjectSource is what the file route needs from the storage layer: the signature check
// and the bytes. The local adapter provides both, and only it — a bucket serves its own links,
// so this route is never mounted beside one.
type SignedObjectSource interface {
	Verify(key string, expiresAt time.Time, signature string) error
	Download(ctx context.Context, key string) (*domain.StoredObject, error)
}

// FileHandler serves the objects the local storage adapter signed links for.
type FileHandler struct {
	source SignedObjectSource
}

// NewFileHandler builds a FileHandler over the adapter that signs the links it serves.
func NewFileHandler(source SignedObjectSource) *FileHandler {
	return &FileHandler{source: source}
}

// Get streams one object to whoever holds an unexpired signed link. It resolves no session on
// purpose: the signature is the authorization, which is what lets a client open a quote
// document from a link without an account.
//
//	@Summary		Download a signed object
//	@Description	Serves a stored file to the holder of an unexpired signed link. No session: the signature is the authorization.
//	@Tags			files
//	@Produce		octet-stream
//	@Param			key			path		string	true	"Object key"
//	@Param			expires		query		integer	true	"Deadline, in unix seconds"
//	@Param			signature	query		string	true	"Signature covering the key and the deadline"
//	@Success		200			{file}		binary
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/files/{key} [get]
func (h *FileHandler) Get(c *gin.Context) {
	// Gin's wildcard keeps the leading separator; a stored key has none.
	key := strings.TrimPrefix(c.Param("key"), "/")
	seconds, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil {
		Respond(c, domain.WithCode(domain.CodeInvalidLink,
			fmt.Errorf("%w: expires is not a unix timestamp", domain.ErrForbidden)))
		return
	}
	if err := h.source.Verify(key, time.Unix(seconds, 0), c.Query("signature")); err != nil {
		Respond(c, err)
		return
	}
	object, err := h.source.Download(c.Request.Context(), key)
	if err != nil {
		Respond(c, err)
		return
	}
	defer object.Body.Close()

	contentType := object.ContentType
	if contentType == "" {
		contentType = defaultObjectContentType
	}
	c.DataFromReader(http.StatusOK, object.Size, contentType, object.Body, nil)
}
