package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// InvoiceService is the invoicing surface the handler needs.
type InvoiceService interface {
	GetSettings(ctx context.Context, tenant domain.Tenant) (*domain.InvoicingSettings, error)
	UpdateSettings(ctx context.Context, tenant domain.Tenant, in domain.InvoicingSettingsUpdate) (*domain.InvoicingSettings, error)
	UploadCredentials(ctx context.Context, tenant domain.Tenant, certificatePEM, keyPEM []byte) (*domain.ARCACredentialStatus, error)
	DeleteCredentials(ctx context.Context, tenant domain.Tenant) error
	GetClientFiscal(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID) (*domain.ClientFiscal, error)
	UpdateClientFiscal(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID, in domain.ClientFiscalUpdate) (*domain.ClientFiscal, error)
	Preview(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.InvoicePreview, error)
	Issue(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (*domain.Invoice, error)
}

// InvoiceHandler serves ARCA invoicing: the account's setup, buyers' fiscal data and the invoices.
type InvoiceHandler struct {
	invoices InvoiceService
	maxBytes int64
}

// NewInvoiceHandler builds an InvoiceHandler; maxBytes caps a certificate upload.
func NewInvoiceHandler(invoices InvoiceService, maxBytes int64) *InvoiceHandler {
	return &InvoiceHandler{invoices: invoices, maxBytes: maxBytes}
}

// GetSettings returns the account's invoicing setup.
//
//	@Summary		Get invoicing settings
//	@Description	Returns the account's IVA condition, how its prices treat IVA, each branch's ARCA point of sale and the uploaded certificate.
//	@Tags			invoicing
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.InvoicingSettingsResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/invoicing/settings [get]
func (h *InvoiceHandler) GetSettings(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	settings, err := h.invoices.GetSettings(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toInvoicingSettingsResponse(*settings))
}

// UpdateSettings replaces the account's fiscal settings and the listed branches' points of sale.
//
//	@Summary		Update invoicing settings
//	@Description	Replaces the account's IVA condition and price treatment and the listed branches' points of sale.
//	@Tags			invoicing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.UpdateInvoicingSettingsRequest	true	"Invoicing settings"
//	@Success		200		{object}	dto.InvoicingSettingsResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse
//	@Router			/v1/invoicing/settings [put]
func (h *InvoiceHandler) UpdateSettings(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.UpdateInvoicingSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	in := domain.InvoicingSettingsUpdate{PricesIncludeVAT: body.PricesIncludeVAT,
		PointsOfSale: make(map[uuid.UUID]*int, len(body.Branches))}
	if body.IVACondition != nil {
		condition := domain.IVACondition(*body.IVACondition)
		in.IVACondition = &condition
	}
	for _, b := range body.Branches {
		in.PointsOfSale[b.BranchID] = b.PointOfSale
	}
	settings, err := h.invoices.UpdateSettings(c.Request.Context(), tenant, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toInvoicingSettingsResponse(*settings))
}

// UploadCredentials stores the account's ARCA certificate and private key.
//
//	@Summary		Upload ARCA credentials
//	@Description	Stores the account's ARCA certificate and its private key (sealed at rest). The key must match the certificate.
//	@Tags			invoicing
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			certificate	formData	file	true	"X.509 certificate (PEM)"
//	@Param			private_key	formData	file	true	"RSA private key (PEM)"
//	@Success		200			{object}	dto.ARCACredentialResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		413			{object}	dto.ErrorResponse	"FILE_TOO_LARGE"
//	@Failure		422			{object}	dto.ErrorResponse	"ARCA_CREDENTIALS"
//	@Failure		503			{object}	dto.ErrorResponse	"NOT_CONFIGURED"
//	@Router			/v1/invoicing/credentials [put]
func (h *InvoiceHandler) UploadCredentials(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes)
	certificate, ok := readFormFile(c, "certificate")
	if !ok {
		return
	}
	key, ok := readFormFile(c, "private_key")
	if !ok {
		return
	}
	status, err := h.invoices.UploadCredentials(c.Request.Context(), tenant, certificate, key)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toARCACredentialResponse(*status))
}

