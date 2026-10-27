package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// List returns the fiscal history of the active branch, or of every branch the caller reaches.
// @Summary List invoices
// @Tags invoicing
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string false "Active branch; omitted lists every branch the caller reaches"
// @Param page query int false "Page number"
// @Success 200 {object} dto.InvoiceListResponse
// @Router /v1/invoices [get]
func (h *InvoiceHandler) List(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		RespondBindError(c, err)
		return
	}
	invoices, total, err := h.invoices.List(c.Request.Context(), tenant, page)
	if err != nil {
		Respond(c, err)
		return
	}
	response := dto.InvoiceListResponse{Items: make([]dto.InvoiceResponse, 0, len(invoices)), Total: total, Page: page, PageSize: domain.InvoicePageSize}
	for _, invoice := range invoices {
		response.Items = append(response.Items, toInvoiceResponse(invoice))
	}
	c.JSON(http.StatusOK, response)
}

// ListAwaitingInvoice returns the accepted sales still waiting for an invoice, paid first.
// @Summary List sales awaiting an invoice
// @Description Accepted, unarchived sales with no live invoice, no pending voucher and no waiver, in the caller's branch and seller reach; empty while the account does not invoice.
// @Tags invoicing
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string false "Active branch; omitted lists every branch the caller reaches"
// @Param page query int false "Page number"
// @Success 200 {object} dto.SaleAwaitingInvoiceListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Router /v1/quotes/awaiting-invoice [get]
func (h *InvoiceHandler) ListAwaitingInvoice(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		RespondBindError(c, err)
		return
	}
	sales, total, err := h.invoices.ListAwaitingInvoice(c.Request.Context(), tenant, page)
	if err != nil {
		Respond(c, err)
		return
	}
	response := dto.SaleAwaitingInvoiceListResponse{
		Items: make([]dto.SaleAwaitingInvoiceResponse, 0, len(sales)),
		Total: total, Page: page, PageSize: domain.InvoicePageSize,
	}
	for _, sale := range sales {
		response.Items = append(response.Items, dto.SaleAwaitingInvoiceResponse{
			QuoteID: sale.QuoteID, RFQID: sale.RFQID, Number: sale.Number, BranchID: sale.BranchID,
			BranchName: sale.BranchName, ClientID: sale.ClientID, Client: sale.ClientLabel,
			Total: sale.Total.StringFixed(2), AcceptedAt: sale.AcceptedAt, PaidAt: sale.PaidAt,
		})
	}
	c.JSON(http.StatusOK, response)
}

// DownloadPDF returns the authorized invoice's canonical fiscal document.
// @Summary Download invoice PDF
// @Tags invoicing
// @Produce application/pdf
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param invoiceId path string true "Invoice id"
// @Success 200 {file} binary
// @Router /v1/invoices/{invoiceId}/pdf [get]
func (h *InvoiceHandler) DownloadPDF(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "invoiceId")
	if !ok {
		return
	}
	content, err := h.invoices.DownloadPDF(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="invoice-`+id.String()+`.pdf"`)
	c.Data(http.StatusOK, "application/pdf", content)
}

// Recover consults a pending authorization without issuing another invoice.
// @Summary Recover invoice authorization
// @Tags invoicing
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param invoiceId path string true "Invoice id"
// @Success 200 {object} dto.InvoiceResponse
// @Router /v1/invoices/{invoiceId}/recover [post]
func (h *InvoiceHandler) Recover(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "invoiceId")
	if !ok {
		return
	}
	invoiced, err := h.invoices.Recover(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toInvoiceResponse(*invoiced))
}
