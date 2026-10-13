import { beforeEach, describe, expect, it, vi } from 'vitest';

import { reconnectARCA } from '@/app/(protected)/settings/invoicing/arca/actions';
import { ApiError } from '@/lib/api/errors';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('reconnectARCA', () => {
  it('removes the old connection, then requests a new one', async () => {
    vi.mocked(apiRequest).mockResolvedValue(undefined);

    await expect(reconnectARCA()).resolves.toEqual({ ok: true });
    expect(vi.mocked(apiRequest).mock.calls.map(([request]) => request.method)).toEqual([
      'DELETE',
      'POST',
    ]);
  });

  it('requests nothing new while the old connection cannot go', async () => {
    vi.mocked(apiRequest).mockRejectedValueOnce(new ApiError('INVOICE_PENDING', 409));

    await expect(reconnectARCA()).resolves.toEqual({ error: 'INVOICE_PENDING' });
    expect(apiRequest).toHaveBeenCalledOnce();
  });
});