// DeleteCredentials removes the account's ARCA certificate and key.
//
//	@Summary		Delete ARCA credentials
//	@Tags			invoicing
//	@Security		BearerAuth
//	@Success		204
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/invoicing/credentials [delete]
func (h *InvoiceHandler) DeleteCredentials(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.invoices.DeleteCredentials(c.Request.Context(), tenant); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetClientFiscal returns a client's fiscal identity.
//
//	@Summary		Get a client's fiscal data
//	@Tags			clients
//	@Produce		json
//	@Security		BearerAuth
//	@Param			clientId	path		string	true	"Client id"
//	@Success		200			{object}	dto.ClientFiscalResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/clients/{clientId}/fiscal [get]
func (h *InvoiceHandler) GetClientFiscal(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clientID, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	client, err := h.invoices.GetClientFiscal(c.Request.Context(), tenant, clientID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toClientFiscalResponse(*client))
}

// UpdateClientFiscal replaces a client's legal name, tax id and IVA condition.
//
//	@Summary		Update a client's fiscal data
//	@Description	Replaces the client's legal name, CUIT or DNI (check digit verified) and IVA condition; omitted fields clear.
//	@Tags			clients
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			clientId	path		string							true	"Client id"
//	@Param			body		body		dto.UpdateClientFiscalRequest	true	"Fiscal data"
//	@Success		200			{object}	dto.ClientFiscalResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse	"INVALID_TAX_ID"
//	@Router			/v1/clients/{clientId}/fiscal [put]
func (h *InvoiceHandler) UpdateClientFiscal(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	clientID, ok := pathUUID(c, "clientId")
	if !ok {
		return
	}
	var body dto.UpdateClientFiscalRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	in := domain.ClientFiscalUpdate{LegalName: body.LegalName, TaxID: body.TaxID}
	if body.IVACondition != nil {
		condition := domain.IVACondition(*body.IVACondition)
		in.IVACondition = &condition
	}
	client, err := h.invoices.UpdateClientFiscal(c.Request.Context(), tenant, clientID, in)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toClientFiscalResponse(*client))
}

// Preview returns the invoice an accepted quote would produce, or the one it has.
//
//	@Summary		Preview a quote's invoice
//	@Description	Returns the invoice type, buyer and amounts the quote would be invoiced with now, every gap that stops it, and the invoice it already has.
//	@Tags			invoicing
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"The quote's branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.InvoicePreviewResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/quotes/{quoteId}/invoice [get]
func (h *InvoiceHandler) Preview(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	preview, err := h.invoices.Preview(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toInvoicePreviewResponse(*preview))
}

// Issue asks ARCA to authorize the invoice for an accepted quote.
//
//	@Summary		Issue a quote's invoice
//	@Description	Asks ARCA to authorize the electronic invoice for an accepted quote. Irreversible: a mistake is corrected with a credit note.
//	@Tags			invoicing
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"The quote's branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		201			{object}	dto.InvoiceResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_ALREADY_INVOICED, INVOICE_IN_PROGRESS"
//	@Failure		422			{object}	dto.ErrorResponse	"INVOICE_NOT_READY, INVOICE_REJECTED"
//	@Failure		503			{object}	dto.ErrorResponse	"INVOICING_UNAVAILABLE"
//	@Router			/v1/quotes/{quoteId}/invoice [post]
func (h *InvoiceHandler) Issue(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	invoice, err := h.invoices.Issue(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toInvoiceResponse(*invoice))
}

// readFormFile reads one named multipart file whole. The second result is false once the
// response has been written.
func readFormFile(c *gin.Context, field string) ([]byte, bool) {
	header, err := c.FormFile(field)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.ErrorResponse{
				Error: "file too large", Code: string(domain.CodeFileTooLarge)})
			return nil, false
		}
		RespondBindError(c, err)
		return nil, false
	}
	file, err := header.Open()
	if err != nil {
		Respond(c, err)
		return nil, false
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		Respond(c, err)
		return nil, false
	}
	return body, true
}

