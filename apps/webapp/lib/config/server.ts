import 'server-only';

import { API_URL } from '@/lib/config';

export const API_INTERNAL_URL = process.env.API_INTERNAL_URL ?? API_URL;
