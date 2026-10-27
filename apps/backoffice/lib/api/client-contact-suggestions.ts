export interface ContactSuggestionRaw {
  id: string;
  source_client_id: string | null;
  candidate_client_id: string | null;
  candidate_name: string | null;
  tax_id: string | null;
  kind: 'WHATSAPP' | 'EMAIL';
  value: string;
  declared_name: string | null;
  original_text: string;
  created_at: string;
}
export interface ContactSuggestion {
  id: string;
  sourceClientId: string | null;
  candidateClientId: string | null;
  candidateName: string | null;
  taxId: string | null;
  kind: 'WHATSAPP' | 'EMAIL';
  value: string;
  declaredName: string | null;
  originalText: string;
  createdAt: string;
}
export function mapContactSuggestion(raw: ContactSuggestionRaw): ContactSuggestion {
  return {
    id: raw.id,
    sourceClientId: raw.source_client_id,
    candidateClientId: raw.candidate_client_id,
    candidateName: raw.candidate_name,
    taxId: raw.tax_id,
    kind: raw.kind,
    value: raw.value,
    declaredName: raw.declared_name,
    originalText: raw.original_text,
    createdAt: raw.created_at,
  };
}
