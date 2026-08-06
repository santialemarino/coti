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
  'PASSWORD_POLICY',
  'INVALID_LINK',
  'INTERNAL',
  'UNREACHABLE',
  'SESSION_EXPIRED',
] as const;

export type ApiErrorCode = (typeof API_ERROR_CODES)[number];

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;

  constructor(code: ApiErrorCode, status: number, message?: string) {
    super(message ?? code);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

export function errorCodeOf(error: unknown): ApiErrorCode {
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
