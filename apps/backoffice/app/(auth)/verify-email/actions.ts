'use server';

import { apiRequest, errorCodeOf } from '@/lib/api/client';

export interface ConfirmEmailResult {
  done?: boolean;
  error?: 'invalidLink' | 'unexpected';
}

export async function confirmEmail(
  _previous: ConfirmEmailResult,
  formData: FormData,
): Promise<ConfirmEmailResult> {
  const token = String(formData.get('token') ?? '');
  if (!token) return { error: 'invalidLink' };

  try {
    await apiRequest({
      path: '/v1/public/auth/verify-email',
      method: 'POST',
      authenticated: false,
      body: { token },
    });
    return { done: true };
  } catch (error) {
    // The API answers 401 for a link that is unknown, expired or already used, and the
    // screen keeps them together for the same reason it does.
    if (errorCodeOf(error) === 'unauthenticated') return { error: 'invalidLink' };
    return { error: 'unexpected' };
  }
}

export interface ResendVerificationResult {
  sent?: boolean;
  error?: 'invalidEmail' | 'unexpected';
}

// Answers the same whether or not the address is registered or already confirmed.
export async function resendVerification(email: string): Promise<ResendVerificationResult> {
  if (!email) return { error: 'invalidEmail' };

  try {
    await apiRequest({
      path: '/v1/public/auth/resend-verification',
      method: 'POST',
      authenticated: false,
      body: { email },
    });
    return { sent: true };
  } catch (error) {
    return { error: errorCodeOf(error) === 'badRequest' ? 'invalidEmail' : 'unexpected' };
  }
}
