export type ContactKind = 'WHATSAPP' | 'EMAIL';
export interface CustomerContact {
  kind: ContactKind;
  value: string;
  primary: boolean;
  verified?: boolean;
  // The contact account recovery preselects; always a verified one.
  main?: boolean;
}
export interface CustomerProfileRaw {
  client_id: string | null;
  review_pending: boolean;
  ownership_unproved: boolean;
  tax_id: string;
  name: string;
  contacts: CustomerContact[];
  legal_name?: string | null;
  iva_condition?: string | null;
  fiscal_address?: string | null;
}
export interface CustomerProfile {
  legalName?: string | null;
  ivaCondition?: string | null;
  address?: string | null;
  clientId: string | null;
  reviewPending: boolean;
  // An earlier profile with this CUIT stays hidden, fiscal data included, until a contact it had is verified.
  ownershipUnproved: boolean;
  taxId: string;
  name: string;
  contacts: CustomerContact[];
}

export function mapCustomerProfile(raw: CustomerProfileRaw): CustomerProfile {
  return {
    clientId: raw.client_id,
    reviewPending: raw.review_pending,
    ownershipUnproved: raw.ownership_unproved,
    taxId: raw.tax_id,
    name: raw.name,
    legalName: raw.legal_name ?? null,
    ivaCondition: raw.iva_condition ?? null,
    address: raw.fiscal_address ?? null,
    contacts: raw.contacts.map((contact) => ({ ...contact })),
  };
}
