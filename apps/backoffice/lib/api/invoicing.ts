// The invoicing vocabulary both halves share: the server read of the settings and the browser's
// reads and writes on an order. No transport here, so either side can import it.

import { formatTaxId } from '@/lib/utils/tax-id';

export const IVA_CONDITIONS = ['REGISTERED', 'MONOTRIBUTO', 'EXEMPT', 'FINAL_CONSUMER'] as const;
export type IvaCondition = (typeof IVA_CONDITIONS)[number];

export const VAT_RATES = [
  'VAT_21',
  'VAT_10_5',
  'VAT_27',
  'VAT_5',
  'VAT_2_5',
  'VAT_0',
  'EXEMPT',
] as const;
export type VatRate = (typeof VAT_RATES)[number];

export const DEFAULT_VAT_RATE: VatRate = 'VAT_21';

// A product's rate from the wire, defaulting a value this app does not know to the account's norm.
export function vatRateOf(value: string | null | undefined): VatRate {
  return VAT_RATES.find((rate) => rate === value) ?? DEFAULT_VAT_RATE;
}

// ARCA's two deployments. HOMOLOGATION is its test environment: what it authorizes has no fiscal
// validity, so everything made there is marked as a test.
export const ARCA_ENVIRONMENTS = ['HOMOLOGATION', 'PRODUCTION'] as const;
export type ARCAEnvironment = (typeof ARCA_ENVIRONMENTS)[number];

// A value this app does not know reads as a test, so the mark is never dropped by mistake.
export function arcaEnvironmentOf(value: string | null | undefined): ARCAEnvironment {
  return value === 'PRODUCTION' ? 'PRODUCTION' : 'HOMOLOGATION';
}

// The point-of-sale range ARCA accepts, mirrored from the API.
export const POINT_OF_SALE_MIN = 1;
export const POINT_OF_SALE_MAX = 99998;

// --- Raw types (API JSON shape, snake_case) ---

interface InvoicingBranchRaw {
  branch_id: string;
  name: string;
  is_active: boolean;
  point_of_sale: number | null;
}

export interface InvoicingSettingsRaw {
  profile?: { address: string; gross_income_registration: string; activity_started_on: string };
  legal_name: string | null;
  tax_id: string | null;
  iva_condition: string | null;
  prices_include_vat: boolean;
  branches: InvoicingBranchRaw[] | null;
  invoice_on_payment: boolean;
}

interface VatAmountRaw {
  rate: string;
  base: string;
  amount: string;
}

interface InvoiceReceiverRaw {
  address?: string;
  name: string;
  doc_type: string;
  doc_number: string;
  iva_condition: string;
}

interface InvoiceAmountsRaw {
  net: string;
  exempt: string;
  vat: string;
  total: string;
  by_rate: VatAmountRaw[] | null;
}

interface VoucherReferenceRaw {
  id: string;
  type: string;
  point_of_sale: number;
  number: number | null;
  issued_on: string;
}

export interface InvoiceRaw {
  id: string;
  branch_id: string;
  kind: string;
  environment: string;
  status: string;
  type: string;
  point_of_sale: number;
  number: number | null;
  issued_on: string;
  cae: string | null;
  cae_expires_on: string | null;
  issuer_cuit: string;
  receiver: InvoiceReceiverRaw;
  amounts: InvoiceAmountsRaw;
  currency: string;
  issues: string[] | null;
  qr_url: string;
  created_at: string;
  credit_note?: InvoiceRaw | null;
  credited_invoice?: VoucherReferenceRaw | null;
}

export interface InvoicePreviewRaw {
  environment: string;
  issuer_cuit: string;
  fingerprint: string;
  version_id: string;
  type: string;
  point_of_sale: number | null;
  receiver: InvoiceReceiverRaw;
  amounts: InvoiceAmountsRaw;
  currency: string;
  issues: string[] | null;
  invoice: InvoiceRaw | null;
  other_environment_invoice?: InvoiceRaw | null;
  credited_invoice?: InvoiceRaw | null;
  customer_signup_url?: string;
}

export interface ClientFiscalRaw {
  address?: string | null;
  id: string;
  name: string | null;
  legal_name: string | null;
  tax_id: string | null;
  iva_condition: string | null;
}

// --- Frontend types (camelCase) ---

