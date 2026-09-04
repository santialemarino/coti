package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// openUpload reads the "file" part of a multipart request, capping the body first so an
// oversized upload is refused while it is still arriving rather than after it is buffered.
// The second result carries the declared content type and size; the third is false once the
// response has been written.
func openUpload(c *gin.Context, maxBytes int64) (multipart.File, *multipart.FileHeader, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	header, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.ErrorResponse{
				Error: "file too large", Code: string(domain.CodeFileTooLarge)})
			return nil, nil, false
		}
		RespondBindError(c, err)
		return nil, nil, false
	}
	file, err := header.Open()
	if err != nil {
		Respond(c, err)
		return nil, nil, false
	}
	return file, header, true
}
