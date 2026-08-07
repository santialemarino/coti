import 'server-only';

import { ApiError, codeForStatus, knownErrorCode } from '@/lib/api/errors';
import { getActiveBranchId } from '@/lib/auth/branch';
import { clientAddress } from '@/lib/auth/client-address';
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

export interface ApiRequest {
  path: string;
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE';
  // Sent as JSON. Keys stay snake_case here: this is the wire contract, not app code.
  body?: unknown;
  formData?: FormData;
  query?: Record<string, string | undefined>;
  // Pins the request to one branch instead of the caller's active one, for a write that must
  // land where it was prepared even if the branch changed under it.
  branchId?: string;
  // Off for the public routes, which have no session to attach.
  authenticated?: boolean;
  /*
   * Off for the two reads that have to survive a branch the API would refuse: identity, where
   * a 403 would read as a dead session and sign the caller out, and the branch list, which is
   * the only way back from a branch they can no longer reach.
   */
  branchScoped?: boolean;
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
    throw new ApiError('INTERNAL', response.status, 'response body is not json');
  }
}

// The escape hatch for a caller needing the raw response: a download reading
// Content-Disposition, or a body that is not JSON.
export async function apiFetch(request: ApiRequest): Promise<Response> {
  const {
    path,
    method = 'GET',
    body,
    formData,
    query,
    branchId,
    authenticated = true,
    branchScoped = true,
  } = request;

  const headers = new Headers();
  if (authenticated) {
    const token = await getAccessToken();
    // No token means the proxy let a public route through, or the session expired
    // between the gate and here. Either way it is the same answer the API gives.
    if (!token) throw new ApiError('UNAUTHENTICATED', 401);
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (branchScoped) {
    const branch = branchId ?? (await getActiveBranchId());
    if (branch) headers.set('X-Branch-Id', branch);
  }
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  // Without this the API counts every user's unauthenticated requests against this server's
  // address, so one allowance covers the whole product.
  const caller = await clientAddress();
  if (caller) headers.set('X-Forwarded-For', caller);

  try {
    return await fetch(`${API_URL}${path}${buildQuery(query)}`, {
      method,
      headers,
      body: formData ?? (body === undefined ? undefined : JSON.stringify(body)),
      cache: 'no-store',
    });
  } catch {
    throw new ApiError('UNREACHABLE', 0);
  }
}

/*
 * toApiError reads the API's {error, code, detail} envelope: `code` is the contract a screen
 * branches on, `error` and `detail` are English prose kept for the log and never rendered.
 * The status decides only when no code arrived — which the four aborts in the API's auth
 * middleware still do.
 */
export async function toApiError(response: Response): Promise<ApiError> {
  let code: string | undefined;
  let detail = '';
  try {
    const payload = (await response.json()) as { error?: string; code?: string; detail?: string };
    code = payload.code;
    detail = [payload.error, payload.detail].filter(Boolean).join(': ');
  } catch {
    // A body that is not the envelope tells us nothing extra; the status still does.
  }
  return new ApiError(
    knownErrorCode(code) ?? codeForStatus(response.status),
    response.status,
    detail || undefined,
  );
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
