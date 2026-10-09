package services

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Issue codes an invoice preview reports. The frontend words each one and links to its fix.
const (
	invoiceIssueDisabled           = "INVOICING_DISABLED"
	invoiceIssueNotAccepted        = "QUOTE_NOT_ACCEPTED"
	invoiceIssueEmpty              = "QUOTE_EMPTY"
	invoiceIssueCurrency           = "QUOTE_CURRENCY"
	invoiceIssueIVACondition       = "ACCOUNT_IVA_CONDITION"
	invoiceIssueTaxID              = "ACCOUNT_TAX_ID"
	invoiceIssuePointOfSale        = "BRANCH_POINT_OF_SALE"
	invoiceIssueCredentials        = "ARCA_CREDENTIALS"
	invoiceIssueCredentialsExpired = "ARCA_CREDENTIALS_EXPIRED"
	invoiceIssueCredentialsCUIT    = "ARCA_CREDENTIALS_CUIT"
	invoiceIssueReceiverCUIT       = "RECEIVER_CUIT_REQUIRED"
	invoiceIssueReceiverID         = "RECEIVER_ID_REQUIRED"
	invoiceIssueTotalMismatch      = "QUOTE_TOTAL_MISMATCH"
)

// arcaCurrency is the only currency invoices are issued in for now.
const arcaCurrency = "ARS"

// vatRateOrder fixes the order rates appear in, so a breakdown never reshuffles between reads.
var vatRateOrder = []domain.VATRate{
	domain.VATRateTwentyOne, domain.VATRateTenFive, domain.VATRateTwentySeven, domain.VATRateFive,
	domain.VATRateTwoFive, domain.VATRateZero, domain.VATRateExempt,
}

// grossByRate spreads the version's discounts over its lines and adds the result up per rate:
// item discounts over the lines they name, then total discounts over every line, in proportion.
func grossByRate(
	items []domain.QuoteItem, rates map[uuid.UUID]domain.VATRate, discounts []domain.QuoteDiscount,
) map[domain.VATRate]decimal.Decimal {
	order := make([]uuid.UUID, 0, len(items))
	amounts := make(map[uuid.UUID]decimal.Decimal, len(items))
	for _, item := range items {
		if !item.Subtotal.Valid {
			continue
		}
		order = append(order, item.ID)
		amounts[item.ID] = item.Subtotal.Decimal
	}

	var total []domain.QuoteDiscount
	for _, discount := range discounts {
		if discount.SuppressedBySeller {
			continue
		}
		if discount.Scope == domain.DiscountScopeTotal {
			total = append(total, discount)
			continue
		}
		subtractShare(amounts, discount.ItemIDs, discount.Amount)
	}
	for _, discount := range total {
		subtractShare(amounts, order, discount.Amount)
	}

	byRate := make(map[domain.VATRate]decimal.Decimal)
	for _, item := range items {
		amount, ok := amounts[item.ID]
		if !ok {
			continue
		}
		rate := domain.DefaultVATRate
		if item.ProductID != nil {
			if r, found := rates[*item.ProductID]; found {
				rate = r
			}
		}
		byRate[rate] = byRate[rate].Add(amount)
	}
	return byRate
}

// sumGross adds up what every rate collects.
func sumGross(byRate map[domain.VATRate]decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, amount := range byRate {
		total = total.Add(amount)
	}
	return total
}

// subtractShare takes amount off the named lines in proportion to what each still carries. The
// last line absorbs the rounding, so the shares always add up to the discount.
func subtractShare(amounts map[uuid.UUID]decimal.Decimal, ids []uuid.UUID, amount decimal.Decimal) {
	var lines []uuid.UUID
	base := decimal.Zero
	for _, id := range ids {
		if value, ok := amounts[id]; ok && value.IsPositive() {
			lines = append(lines, id)
			base = base.Add(value)
		}
	}
	if len(lines) == 0 || !amount.IsPositive() {
		return
	}
	if amount.GreaterThan(base) {
		amount = base
	}
	remaining := amount
	for i, id := range lines {
		share := remaining
		if i < len(lines)-1 {
			share = amount.Mul(amounts[id]).Div(base).Round(domain.MoneyScale)
		}
		amounts[id] = amounts[id].Sub(share)
		remaining = remaining.Sub(share)
	}
}

