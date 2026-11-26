'use server';

import { revalidatePath } from 'next/cache';
import { z } from 'zod';

import {
  whatsappChannelSchema,
  type WhatsAppChannelValues,
} from '@/app/(protected)/settings/branches/[branchId]/channels/form-schema';
import { ROUTES } from '@/config/routes';
import { getBranchChannels } from '@/lib/api/branch-channels';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import { requireAdmin } from '@/lib/auth/session';

interface ChannelResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function saveWhatsAppChannel(
  branchId: string,
  channelId: string,
  values: WhatsAppChannelValues,
): Promise<ChannelResult> {
  await requireAdmin();
  const parsed = whatsappChannelSchema(undefined, true).safeParse(values);
  if (
    !z.uuid().safeParse(branchId).success ||
    !z.uuid().safeParse(channelId).success ||
    !parsed.success
  ) {
    return { error: 'INVALID_BODY' };
  }

  try {
    const channels = await getBranchChannels(branchId);
    const channel = channels.find(
      (item) => item.id === channelId && item.branchId === branchId && item.type === 'WHATSAPP',
    );
    if (!channel) return { error: 'NOT_FOUND' };
    const checked = whatsappChannelSchema(
      undefined,
      channel.isConfigured,
      channel.identifier,
    ).safeParse(parsed.data);
    if (!checked.success) return { error: 'INVALID_BODY' };
    const data = checked.data;
    await apiRequest({
      path: `/v1/channels/${channelId}/whatsapp`,
      method: 'PATCH',
      branchId,
      body: {
        identifier: data.identifier,
        is_active: data.isActive,
        ...(data.phoneNumberId && { phone_number_id: data.phoneNumberId }),
        ...(data.businessAccountId && { business_account_id: data.businessAccountId }),
        ...(data.accessToken && { access_token: data.accessToken }),
        ...(data.webhookVerifyToken && { webhook_verify_token: data.webhookVerifyToken }),
      },
    });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.branchChannels(branchId));
  revalidatePath(ROUTES.branchSettings);
  return { ok: true };
}

const signupResultSchema = z.object({
  code: z.string().trim().min(1).max(4096),
  businessAccountId: z.string().regex(/^\d{1,32}$/),
  phoneNumberId: z.string().regex(/^\d{1,32}$/),
});

export async function connectWhatsAppSignup(
  branchId: string,
  channelId: string,
  result: { code: string; businessAccountId: string; phoneNumberId: string },
): Promise<ChannelResult> {
  await requireAdmin();
  const parsed = signupResultSchema.safeParse(result);
  if (
    !z.uuid().safeParse(branchId).success ||
    !z.uuid().safeParse(channelId).success ||
    !parsed.success
  ) {
    return { error: 'INVALID_BODY' };
  }
  try {
    await apiRequest({
      path: `/v1/channels/${channelId}/whatsapp/signup`,
      method: 'POST',
      branchId,
      body: {
        code: parsed.data.code,
        business_account_id: parsed.data.businessAccountId,
        phone_number_id: parsed.data.phoneNumberId,
      },
    });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.branchChannels(branchId));
  revalidatePath(ROUTES.branchSettings);
  return { ok: true };
}
