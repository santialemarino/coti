import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { errorCodeOf } from '@/lib/api/errors';

type SignupInviteRefusal = 'SIGNUP_INVITE_INVALID' | 'SIGNUP_INVITE_USED' | 'SIGNUP_INVITE_EXPIRED';

// Why the API would refuse this invite, or null while it can still register a corralón.
export async function signupInviteRefusal(token: string): Promise<SignupInviteRefusal | null> {
  try {
    await apiRequest<void>({
      path: '/v1/public/signup-invites/check',
      method: 'POST',
      authenticated: false,
      branchScoped: false,
      body: { token },
    });
    return null;
  } catch (error) {
    const code = errorCodeOf(error);
    switch (code) {
      case 'SIGNUP_INVITE_INVALID':
      case 'SIGNUP_INVITE_USED':
      case 'SIGNUP_INVITE_EXPIRED':
        return code;
      default:
        throw error;
    }
  }
}
