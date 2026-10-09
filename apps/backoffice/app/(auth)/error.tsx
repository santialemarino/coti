'use client';

import { ErrorCard } from '@/components/error-card';

// The auth layout already draws the branded frame, so only the card goes inside it.
export default function AuthError({ reset }: { error: Error; reset: () => void }) {
  return <ErrorCard reset={reset} />;
}
