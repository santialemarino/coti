import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { API_URL } from '@/lib/config';

interface AccountLogoUploadRaw {
  path: string;
}

export async function uploadAccountLogo(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  const uploaded = await apiRequest<AccountLogoUploadRaw>({
    path: '/v1/account/logo',
    method: 'POST',
    formData,
    branchScoped: false,
  });
  return new URL(uploaded.path, API_URL).toString();
}
