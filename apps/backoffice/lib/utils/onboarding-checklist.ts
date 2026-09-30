import type { Onboarding } from '@/lib/api/onboarding';

// The checklist has something to say once the wizard is closed and a step is still pending.
export function hasPendingChecklist(onboarding: Onboarding): boolean {
  return onboarding.status !== 'IN_PROGRESS' && onboarding.checklist.some((item) => !item.done);
}

// "No mostrar más" hides only the home card; the settings entry stays until nothing is pending.
export function showsChecklistOnHome(onboarding: Onboarding): boolean {
  return hasPendingChecklist(onboarding) && onboarding.checklistHiddenAt === null;
}
