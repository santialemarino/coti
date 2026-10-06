import { unstable_rethrow } from 'next/navigation';

/*
 * The error vocabulary the interface renders. It is the API's own `code` — the contract in
 * docs/technical/api-specification.md, "The error envelope" — plus the two the client decides
 * for itself, because the API cannot answer them: a request that never reached it, and a
 * session a re-check confirmed is over.
 *
 * A code names which rule refused the request, which the status alone cannot: one route
 * answers 422 for several reasons and the screens differ.
 */
export const API_ERROR_CODES = [
  'NOT_FOUND',
  'CONFLICT',
  'INVALID_INPUT',
  'INVALID_BODY',
  'UNAUTHENTICATED',
  'FORBIDDEN',
  'IMMUTABLE',
  'ACCOUNT_LOCKED',
  'EMAIL_NOT_VERIFIED',
  'RATE_LIMITED',
  'FILE_TOO_LARGE',
  'EMAIL_TAKEN',
  'LAST_ACTIVE_BRANCH',
  'SELF_DEACTIVATION',
  'SELF_ROLE_CHANGE',
  'SELF_EMAIL_CHANGE',
  'PASSWORD_POLICY',
  'INVALID_LINK',
  'INVITE_NOT_PENDING',
  'MAIL_NOT_CONFIGURED',
  'BRANCH_NOT_ACCESSIBLE',
  'BRANCH_MAILBOX_REQUIRED',
  'QUOTE_ARCHIVED',
  'QUOTE_NOT_DRAFT',
  'QUOTE_NOT_REACTIVATABLE',
  'QUOTE_NOT_SENDABLE',
  'DELIVERY_CHANNEL',
  'CHANNEL_IDENTIFIER',
  'DELIVERY_UNAVAILABLE',
  'NOT_CONFIGURED',
  'UNSUPPORTED_FILE_TYPE',
  'LEGACY_EXCEL_FILE',
  'AI_UNAVAILABLE',
  'INVOICE_NOT_READY',
  'INVOICE_REJECTED',
  'INVOICE_IN_PROGRESS',
  'INVOICE_STALE',
  'POINT_OF_SALE_TAKEN',
  'QUOTE_ALREADY_INVOICED',
  'ARCA_CREDENTIALS',
  'INVOICING_UNAVAILABLE',
  'INVALID_TAX_ID',
  'INTERNAL',
  'UNREACHABLE',
  'SESSION_EXPIRED',
] as const;

export type ApiErrorCode = (typeof API_ERROR_CODES)[number];

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  // What a refusal lists beside its code: the gaps that stop an invoice, or ARCA's own reasons.
  readonly issues: string[];

  constructor(code: ApiErrorCode, status: number, message?: string, issues: string[] = []) {
    super(message ?? code);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.issues = issues;
  }
}

// A redirect or a notFound thrown under an action's try is rethrown, so the catch that words a
// refusal cannot swallow the navigation instead.
export function errorCodeOf(error: unknown): ApiErrorCode {
  unstable_rethrow(error);
  return error instanceof ApiError ? error.code : 'INTERNAL';
}

/*
 * The code a status implies, for a response carrying none — an error the delivery layer
 * writes before a handler is reached, or a proxy answering on the API's behalf.
 */
export function codeForStatus(status: number): ApiErrorCode {
  switch (status) {
    case 400:
      return 'INVALID_BODY';
    case 401:
      return 'UNAUTHENTICATED';
    case 403:
      return 'FORBIDDEN';
    case 404:
      return 'NOT_FOUND';
    case 409:
      return 'CONFLICT';
    case 413:
      return 'FILE_TOO_LARGE';
    case 422:
      return 'INVALID_INPUT';
    case 429:
      return 'RATE_LIMITED';
    default:
      return 'INTERNAL';
  }
}

/*
 * Narrows what arrived on the wire, so a code added to the API before this app knows it falls
 * back to the status rather than reaching a catalog that has no wording for it.
 */
export function knownErrorCode(value: unknown): ApiErrorCode | undefined {
  return API_ERROR_CODES.find((code) => code === value);
}
