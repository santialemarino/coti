package handler

import (
	"mime/multipart"

	"github.com/gin-gonic/gin"
)

// openSpreadsheetUpload reads the uploaded spreadsheet and its filename, which is what decides
// how the importers parse it.
func openSpreadsheetUpload(c *gin.Context, maxBytes int64) (multipart.File, string, bool) {
	file, header, ok := openUpload(c, maxBytes)
	if !ok {
		return nil, "", false
	}
	return file, header.Filename, true
}