// invoiceAmounts splits what each rate collects into base and IVA, dividing it out of prices that
// carry it or adding it on top; a C invoice breaks nothing out.
func invoiceAmounts(
	byRate map[domain.VATRate]decimal.Decimal, invoiceType domain.InvoiceType, pricesIncludeVAT bool,
) domain.InvoiceAmounts {
	amounts := domain.InvoiceAmounts{Net: decimal.Zero, Exempt: decimal.Zero, VAT: decimal.Zero}
	if !invoiceType.DiscriminatesVAT() {
		for _, gross := range byRate {
			amounts.Net = amounts.Net.Add(gross)
		}
		amounts.Net = amounts.Net.Round(domain.MoneyScale)
		amounts.Total = amounts.Net
		return amounts
	}

	hundred := decimal.NewFromInt(100)
	for _, rate := range vatRateOrder {
		gross, ok := byRate[rate]
		if !ok || gross.IsZero() {
			continue
		}
		if rate == domain.VATRateExempt {
			amounts.Exempt = amounts.Exempt.Add(gross)
			continue
		}
		fraction := rate.Percent().Div(hundred)
		var base, vat decimal.Decimal
		if pricesIncludeVAT {
			base = gross.Div(decimal.NewFromInt(1).Add(fraction)).RoundBank(domain.MoneyScale)
			vat = gross.Sub(base)
		} else {
			base = gross
			vat = gross.Mul(fraction).RoundBank(domain.MoneyScale)
		}
		amounts.ByRate = append(amounts.ByRate, domain.VATAmount{Rate: rate, Base: base, Amount: vat})
		amounts.Net = amounts.Net.Add(base)
		amounts.VAT = amounts.VAT.Add(vat)
	}
	amounts.Total = amounts.Net.Add(amounts.VAT).Add(amounts.Exempt)
	return amounts
}

// invoiceTypeFor decides the letter from both parties' standing: a registered issuer bills A to
// another registered or monotributo buyer and B to everyone else; anyone else always bills C.
func invoiceTypeFor(issuer, receiver domain.IVACondition) domain.InvoiceType {
	if issuer != domain.IVAConditionRegistered {
		return domain.InvoiceTypeC
	}
	if receiver == domain.IVAConditionRegistered || receiver == domain.IVAConditionMonotributo {
		return domain.InvoiceTypeA
	}
	return domain.InvoiceTypeB
}

// invoiceReceiver names the buyer from their profile. A sale with no profile, or one without a
// condition, goes to a consumidor final.
func invoiceReceiver(client *domain.ClientFiscal, fallbackName string) domain.InvoiceReceiver {
	receiver := domain.InvoiceReceiver{
		Name:         strings.TrimSpace(fallbackName),
		DocType:      domain.ReceiverDocNone,
		IVACondition: domain.IVAConditionFinalConsumer,
	}
	if client == nil {
		return receiver
	}
	if name := firstNonEmpty(client.LegalName, client.Name); name != "" {
		receiver.Name = name
	}
	if client.IVACondition != nil {
		receiver.IVACondition = *client.IVACondition
	}
	if client.TaxID != nil {
		switch digits := *client.TaxID; {
		case len(digits) == 11:
			receiver.DocType, receiver.DocNumber = domain.ReceiverDocCUIT, digits
		case len(digits) >= 7 && len(digits) <= 8:
			receiver.DocType, receiver.DocNumber = domain.ReceiverDocDNI, digits
		}
	}
	return receiver
}

// invoiceInputs is everything the preview reads before deciding whether an invoice can go out.
type invoiceInputs struct {
	enabled         bool
	quote           domain.Quote
	version         domain.QuoteVersion
	gross           decimal.Decimal // what the lines add up to once their discounts are spread.
	account         domain.AccountFiscal
	pointOfSale     *int
	credential      *domain.ARCACredentialStatus
	receiver        domain.InvoiceReceiver
	amounts         domain.InvoiceAmounts
	invoiceType     domain.InvoiceType
	unidentifiedMax decimal.Decimal
	now             time.Time
}

// invoiceIssues lists every gap that stops an invoice, all at once, so the seller fixes them in one
// pass rather than one refusal at a time.
func invoiceIssues(in invoiceInputs) []string {
	var issues []string
	add := func(issue string) { issues = append(issues, issue) }

	if !in.enabled {
		add(invoiceIssueDisabled)
	}
	if in.quote.CurrentStatus != domain.QuoteStatusAccepted {
		add(invoiceIssueNotAccepted)
	}
	if !in.amounts.Total.IsPositive() {
		add(invoiceIssueEmpty)
	}
	// Discounts that outgrow their lines make the quote's total something no invoice line can carry.
	if !in.gross.Equal(in.version.Total) {
		add(invoiceIssueTotalMismatch)
	}
	if in.version.Currency != "" && in.version.Currency != arcaCurrency {
		add(invoiceIssueCurrency)
	}
	if in.account.IVACondition == nil {
		add(invoiceIssueIVACondition)
	}
	cuit := in.account.CUIT()
	if cuit == "" {
		add(invoiceIssueTaxID)
	}
	if in.pointOfSale == nil {
		add(invoiceIssuePointOfSale)
	}
	switch {
	case in.credential == nil:
		add(invoiceIssueCredentials)
	case !in.credential.ExpiresAt.After(in.now):
		add(invoiceIssueCredentialsExpired)
	case cuit != "" && in.credential.CUIT != cuit:
		add(invoiceIssueCredentialsCUIT)
	}

	switch {
	case in.invoiceType == domain.InvoiceTypeA && in.receiver.DocType != domain.ReceiverDocCUIT:
		add(invoiceIssueReceiverCUIT)
	case in.receiver.DocType == domain.ReceiverDocNone && in.amounts.Total.GreaterThanOrEqual(in.unidentifiedMax):
		add(invoiceIssueReceiverID)
	}
	return issues
}

// firstNonEmpty returns the first value that is set and not blank.
func firstNonEmpty(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}
	return ""
}
