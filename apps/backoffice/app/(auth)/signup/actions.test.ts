import { beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { signup } from '@/app/(auth)/signup/actions';
import { type SignupValues } from '@/app/(auth)/signup/form-schema';
import { ROUTES } from '@/config/routes';
import { ACCESS_COOKIE, REFRESH_COOKIE, REMEMBER_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
// Only the request: the error vocabulary is what maps a status onto a rejection, so mocking it
// too would leave the mapping this file is about untested.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { cookies } = await import('next/headers');
const { ApiError, apiRequest } = await import('@/lib/api/client');

const VALUES: SignupValues = {
  accountName: 'Corralón San Martín',
  legalName: '',
  taxId: '',
  branchName: 'Villa Bosch',
  branchAddress: '',
  adminName: 'Ana Pérez',
  adminEmail: 'ana@corralonsanmartin.test',
  adminPassword: 'coti1234',
  confirmPassword: 'coti1234',
};

const TOKENS = { access_token: 'access', refresh_token: 'refresh' };

function jar() {
  const fake = cookieJar();
  vi.mocked(cookies).mockResolvedValue(fake as unknown as Awaited<ReturnType<typeof cookies>>);
  return fake;
}

function bodySent(): Record<string, unknown> {
  const request = vi.mocked(apiRequest).mock.calls[0]?.[0];
  return (request?.body ?? {}) as Record<string, unknown>;
}

beforeEach(() => vi.clearAllMocks());

describe('signup', () => {
  it('posts the registration to the public route with no bearer', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue({ tokens: TOKENS });

    await signup(VALUES);

    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/public/accounts',
      method: 'POST',
      // There is no session yet, and the client throws `unauthenticated` before it ever
      // reaches the network when it is told to attach one.
      authenticated: false,
    });
  });

  it('renames every field onto the wire contract', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue({ tokens: TOKENS });

    await signup({ ...VALUES, legalName: 'San Martín S.A.', taxId: '30-11111111-1' });

    expect(bodySent()).toEqual({
      account_name: 'Corralón San Martín',
      legal_name: 'San Martín S.A.',
      tax_id: '30-11111111-1',
      branch_name: 'Villa Bosch',
      branch_address: undefined,
      admin_name: 'Ana Pérez',
      admin_email: 'ana@corralonsanmartin.test',
      admin_password: 'coti1234',
    });
  });

  /*
   * The API's optional fields are pointers with `omitempty`, which only skips a nil one — a
   * pointer to "" passes validation and lands in the column. So a field the caller left blank
   * has to be absent from the body, not empty in it.
   */
  it('omits a blank optional field rather than sending an empty string', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue({ tokens: TOKENS });

    await signup(VALUES);

    const body = bodySent();
    expect(JSON.parse(JSON.stringify(body))).not.toHaveProperty('legal_name');
    expect(JSON.parse(JSON.stringify(body))).not.toHaveProperty('tax_id');
    expect(JSON.parse(JSON.stringify(body))).not.toHaveProperty('branch_address');
  });

  it('opens a session from the pair it is answered with and sends the caller to verify', async () => {
    const store = jar();
    vi.mocked(apiRequest).mockResolvedValue({ tokens: TOKENS });

    await expect(signup(VALUES)).resolves.toEqual({ redirectTo: ROUTES.verifyEmail });
    expect(store.get(ACCESS_COOKIE)?.value).toBe('access');
    expect(store.get(REFRESH_COOKIE)?.value).toBe('refresh');
    // Registration is not a remembered login: the session dies with the browser unless the
    // caller asks for otherwise next time they sign in.
    expect(store.get(REMEMBER_COOKIE)).toBeUndefined();
  });

  it('opens no session when the answer carries no pair', async () => {
    const store = jar();
    vi.mocked(apiRequest).mockResolvedValue({ tokens: { access_token: 'access' } });

    await expect(signup(VALUES)).resolves.toEqual({ error: 'unexpected' });
    expect(store.set).not.toHaveBeenCalled();
  });

  // The address is the one field registration can refuse, and it has to be named: a form-level
  // error would leave the caller re-reading eight fields for the one the API meant.
  it('lands a refused address on the email field', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('conflict', 409));

    await expect(signup(VALUES)).resolves.toEqual({
      fieldError: { field: 'adminEmail', key: 'emailTaken' },
    });
  });

  // Only reachable when the API's own floor sits above the one the form mirrors, which is
  // exactly when the caller needs to be told which field to change.
  it('lands a refused password on the password field', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('unprocessable', 422));

    await expect(signup(VALUES)).resolves.toEqual({
      fieldError: { field: 'adminPassword', key: 'tooShort' },
    });
  });

  it.each([
    ['rateLimited', 429, 'rateLimited'],
    ['unreachable', 0, 'unreachable'],
    ['unexpected', 500, 'unexpected'],
    ['badRequest', 400, 'unexpected'],
  ] as const)('reports a %s as the form error %s', async (code, status, expected) => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError(code, status));

    await expect(signup(VALUES)).resolves.toEqual({ error: expected });
  });

  it('never reaches the API with values its own schema refuses', async () => {
    jar();

    await expect(signup({ ...VALUES, adminEmail: 'nope' })).resolves.toEqual({
      error: 'unexpected',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
