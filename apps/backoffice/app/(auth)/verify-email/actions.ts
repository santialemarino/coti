'use server';

import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface ConfirmEmailResult {
  done?: boolean;
  error?: ApiErrorCode;
}

export async function confirmEmail(
  _previous: ConfirmEmailResult,
  formData: FormData,
): Promise<ConfirmEmailResult> {
  const token = String(formData.get('token') ?? '');
  if (!token) return { error: 'INVALID_LINK' };

  try {
    await apiRequest({
      path: '/v1/public/auth/verify-email',
      method: 'POST',
      authenticated: false,
      body: { token },
    });
    return { done: true };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export interface ResendVerificationResult {
  sent?: boolean;
  error?: ApiErrorCode;
  /* The address is the only thing this form can be wrong about. */
  field?: 'email';
}

/* Which field a refusal belongs on. A code absent from the map belongs to the form. */
const RESEND_FIELD_FOR: Partial<Record<ApiErrorCode, 'email'>> = { INVALID_BODY: 'email' };

// Answers the same whether or not the address is registered or already confirmed.
export async function resendVerification(email: string): Promise<ResendVerificationResult> {
  if (!email) return { error: 'INVALID_BODY', field: 'email' };

  try {
    await apiRequest({
      path: '/v1/public/auth/resend-verification',
      method: 'POST',
      authenticated: false,
      body: { email },
    });
    return { sent: true };
  } catch (error) {
    const code = errorCodeOf(error);
    return { error: code, field: RESEND_FIELD_FOR[code] };
  }
}
