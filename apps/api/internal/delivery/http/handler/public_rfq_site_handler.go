package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
)

// GetSite returns public supplier context and branch choices for a registered hostname.
//
// @Summary Get a public RFQ site
// @Tags public-rfqs
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Success 200 {object} dto.PublicRFQSiteResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /v1/public/rfq-sites/{hostname} [get]
func (h *PublicRFQHandler) GetSite(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	site, err := h.service.GetSite(c.Request.Context(), c.Param("hostname"))
	if err != nil {
		Respond(c, err)
		return
	}
	response := dto.PublicRFQSiteResponse{MaxTextCharacters: site.MaxTextCharacters, MaxFileSize: site.MaxFileSize, AcceptedContentTypes: site.AcceptedContentTypes, SupplierName: site.SupplierName, LogoURL: site.LogoURL, Branches: []dto.PublicRFQSiteBranchResponse{}}
	for _, branch := range site.Branches {
		response.Branches = append(response.Branches, dto.PublicRFQSiteBranchResponse{Name: branch.Name, ChannelID: branch.ChannelID})
	}
	c.JSON(http.StatusOK, response)
}
