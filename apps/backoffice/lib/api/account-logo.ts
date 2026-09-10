import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { ApiError } from '@/lib/api/errors';
import { API_URL } from '@/lib/config';

interface AccountLogoUploadRaw {
  path: string;
}

export async function uploadAccountLogo(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  try {
    const uploaded = await apiRequest<AccountLogoUploadRaw>({
      path: '/v1/account/logo',
      method: 'POST',
      formData,
      branchScoped: false,
    });
    return new URL(uploaded.path, API_URL).toString();
  } catch (error) {
    // This route resolves no entity, so 404 means the API deployment does not expose it.
    if (error instanceof ApiError && error.code === 'NOT_FOUND') {
      throw new ApiError('INTERNAL', 500, 'account logo upload route is unavailable');
    }
    throw error;
  }
}
