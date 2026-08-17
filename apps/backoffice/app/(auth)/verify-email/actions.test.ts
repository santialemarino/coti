import { beforeEach, describe, expect, it, vi } from 'vitest';

import { confirmEmail, resendVerification } from '@/app/(auth)/verify-email/actions';

// Only the request: the error vocabulary is what turns a refusal into a code, and that is what
// this file is about.
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

function submission(token: string): FormData {
  const data = new FormData();
  data.set('token', token);
  return data;
}

beforeEach(() => vi.clearAllMocks());

describe('confirmEmail', () => {
  it('posts the token to the public route with no bearer', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(confirmEmail({}, submission('t0ken'))).resolves.toEqual({ done: true });
    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/public/auth/verify-email',
      method: 'POST',
      authenticated: false,
    });
  });

  // Unknown, expired, already used and wrong-typed all arrive as one code, the way the API keeps
  // them together — nothing here should tell them apart.
  it('carries a rejected link through', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_LINK', 401));

    await expect(confirmEmail({}, submission('t0ken'))).resolves.toEqual({ error: 'INVALID_LINK' });
  });

  it('reads an absent token as a rejected link without asking the API', async () => {
    await expect(confirmEmail({}, submission(''))).resolves.toEqual({ error: 'INVALID_LINK' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  // The route carries its own allowance, and spending it used to read as "unexpected".
  it('reports the allowance running out as itself', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('RATE_LIMITED', 429));

    await expect(confirmEmail({}, submission('t0ken'))).resolves.toEqual({ error: 'RATE_LIMITED' });
  });
});

describe('resendVerification', () => {
  it('answers the same whether or not the address is registered', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(resendVerification('ana@corralonsanmartin.test')).resolves.toEqual({ sent: true });
  });

  it('reports the mail allowance running out as itself', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('RATE_LIMITED', 429));

    await expect(resendVerification('ana@corralonsanmartin.test')).resolves.toEqual({
      error: 'RATE_LIMITED',
      field: undefined,
    });
  });

  it('lands a rejected body on the address, which is the only field there is', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_BODY', 400));

    await expect(resendVerification('ana@corralonsanmartin.test')).resolves.toEqual({
      error: 'INVALID_BODY',
      field: 'email',
    });
  });

  it('never asks with a blank address', async () => {
    await expect(resendVerification('')).resolves.toEqual({
      error: 'INVALID_BODY',
      field: 'email',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
