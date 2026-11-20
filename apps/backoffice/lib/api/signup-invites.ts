import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

// Why the API would refuse this invite, or null while it can still register a corralón.
export async function signupInviteRefusal(token: string): Promise<ApiErrorCode | null> {
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
    return errorCodeOf(error);
  }
}
