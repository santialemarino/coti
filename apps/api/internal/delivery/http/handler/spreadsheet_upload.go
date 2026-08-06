package handler

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func openSpreadsheetUpload(c *gin.Context, maxBytes int64) (multipart.File, string, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.ErrorResponse{
				Error: "file too large", Code: string(domain.CodeFileTooLarge)})
			return nil, "", false
		}
		RespondBindError(c, err)
		return nil, "", false
	}
	file, err := fileHeader.Open()
	if err != nil {
		Respond(c, err)
		return nil, "", false
	}
	return file, fileHeader.Filename, true
}