export interface InvoicingBranch {
  branchId: string;
  name: string;
  isActive: boolean;
  pointOfSale: number | null;
}

export interface InvoicingSettings {
  profile?: { address: string; grossIncomeRegistration: string; activityStartedOn: string };
  legalName: string | null;
  taxId: string | null;
  ivaCondition: IvaCondition | null;
  pricesIncludeVat: boolean;
  branches: InvoicingBranch[];
  // Whether «Marcar cobrado» starts with «Facturar también» ticked.
  invoiceOnPayment: boolean;
}

export interface VatAmount {
  rate: string;
  base: string;
  amount: string;
}

export interface InvoiceReceiver {
  address?: string;
  name: string;
  docType: string;
  docNumber: string;
  ivaCondition: string;
}

export interface InvoiceAmounts {
  net: string;
  exempt: string;
  vat: string;
  total: string;
  byRate: VatAmount[];
}

// An invoice bills a sale; a credit note cancels one invoice in full.
export type VoucherKind = 'INVOICE' | 'CREDIT_NOTE';

// An authorized voucher as another one names it.
export interface VoucherReference {
  id: string;
  type: string;
  pointOfSale: number;
  number: number | null;
  issuedOn: string;
}

export interface Invoice {
  id: string;
  branchId: string;
  kind: VoucherKind;
  environment: ARCAEnvironment;
  status: 'PENDING' | 'ISSUED' | 'REJECTED';
  type: string;
  pointOfSale: number;
  number: number | null;
  issuedOn: string;
  cae: string | null;
  caeExpiresOn: string | null;
  issuerCuit: string;
  receiver: InvoiceReceiver;
  amounts: InvoiceAmounts;
  currency: string;
  // ARCA's own reasons for a refusal, shown verbatim.
  issues: string[];
  qrUrl: string;
  createdAt: string;
  // An invoice's newest credit note: an ISSUED one cancels the invoice.
  creditNote: Invoice | null;
  // The invoice a credit note cancels.
  creditedInvoice: VoucherReference | null;
}

export interface InvoicePreview {
  // Where an invoice issued now would be authorized.
  environment: ARCAEnvironment;
  issuerCuit: string;
  fingerprint: string;
  versionId: string;
  type: string;
  pointOfSale: number | null;
  receiver: InvoiceReceiver;
  amounts: InvoiceAmounts;
  currency: string;
  issues: string[];
  invoice: Invoice | null;
  // The newest invoice issued for the sale in the other ARCA environment: history, never a hold.
  otherEnvironmentInvoice: Invoice | null;
  // The newest invoice of the sale a credit note cancelled here, with that note: history.
  creditedInvoice: Invoice | null;
  // The supplier's customer sign-up page, while the invoice waits on the buyer's CUIT or document.
  customerSignupUrl: string | null;
}

export interface ClientFiscal {
  address?: string | null;
  id: string;
  name: string | null;
  legalName: string | null;
  taxId: string | null;
  ivaCondition: IvaCondition | null;
}

// --- Mappers ---

function ivaConditionOf(value: string | null): IvaCondition | null {
  return IVA_CONDITIONS.find((condition) => condition === value) ?? null;
}

export function mapInvoicingSettings(raw: InvoicingSettingsRaw): InvoicingSettings {
  return {
    profile: raw.profile
      ? {
          address: raw.profile.address,
          grossIncomeRegistration: raw.profile.gross_income_registration,
          activityStartedOn: raw.profile.activity_started_on,
        }
      : undefined,
    legalName: raw.legal_name,
    taxId: raw.tax_id,
    ivaCondition: ivaConditionOf(raw.iva_condition),
    pricesIncludeVat: raw.prices_include_vat,
    invoiceOnPayment: raw.invoice_on_payment,
    branches: (raw.branches ?? []).map((branch) => ({
      branchId: branch.branch_id,
      name: branch.name,
      isActive: branch.is_active,
      pointOfSale: branch.point_of_sale,
    })),
  };
}

function mapReceiver(raw: InvoiceReceiverRaw): InvoiceReceiver {
  return {
    address: raw.address,
    name: raw.name,
    docType: raw.doc_type,
    docNumber: raw.doc_number,
    ivaCondition: raw.iva_condition,
  };
}