func toInvoicingSettingsResponse(s domain.InvoicingSettings) dto.InvoicingSettingsResponse {
	response := dto.InvoicingSettingsResponse{
		Enabled: s.Enabled, Environment: s.Environment, LegalName: s.Account.LegalName,
		TaxID: s.Account.TaxID, IVACondition: conditionString(s.Account.IVACondition),
		PricesIncludeVAT: s.Account.PricesIncludeVAT, Branches: make([]dto.BranchPointOfSaleResponse, 0, len(s.Branches)),
	}
	for _, b := range s.Branches {
		response.Branches = append(response.Branches, dto.BranchPointOfSaleResponse{
			BranchID: b.BranchID, Name: b.BranchName, IsActive: b.IsActive, PointOfSale: b.PointOfSale})
	}
	if s.Credential != nil {
		credential := toARCACredentialResponse(*s.Credential)
		response.Credential = &credential
	}
	return response
}

func toARCACredentialResponse(s domain.ARCACredentialStatus) dto.ARCACredentialResponse {
	return dto.ARCACredentialResponse{CUIT: s.CUIT, Subject: s.Subject, ExpiresAt: s.ExpiresAt, UpdatedAt: s.UpdatedAt}
}

func toClientFiscalResponse(c domain.ClientFiscal) dto.ClientFiscalResponse {
	return dto.ClientFiscalResponse{ID: c.ID, Name: c.Name, LegalName: c.LegalName, TaxID: c.TaxID,
		IVACondition: conditionString(c.IVACondition)}
}

func toInvoicePreviewResponse(p domain.InvoicePreview) dto.InvoicePreviewResponse {
	response := dto.InvoicePreviewResponse{
		Type: string(p.Type), PointOfSale: p.PointOfSale, Receiver: toInvoiceReceiverResponse(p.Receiver),
		Amounts: toInvoiceAmountsResponse(p.Amounts), Currency: p.Currency, Issues: p.Issues,
	}
	if p.Invoice != nil {
		invoice := toInvoiceResponse(*p.Invoice)
		response.Invoice = &invoice
	}
	return response
}

func toInvoiceResponse(inv domain.Invoice) dto.InvoiceResponse {
	response := dto.InvoiceResponse{
		ID: inv.ID, Status: string(inv.Status), Type: string(inv.Type), PointOfSale: inv.PointOfSale,
		Number: inv.Number, IssuedOn: inv.IssuedOn.Format("2006-01-02"), CAE: inv.CAE,
		IssuerCUIT: inv.IssuerCUIT, Receiver: toInvoiceReceiverResponse(inv.Receiver),
		Amounts: toInvoiceAmountsResponse(inv.Amounts), Currency: inv.Currency, Issues: inv.Issues,
		QRURL: domain.InvoiceQRURL(inv), CreatedAt: inv.CreatedAt,
	}
	if response.Issues == nil {
		response.Issues = []string{}
	}
	if inv.CAEExpiresOn != nil {
		expires := inv.CAEExpiresOn.Format("2006-01-02")
		response.CAEExpiresOn = &expires
	}
	return response
}

func toInvoiceReceiverResponse(r domain.InvoiceReceiver) dto.InvoiceReceiverResponse {
	return dto.InvoiceReceiverResponse{Name: r.Name, DocType: string(r.DocType), DocNumber: r.DocNumber,
		IVACondition: string(r.IVACondition)}
}

func toInvoiceAmountsResponse(a domain.InvoiceAmounts) dto.InvoiceAmountsResponse {
	response := dto.InvoiceAmountsResponse{Net: a.Net.StringFixed(2), Exempt: a.Exempt.StringFixed(2),
		VAT: a.VAT.StringFixed(2), Total: a.Total.StringFixed(2), ByRate: make([]dto.VATAmountResponse, 0, len(a.ByRate))}
	for _, r := range a.ByRate {
		response.ByRate = append(response.ByRate, dto.VATAmountResponse{Rate: string(r.Rate),
			Base: r.Base.StringFixed(2), Amount: r.Amount.StringFixed(2)})
	}
	return response
}

func conditionString(c *domain.IVACondition) *string {
	if c == nil {
		return nil
	}
	s := string(*c)
	return &s
}
