import 'server-only';

import { getAccessToken } from '@/lib/auth/session';
import { API_URL } from '@/lib/config';

/*
 * The one place the backoffice talks to the API. Every screen goes through it, so
 * the base URL, the bearer, the active branch header and the error vocabulary are
 * decided once instead of per screen.
 *
 * It carries transport only. Turning the API's snake_case JSON into the camelCase
 * the component tree uses is each lib/api/<feature> module's job.
 */

// The error vocabulary the interface translates. Every status the API can answer
// maps onto one of these, so no screen ever branches on a raw status code.
export type ApiErrorCode =
  | 'badRequest'
  | 'unauthenticated'
  | 'forbidden'
  | 'notFound'
  | 'conflict'
  | 'unprocessable'
  | 'rateLimited'
  | 'unreachable'
  | 'unexpected';

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

export interface ApiRequest {
  path: string;
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE';
  // Sent as JSON. Keys stay snake_case here: this is the wire contract, not app code.
  body?: unknown;
  formData?: FormData;
  query?: Record<string, string | undefined>;
  // The active branch. What that scope reaches is the backend's decision.
  branchId?: string;
  // Off for the public routes, which have no session to attach.
  authenticated?: boolean;
}

export async function apiRequest<T>(request: ApiRequest): Promise<T> {
  const response = await apiFetch(request);
  if (!response.ok) throw await toApiError(response);

  // Emptiness is read off the body rather than off the status: the API answers 204
  // for a completed write and 202 for an accepted one, and neither carries a body.
  const text = await response.text();
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new ApiError('unexpected', response.status, 'response body is not json');
  }
}

// The escape hatch for a caller needing the raw response: a download reading
// Content-Disposition, or a body that is not JSON.
export async function apiFetch(request: ApiRequest): Promise<Response> {
  const { path, method = 'GET', body, formData, query, branchId, authenticated = true } = request;

  const headers = new Headers();
  if (authenticated) {
    const token = await getAccessToken();
    // No token means middleware let a public route through, or the session expired
    // between the gate and here. Either way it is the same answer the API gives.
    if (!token) throw new ApiError('unauthenticated', 401);
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (branchId) headers.set('X-Branch-Id', branchId);
  if (body !== undefined) headers.set('Content-Type', 'application/json');

  try {
    return await fetch(`${API_URL}${path}${buildQuery(query)}`, {
      method,
      headers,
      body: formData ?? (body === undefined ? undefined : JSON.stringify(body)),
      cache: 'no-store',
    });
  } catch {
    throw new ApiError('unreachable', 0);
  }
}

// toApiError reads the API's {error, detail} envelope for the log, and never lets
// its text reach a screen: the interface renders its own message per code.
async function toApiError(response: Response): Promise<ApiError> {
  let detail = '';
  try {
    const payload = (await response.json()) as { error?: string; detail?: string };
    detail = [payload.error, payload.detail].filter(Boolean).join(': ');
  } catch {
    // A body that is not the envelope tells us nothing extra; the status still does.
  }
  return new ApiError(codeForStatus(response.status), response.status, detail || undefined);
}

function codeForStatus(status: number): ApiErrorCode {
  switch (status) {
    case 400:
      return 'badRequest';
    case 401:
      return 'unauthenticated';
    case 403:
      return 'forbidden';
    case 404:
      return 'notFound';
    case 409:
      return 'conflict';
    case 422:
      return 'unprocessable';
    case 429:
      return 'rateLimited';
    default:
      return 'unexpected';
  }
}

export function errorCodeOf(error: unknown): ApiErrorCode {
  return error instanceof ApiError ? error.code : 'unexpected';
}

function buildQuery(query: ApiRequest['query']): string {
  if (!query) return '';
  const params = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined) params.append(key, value);
  });
  const encoded = params.toString();
  return encoded ? `?${encoded}` : '';
}
