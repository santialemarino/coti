export type ContactKind = 'WHATSAPP' | 'EMAIL';
export interface CustomerContact {
  kind: ContactKind;
  value: string;
  primary: boolean;
}
export interface CustomerProfileRaw {
  client_id: string | null;
  review_pending: boolean;
  tax_id: string;
  name: string;
  contacts: CustomerContact[];
}
export interface CustomerProfile {
  clientId: string | null;
  reviewPending: boolean;
  taxId: string;
  name: string;
  contacts: CustomerContact[];
}

export function mapCustomerProfile(raw: CustomerProfileRaw): CustomerProfile {
  return {
    clientId: raw.client_id,
    reviewPending: raw.review_pending,
    taxId: raw.tax_id,
    name: raw.name,
    contacts: raw.contacts.map((contact) => ({ ...contact })),
  };
}
