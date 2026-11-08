import { throwApiError } from '@/lib/api/errors';

export interface ContactSuggestionRaw {
  id: string;
  source_client_id: string | null;
  candidate_client_id: string | null;
  candidate_name: string | null;
  source_name: string | null;
  source_registered: boolean;
  candidate_registered: boolean;
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
  sourceName: string | null;
  sourceRegistered: boolean;
  candidateRegistered: boolean;
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
    sourceName: raw.source_name,
    sourceRegistered: raw.source_registered,
    candidateRegistered: raw.candidate_registered,
    taxId: raw.tax_id,
    kind: raw.kind,
    value: raw.value,
    declaredName: raw.declared_name,
    originalText: raw.original_text,
    createdAt: raw.created_at,
  };
}

// A seller merges only two profiles without a CUIT; a registered one belongs to its customer.
export function isMergeable(suggestion: ContactSuggestion): boolean {
  return (
    suggestion.sourceClientId !== null &&
    suggestion.candidateClientId !== null &&
    !suggestion.sourceRegistered &&
    !suggestion.candidateRegistered
  );
}

export async function resolveContactSuggestion(
  id: string,
  decision: { action: 'MERGE'; keepClientId: string } | { action: 'SEPARATE' },
  branchId: string | null,
): Promise<void> {
  const response = await fetch(`/api/client-contact-suggestions/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...(branchId ? { 'X-Branch-Id': branchId } : {}),
    },
    body: JSON.stringify(
      decision.action === 'MERGE'
        ? { action: 'MERGE', keep_client_id: decision.keepClientId }
        : { action: 'SEPARATE' },
    ),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
}
