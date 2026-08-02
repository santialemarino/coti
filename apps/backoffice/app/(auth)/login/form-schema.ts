import { z } from 'zod';

import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

export const loginSchema = z.object({
  email: z.email(),
  password: z.string().min(PASSWORD_MIN_LENGTH),
});
