package domain

import (
	"context"
	"strings"
	"time"
	_ "time/tzdata" // the API images ship without a zoneinfo database.

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// IVACondition is a party's standing before ARCA for IVA, which decides the invoice type.
type IVACondition string

const (
	IVAConditionRegistered    IVACondition = "REGISTERED"
	IVAConditionMonotributo   IVACondition = "MONOTRIBUTO"
	IVAConditionExempt        IVACondition = "EXEMPT"
	IVAConditionFinalConsumer IVACondition = "FINAL_CONSUMER"
)

// Valid reports whether c is one of the declared conditions.
func (c IVACondition) Valid() bool {
	switch c {
	case IVAConditionRegistered, IVAConditionMonotributo, IVAConditionExempt, IVAConditionFinalConsumer:
		return true
	}
	return false
}

// ARCAReceiverID is the CondicionIVAReceptorId WSFEv1 takes for a buyer in this condition.
func (c IVACondition) ARCAReceiverID() int {
	switch c {
	case IVAConditionRegistered:
		return 1
	case IVAConditionExempt:
		return 4
	case IVAConditionMonotributo:
		return 6
	default:
		return 5
	}
}

// VATRate is the IVA rate a product is sold at.
type VATRate string

const (
	VATRateZero        VATRate = "VAT_0"
	VATRateTwoFive     VATRate = "VAT_2_5"
	VATRateFive        VATRate = "VAT_5"
	VATRateTenFive     VATRate = "VAT_10_5"
	VATRateTwentyOne   VATRate = "VAT_21"
	VATRateTwentySeven VATRate = "VAT_27"
	VATRateExempt      VATRate = "EXEMPT"
)

// DefaultVATRate is the rate a product carries until someone sets another.
const DefaultVATRate = VATRateTwentyOne

// Valid reports whether r is one of the declared rates.
func (r VATRate) Valid() bool {
	_, ok := vatRates[r]
	return ok
}

// Percent is the rate as a fraction-free percentage; zero for EXEMPT.
func (r VATRate) Percent() decimal.Decimal { return vatRates[r].percent }

// ARCAID is the AlicIva Id WSFEv1 takes for the rate; zero for EXEMPT, which ARCA reports as
// an exempt amount rather than as a rate.
func (r VATRate) ARCAID() int { return vatRates[r].arcaID }

var vatRates = map[VATRate]struct {
	percent decimal.Decimal
	arcaID  int
}{
	VATRateZero:        {decimal.Zero, 3},
	VATRateTwoFive:     {decimal.RequireFromString("2.5"), 9},
	VATRateFive:        {decimal.NewFromInt(5), 8},
	VATRateTenFive:     {decimal.RequireFromString("10.5"), 4},
	VATRateTwentyOne:   {decimal.NewFromInt(21), 5},
	VATRateTwentySeven: {decimal.NewFromInt(27), 6},
	VATRateExempt:      {decimal.Zero, 0},
}

// InvoiceType is the letter of an electronic invoice.
type InvoiceType string

const (
	InvoiceTypeA InvoiceType = "A"
	InvoiceTypeB InvoiceType = "B"
	InvoiceTypeC InvoiceType = "C"
)

// DiscriminatesVAT reports whether the invoice breaks IVA out; a C invoice never does.
func (t InvoiceType) DiscriminatesVAT() bool { return t != InvoiceTypeC }

// VoucherKind is what an ARCA voucher does: bill a sale, or cancel an invoice in full.
type VoucherKind string

const (
	VoucherKindInvoice    VoucherKind = "INVOICE"
	VoucherKindCreditNote VoucherKind = "CREDIT_NOTE"
)

// VoucherType is a voucher's kind and letter, which together name its ARCA series.
type VoucherType struct {
	Kind   VoucherKind
	Letter InvoiceType
}

// ARCACode is the CbteTipo WSFEv1 takes: 1, 6, 11 for invoices A, B, C and 3, 8, 13 for their
// credit notes.
func (v VoucherType) ARCACode() int {
	invoice, creditNote := 11, 13
	switch v.Letter {
	case InvoiceTypeA:
		invoice, creditNote = 1, 3
	case InvoiceTypeB:
		invoice, creditNote = 6, 8
	}
	if v.Kind == VoucherKindCreditNote {
		return creditNote
	}
	return invoice
}

// ReceiverDocType is how an invoice identifies its buyer to ARCA.
type ReceiverDocType string

const (
	ReceiverDocCUIT ReceiverDocType = "CUIT"
	ReceiverDocDNI  ReceiverDocType = "DNI"
	ReceiverDocNone ReceiverDocType = "NONE"
)

// ARCACode is the DocTipo WSFEv1 takes.
func (d ReceiverDocType) ARCACode() int {
	switch d {
	case ReceiverDocCUIT:
		return 80
	case ReceiverDocDNI:
		return 96
	default:
		return 99
	}
}

// InvoiceStatus is where an invoice is in being authorized.
type InvoiceStatus string

const (
	// InvoiceStatusPending holds the quote while ARCA is being asked, so a second press cannot
	// authorize a second invoice for the same sale.
	InvoiceStatusPending  InvoiceStatus = "PENDING"
	InvoiceStatusIssued   InvoiceStatus = "ISSUED"
	InvoiceStatusRejected InvoiceStatus = "REJECTED"
)

// VATAmount is the base and the tax of one rate on an invoice.
type VATAmount struct {
	Rate   VATRate
	Base   decimal.Decimal
	Amount decimal.Decimal
}

// InvoiceAmounts is an invoice's money, computed by the backend from the frozen quote.
type InvoiceAmounts struct {
	Net    decimal.Decimal // taxed base; for a C invoice, the whole price.
	Exempt decimal.Decimal
	VAT    decimal.Decimal
	Total  decimal.Decimal
	ByRate []VATAmount
}

// InvoiceReceiver is the buyer as the invoice names them.
type InvoiceReceiver struct {
	Address      string
	Name         string
	DocType      ReceiverDocType
	DocNumber    string // digits only; empty for ReceiverDocNone.
	IVACondition IVACondition
}

// AssociatedVoucher is the authorized voucher a credit note cancels, as CbtesAsoc names it.
type AssociatedVoucher struct {
	Type        VoucherType
	PointOfSale int
	Number      int64
	IssuerCUIT  string
	Date        time.Time
}

// InvoiceRequest is everything ARCA is asked to authorize for one invoice or credit note.
type InvoiceRequest struct {
	IssuerCUIT  string // digits only; the represented CUIT.
	Kind        VoucherKind
	Type        InvoiceType
	Associated  *AssociatedVoucher // set on a credit note only.
	PointOfSale int
	Date        time.Time // the calendar day, at midnight UTC.
	Receiver    InvoiceReceiver
	Amounts     InvoiceAmounts
	// Claim records the number about to be requested, before ARCA can authorize it; an error
	// stops the request.
	Claim func(ctx context.Context, number int64) error
}

// VoucherType is the series the request is numbered in.
func (r InvoiceRequest) VoucherType() VoucherType {
	return VoucherType{Kind: r.Kind, Letter: r.Type}
}

// InvoiceExpectation is the invoice the seller confirmed; issuing refuses if the sale moved since.
type InvoiceExpectation struct {
	Fingerprint string
	VersionID   uuid.UUID
	Type        InvoiceType
	Total       decimal.Decimal
}

// InvoiceAuthorization is ARCA's answer for an authorized invoice.
type InvoiceAuthorization struct {
	Number       int64
	CAE          string
	CAEExpiresOn time.Time
	Observations []string
	RawRequest   []byte
	RawResponse  []byte
}

// Argentina is the zone an invoice's calendar day is read in.
var Argentina = mustLoadLocation("America/Argentina/Buenos_Aires")

// InvoiceDay is the Argentine calendar day of now, as a DATE column holds it: midnight UTC.
func InvoiceDay(now time.Time) time.Time {
	local := now.In(Argentina)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// SealedARCATicket is a WSAA ticket whose token and sign are sealed for storage.
type SealedARCATicket struct {
	Token     string
	Sign      string
	ExpiresAt time.Time
}

// ARCATicketStore keeps an account's live WSAA ticket.
type ARCATicketStore interface {
	Load(ctx context.Context) (*ARCATicket, error)
	Save(ctx context.Context, ticket ARCATicket) error
}

// ARCACredentials are the certificate and key an account signs its ARCA requests with.
type ARCACredentials struct {
	Environment    ARCAEnvironment // the deployment the certificate belongs to, and the one called.
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	Tickets        ARCATicketStore // nil keeps tickets in the issuer's memory only.
}

// InvoiceIssuer authorizes electronic invoices with ARCA. Numbering is the issuer's: it asks
// for the last authorized number and claims the next one in the same exchange.
type InvoiceIssuer interface {
	Issue(ctx context.Context, creds ARCACredentials, req InvoiceRequest) (*InvoiceAuthorization, error)
	// Authorized returns the voucher ARCA holds under number, nil when it holds none; it is how a
	// pending one of unknown outcome is reconciled.
	Authorized(ctx context.Context, creds ARCACredentials, issuerCUIT string, voucher VoucherType,
		pointOfSale int, number int64) (*AuthorizedInvoice, error)
}

// AuthorizedInvoice is an invoice as ARCA has it on record.
type AuthorizedInvoice struct {
	Number       int64
	CAE          string
	CAEExpiresOn time.Time
	Date         time.Time
	DocType      ReceiverDocType
	DocNumber    string
	Total        decimal.Decimal
}

// InvoiceNotReadyError lists the fiscal data missing before an invoice can be asked for.
type InvoiceNotReadyError struct {
	Issues []string
}

// Error describes what is missing.
func (e *InvoiceNotReadyError) Error() string {
	return "invoice not ready: " + strings.Join(e.Issues, "; ")
}

// Unwrap classifies the gap as invalid input.
func (e *InvoiceNotReadyError) Unwrap() error {
	return WithCode(CodeInvoiceNotReady, ErrInvalidInput)
}

// InvoiceRejectedError carries ARCA's reasons for refusing an invoice, which the seller can act on.
type InvoiceRejectedError struct {
	Issues      []string
	RawRequest  []byte
	RawResponse []byte
}

// Error describes the rejection.
func (e *InvoiceRejectedError) Error() string {
	return "invoice rejected by ARCA: " + strings.Join(e.Issues, "; ")
}

// Unwrap classifies a rejection as invalid input.
func (e *InvoiceRejectedError) Unwrap() error {
	return WithCode(CodeInvoiceRejected, ErrInvalidInput)
}

// Invoice is an electronic voucher for an accepted quote: an invoice, or the credit note that
// cancels one in full.
type Invoice struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	BranchID          uuid.UUID
	QuoteID           uuid.UUID
	QuoteVersionID    uuid.UUID
	Environment       ARCAEnvironment // where it is authorized; never changes, so recovery asks the same one.
	Kind              VoucherKind
	CreditedInvoiceID *uuid.UUID // on a credit note: the invoice it cancels.
	ReplacesInvoiceID *uuid.UUID // on an invoice issued after a credit note: the credited invoice it follows.
	Status            InvoiceStatus
	Type              InvoiceType
	PointOfSale       int
	Number            *int64
	ClaimedNumber     *int64 // the number last requested from ARCA, set before the request goes out.
	IssuedOn          time.Time
	CAE               *string
	CAEExpiresOn      *time.Time
	IssuerCUIT        string
	Receiver          InvoiceReceiver
	Amounts           InvoiceAmounts
	Currency          string
	Snapshot          QuoteRepresentationPayload
	IssuerCondition   IVACondition
	IssuerProfile     InvoiceIssuerProfile
	Issues            []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreditNote        *Invoice // on an invoice: its newest credit note, when it has one.
	CreditedInvoice   *Invoice // on a credit note: the invoice it cancels, when loaded.
}

// VoucherType is the series the voucher is numbered in.
func (inv Invoice) VoucherType() VoucherType {
	return VoucherType{Kind: inv.Kind, Letter: inv.Type}
}

// Credited reports whether an issued credit note cancels the invoice.
func (inv Invoice) Credited() bool {
	return inv.CreditNote != nil && inv.CreditNote.Status == InvoiceStatusIssued
}

// ARCACredentialStatus describes the certificate an account uploaded, never the key.
type ARCACredentialStatus struct {
	Environment ARCAEnvironment
	CUIT        string
	Subject     string
	ExpiresAt   time.Time
	UpdatedAt   time.Time
}

// AccountFiscal is the account's side of an invoice.
type AccountFiscal struct {
	Profile          InvoiceIssuerProfile
	Name             string
	LegalName        *string
	TaxID            *string
	IVACondition     *IVACondition
	PricesIncludeVAT bool
}

// CUIT is the account's tax id as ARCA reads it: 11 digits, or empty when it is not one.
func (a AccountFiscal) CUIT() string {
	if a.TaxID == nil {
		return ""
	}
	digits := DigitsOnly(*a.TaxID)
	if len(digits) != 11 {
		return ""
	}
	return digits
}

// ClientFiscal is the buyer's side of an invoice.
type ClientFiscal struct {
	Address      *string
	ID           uuid.UUID
	Name         *string
	LegalName    *string
	TaxID        *string // digits only.
	IVACondition *IVACondition
}

// ClientFiscalUpdate replaces a client's fiscal data; a nil field clears it. A CUIT is never part of
// it: only the customer loads one, by creating their account.
type ClientFiscalUpdate struct {
	Address      *string
	LegalName    *string
	DNI          *string // a consumidor final's DNI; ignored by the store while a CUIT is on file.
	IVACondition *IVACondition
}

// BranchPointOfSale is a branch's ARCA point of sale, nil until an administrator sets it.
type BranchPointOfSale struct {
	BranchID    uuid.UUID
	BranchName  string
	IsActive    bool
	PointOfSale *int
}

// InvoicingSettings is the account's invoicing setup as the settings screen shows it.
type InvoicingSettings struct {
	Enabled     bool
	Environment ARCAEnvironment
	Account     AccountFiscal
	Branches    []BranchPointOfSale
	Credential  *ARCACredentialStatus
	// InvoiceOnPayment ticks «Facturar también» by default when a sale is marked paid.
	InvoiceOnPayment bool
}

// InvoicingSettingsUpdate replaces the account's fiscal settings and the given branches' points of sale.
type InvoicingSettingsUpdate struct {
	Profile          *InvoiceIssuerProfile
	IVACondition     *IVACondition
	PricesIncludeVAT bool
	PointsOfSale     map[uuid.UUID]*int
	// InvoiceOnPayment is left as it is when nil.
	InvoiceOnPayment *bool
}

// InvoicePreview is the invoice an accepted quote would produce, with whatever stops it.
type InvoicePreview struct {
	Environment ARCAEnvironment // where an invoice issued now would be authorized.
	IssuerCUIT  string
	Fingerprint string
	VersionID   uuid.UUID // the quote version the preview was computed from.
	Type        InvoiceType
	PointOfSale *int
	Receiver    InvoiceReceiver
	Amounts     InvoiceAmounts
	Currency    string
	Issues      []string
	Invoice     *Invoice // the invoice already issued or in flight, if any; never a credited one.
	// CreditedInvoice is the newest invoice of the sale an issued credit note cancelled in this
	// environment: history to show, and the one a new invoice follows.
	CreditedInvoice *Invoice
	// OtherEnvironmentInvoice is the newest invoice issued for the sale in the other ARCA environment:
	// history to show, never a hold on this one.
	OtherEnvironmentInvoice *Invoice
	// CustomerSignupURL is the supplier's customer sign-up page, set only while the receiver lacks
	// the CUIT or document the invoice needs.
	CustomerSignupURL string
}

// DigitsOnly drops every character that is not a decimal digit.
func DigitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mustLoadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// InvoicePDFRenderer renders an authorized fiscal snapshot.
type InvoicePDFRenderer interface {
	Render(Invoice, *BrandLogo) ([]byte, error)
}

// InvoiceIssuerProfile is the fiscal identity printed on the authorized document.
type InvoiceIssuerProfile struct {
	Address                 string `json:"address"`
	GrossIncomeRegistration string `json:"gross_income_registration"`
	ActivityStartedOn       string `json:"activity_started_on"`
}
