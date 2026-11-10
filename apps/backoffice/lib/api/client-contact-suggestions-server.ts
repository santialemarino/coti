import 'server-only';

import { cache } from 'react';
import { unstable_rethrow } from 'next/navigation';

import { apiRequest } from '@/lib/api/client';
import {
  mapContactSuggestion,
  type ContactSuggestion,
  type ContactSuggestionRaw,
} from '@/lib/api/client-contact-suggestions';

export const getContactSuggestions = cache(async (): Promise<ContactSuggestion[] | null> => {
  try {
    const items = await apiRequest<ContactSuggestionRaw[]>({
      path: '/v1/client-contact-suggestions',
    });
    return (items ?? []).map(mapContactSuggestion);
  } catch (error) {
    unstable_rethrow(error);
    return null;
  }
});
