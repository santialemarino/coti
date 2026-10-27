'use client';

import { ErrorPanel } from '@/components/error-card';

// Inside the group's own frame, so a failed screen keeps the navigation around it.
export default function GroupError({ reset }: { error: Error; reset: () => void }) {
  return <ErrorPanel reset={reset} />;
}
