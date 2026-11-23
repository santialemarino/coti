import { beforeEach, describe, expect, it, vi } from 'vitest';

import { saveWhatsAppChannel } from '@/app/(protected)/settings/branches/[branchId]/channels/actions';
import { getBranchChannels } from '@/lib/api/branch-channels';
import { apiRequest } from '@/lib/api/client';
import { ApiError } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/branch-channels', () => ({ getBranchChannels: vi.fn() }));
vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));

const BRANCH_ID = 'b0000000-0000-4000-8000-000000000002';
const CHANNEL_ID = 'c0000000-0000-4000-8000-000000000001';
const VALUES = {
  identifier: '+15556530731',
  phoneNumberId: '1379080161952101',
  businessAccountId: '3580903055408468',
  accessToken: 'new-test-token',
  webhookVerifyToken: 'new-test-verifier',
  isActive: true,
};
const CHANNEL = {
  id: CHANNEL_ID,
  branchId: BRANCH_ID,
  type: 'WHATSAPP' as const,
  identifier: '+15556530731',
  isActive: true,
  isConfigured: true,
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getBranchChannels).mockResolvedValue([CHANNEL]);
});

describe('saveWhatsAppChannel', () => {
  it('requires an administrator and saves credentials through the branch-scoped API', async () => {
    await expect(saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, VALUES)).resolves.toEqual({ ok: true });
    expect(requireAdmin).toHaveBeenCalledOnce();
    expect(getBranchChannels).toHaveBeenCalledWith(BRANCH_ID);
    expect(apiRequest).toHaveBeenCalledWith({
      path: `/v1/channels/${CHANNEL_ID}/whatsapp`,
      method: 'PATCH',
      branchId: BRANCH_ID,
      body: {
        identifier: VALUES.identifier,
        is_active: true,
        phone_number_id: VALUES.phoneNumberId,
        business_account_id: VALUES.businessAccountId,
        access_token: VALUES.accessToken,
        webhook_verify_token: VALUES.webhookVerifyToken,
      },
    });
  });

  it('does not read or write credentials when the administrator gate rejects the caller', async () => {
    vi.mocked(requireAdmin).mockRejectedValue(new Error('admin gate'));
    await expect(saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, VALUES)).rejects.toThrow('admin gate');
    expect(getBranchChannels).not.toHaveBeenCalled();
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it.each([
    { ...CHANNEL, branchId: 'another-branch' },
    { ...CHANNEL, type: 'EMAIL' as const },
    { ...CHANNEL, id: 'another-channel' },
  ])('refuses a channel outside the selected WhatsApp scope: %j', async (channel) => {
    vi.mocked(getBranchChannels).mockResolvedValue([channel]);
    await expect(saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, VALUES)).resolves.toEqual({
      error: 'NOT_FOUND',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('refuses incomplete replacements without clearing the stored configuration', async () => {
    vi.mocked(getBranchChannels).mockResolvedValue([{ ...CHANNEL, isConfigured: false }]);
    await expect(
      saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, { ...VALUES, accessToken: '' }),
    ).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('rotates only the access token without sending blanks for retained settings', async () => {
    await expect(
      saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, {
        ...VALUES,
        phoneNumberId: '',
        businessAccountId: '',
        webhookVerifyToken: '',
      }),
    ).resolves.toEqual({ ok: true });
    expect(apiRequest).toHaveBeenCalledWith({
      path: `/v1/channels/${CHANNEL_ID}/whatsapp`,
      method: 'PATCH',
      branchId: BRANCH_ID,
      body: { identifier: VALUES.identifier, is_active: true, access_token: VALUES.accessToken },
    });
  });

  it('refuses switching the number without its Meta identifiers and token', async () => {
    await expect(
      saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, {
        ...VALUES,
        identifier: '+541143210000',
        phoneNumberId: '',
        businessAccountId: '',
        accessToken: '',
      }),
    ).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('rejects malformed resource IDs before constructing the API path', async () => {
    await expect(saveWhatsAppChannel(BRANCH_ID, '../channels', VALUES)).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(getBranchChannels).not.toHaveBeenCalled();
  });

  it('returns only a stable error code after an API rejection', async () => {
    vi.mocked(apiRequest).mockRejectedValue(
      new ApiError('CHANNEL_IDENTIFIER', 422, 'provider detail'),
    );
    await expect(saveWhatsAppChannel(BRANCH_ID, CHANNEL_ID, VALUES)).resolves.toEqual({
      error: 'CHANNEL_IDENTIFIER',
    });
  });
});