function mapAmounts(raw: InvoiceAmountsRaw): InvoiceAmounts {
  return {
    net: raw.net,
    exempt: raw.exempt,
    vat: raw.vat,
    total: raw.total,
    byRate: (raw.by_rate ?? []).map((row) => ({
      rate: row.rate,
      base: row.base,
      amount: row.amount,
    })),
  };
}

export function mapInvoice(raw: InvoiceRaw): Invoice {
  return {
    id: raw.id,
    branchId: raw.branch_id,
    kind: raw.kind === 'CREDIT_NOTE' ? 'CREDIT_NOTE' : 'INVOICE',
    environment: arcaEnvironmentOf(raw.environment),
    status: raw.status as Invoice['status'],
    type: raw.type,
    pointOfSale: raw.point_of_sale,
    number: raw.number,
    issuedOn: raw.issued_on,
    cae: raw.cae,
    caeExpiresOn: raw.cae_expires_on,
    issuerCuit: raw.issuer_cuit,
    receiver: mapReceiver(raw.receiver),
    amounts: mapAmounts(raw.amounts),
    currency: raw.currency,
    issues: raw.issues ?? [],
    qrUrl: raw.qr_url,
    createdAt: raw.created_at,
    creditNote: raw.credit_note ? mapInvoice(raw.credit_note) : null,
    creditedInvoice: raw.credited_invoice
      ? {
          id: raw.credited_invoice.id,
          type: raw.credited_invoice.type,
          pointOfSale: raw.credited_invoice.point_of_sale,
          number: raw.credited_invoice.number,
          issuedOn: raw.credited_invoice.issued_on,
        }
      : null,
  };
}

// Whether an issued credit note cancels the invoice.
export function isCredited(invoice: Invoice): boolean {
  return invoice.creditNote?.status === 'ISSUED';
}

export function mapInvoicePreview(raw: InvoicePreviewRaw): InvoicePreview {
  return {
    environment: arcaEnvironmentOf(raw.environment),
    issuerCuit: raw.issuer_cuit,
    fingerprint: raw.fingerprint,
    versionId: raw.version_id,
    type: raw.type,
    pointOfSale: raw.point_of_sale,
    receiver: mapReceiver(raw.receiver),
    amounts: mapAmounts(raw.amounts),
    currency: raw.currency,
    issues: raw.issues ?? [],
    invoice: raw.invoice ? mapInvoice(raw.invoice) : null,
    otherEnvironmentInvoice: raw.other_environment_invoice
      ? mapInvoice(raw.other_environment_invoice)
      : null,
    creditedInvoice: raw.credited_invoice ? mapInvoice(raw.credited_invoice) : null,
    customerSignupUrl: raw.customer_signup_url || null,
  };
}

export function mapClientFiscal(raw: ClientFiscalRaw): ClientFiscal {
  return {
    address: raw.address,
    id: raw.id,
    name: raw.name,
    legalName: raw.legal_name,
    taxId: raw.tax_id,
    ivaCondition: ivaConditionOf(raw.iva_condition),
  };
}

// --- Formatting ---

const POINT_OF_SALE_DIGITS = 4;
const POINT_OF_SALE_WIDE_DIGITS = 5;
const INVOICE_NUMBER_DIGITS = 8;
const WIDE_POINT_OF_SALE = 10000;

/*
 * The number as Argentine invoices print it: the point of sale zero-padded to four digits (five
 * once it outgrows them) and the sequence to eight, joined by a hyphen — 0003-00000023.
 */
export function formatInvoiceNumber(pointOfSale: number, number: number): string {
  const posDigits =
    pointOfSale >= WIDE_POINT_OF_SALE ? POINT_OF_SALE_WIDE_DIGITS : POINT_OF_SALE_DIGITS;
  return `${String(pointOfSale).padStart(posDigits, '0')}-${String(number).padStart(INVOICE_NUMBER_DIGITS, '0')}`;
}

// A CUIT as it is written: 30-70123456-8. Anything that is not eleven digits reads as it came.
export function formatCuit(digits: string): string {
  return /^\d{11}$/.test(digits)
    ? `${digits.slice(0, 2)}-${digits.slice(2, 10)}-${digits.slice(10)}`
    : digits;
}

// A DNI as it is written: 12.345.678. Anything that is not seven or eight digits reads as it came.
export function formatDni(digits: string): string {
  return /^\d{7,8}$/.test(digits) ? formatTaxId(digits, 'dni') : digits;
}
