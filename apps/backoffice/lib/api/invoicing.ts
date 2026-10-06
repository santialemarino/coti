// The invoicing vocabulary both halves share: the server read of the settings and the browser's
// reads and writes on an order. No transport here, so either side can import it.

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

// Each one names a gap the preview found. Configuration ones map onto a setup issue of the same name.
export const INVOICE_ISSUES = [
  'INVOICING_DISABLED',
  'QUOTE_NOT_ACCEPTED',
  'QUOTE_EMPTY',
  'QUOTE_CURRENCY',
  'ACCOUNT_IVA_CONDITION',
  'ACCOUNT_TAX_ID',
  'BRANCH_POINT_OF_SALE',
  'ARCA_CREDENTIALS',
  'ARCA_CREDENTIALS_EXPIRED',
  'ARCA_CREDENTIALS_CUIT',
  'RECEIVER_CUIT_REQUIRED',
  'RECEIVER_ID_REQUIRED',
] as const;
export type InvoiceIssue = (typeof INVOICE_ISSUES)[number];

export const POINT_OF_SALE_MIN = 1;
export const POINT_OF_SALE_MAX = 99998;

// --- Raw types (API JSON shape, snake_case) ---

interface InvoicingBranchRaw {
  branch_id: string;
  name: string;
  is_active: boolean;
  point_of_sale: number | null;
}

interface ArcaCredentialRaw {
  cuit: string;
  subject: string;
  expires_at: string;
  updated_at: string;
}

export interface InvoicingSettingsRaw {
  enabled: boolean;
  environment: string;
  legal_name: string | null;
  tax_id: string | null;
  iva_condition: string | null;
  prices_include_vat: boolean;
  branches: InvoicingBranchRaw[] | null;
  credential: ArcaCredentialRaw | null;
}

export type ArcaCredentialResponseRaw = ArcaCredentialRaw;

interface VatAmountRaw {
  rate: string;
  base: string;
  amount: string;
}

interface InvoiceReceiverRaw {
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

export interface InvoiceRaw {
  id: string;
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
}

export interface InvoicePreviewRaw {
  version_id: string;
  type: string;
  point_of_sale: number | null;
  receiver: InvoiceReceiverRaw;
  amounts: InvoiceAmountsRaw;
  currency: string;
  issues: string[] | null;
  invoice: InvoiceRaw | null;
}

export interface ClientFiscalRaw {
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

export interface ArcaCredential {
  cuit: string;
  subject: string;
  expiresAt: string;
  updatedAt: string;
}

export interface InvoicingSettings {
  enabled: boolean;
  homologation: boolean;
  legalName: string | null;
  taxId: string | null;
  ivaCondition: IvaCondition | null;
  pricesIncludeVat: boolean;
  branches: InvoicingBranch[];
  credential: ArcaCredential | null;
}

export interface VatAmount {
  rate: string;
  base: string;
  amount: string;
}

export interface InvoiceReceiver {
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

export interface Invoice {
  id: string;
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
}

export interface InvoicePreview {
  versionId: string;
  type: string;
  pointOfSale: number | null;
  receiver: InvoiceReceiver;
  amounts: InvoiceAmounts;
  currency: string;
  issues: string[];
  invoice: Invoice | null;
}

export interface ClientFiscal {
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

export function mapArcaCredential(raw: ArcaCredentialRaw): ArcaCredential {
  return {
    cuit: raw.cuit,
    subject: raw.subject,
    expiresAt: raw.expires_at,
    updatedAt: raw.updated_at,
  };
}

export function mapInvoicingSettings(raw: InvoicingSettingsRaw): InvoicingSettings {
  return {
    enabled: raw.enabled,
    homologation: raw.environment !== 'production',
    legalName: raw.legal_name,
    taxId: raw.tax_id,
    ivaCondition: ivaConditionOf(raw.iva_condition),
    pricesIncludeVat: raw.prices_include_vat,
    branches: (raw.branches ?? []).map((branch) => ({
      branchId: branch.branch_id,
      name: branch.name,
      isActive: branch.is_active,
      pointOfSale: branch.point_of_sale,
    })),
    credential: raw.credential ? mapArcaCredential(raw.credential) : null,
  };
}

function mapReceiver(raw: InvoiceReceiverRaw): InvoiceReceiver {
  return {
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
  };
}

export function mapInvoicePreview(raw: InvoicePreviewRaw): InvoicePreview {
  return {
    versionId: raw.version_id,
    type: raw.type,
    pointOfSale: raw.point_of_sale,
    receiver: mapReceiver(raw.receiver),
    amounts: mapAmounts(raw.amounts),
    currency: raw.currency,
    issues: raw.issues ?? [],
    invoice: raw.invoice ? mapInvoice(raw.invoice) : null,
  };
}

export function mapClientFiscal(raw: ClientFiscalRaw): ClientFiscal {
  return {
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
