import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { OnboardingFlow } from '@/app/(onboarding)/onboarding/_components/onboarding-flow';
import type { UserValues } from '@/app/(protected)/settings/users/form-schema';
import type { AccountUser } from '@/lib/api/users';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: vi.fn(), replace: vi.fn(), push: vi.fn() }),
}));
vi.mock('@/components/catalog-import', () => ({
  CatalogReview: () => null,
  CatalogUpload: () => null,
}));
// Stands in for the real step: one click creates a user, and the `users` prop does not change,
// which is where the page is while the refresh that would bring the user in is still in flight.
vi.mock('@/app/(onboarding)/onboarding/_components/team-step', () => ({
  TeamStep: ({ onCreate }: { onCreate: (values: UserValues) => Promise<unknown> }) => (
    <button type="button" onClick={() => void onCreate({} as UserValues)}>
      crear
    </button>
  ),
}));
vi.mock('@/app/(onboarding)/onboarding/actions', () => ({
  completeOnboarding: vi.fn(async () => ({ ok: true })),
  createOnboardingUser: vi.fn(async () => ({ ok: true })),
  dismissOnboarding: vi.fn(),
  resumeOnboarding: vi.fn(),
  saveOnboardingStep: vi.fn(async () => ({ ok: true })),
  updateOnboardingBranch: vi.fn(),
  updateOnboardingBrand: vi.fn(),
}));

const { createOnboardingUser, saveOnboardingStep } =
  await import('@/app/(onboarding)/onboarding/actions');

const ADMIN = { id: 'u-admin', name: 'Ana', email: 'a@c.test', role: 'ADMIN', isActive: true };

describe('OnboardingFlow team step', () => {
  it('counts a user created in the step before the page has refreshed', async () => {
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <OnboardingFlow
          onboarding={{
            flowVersion: 1,
            status: 'IN_PROGRESS',
            currentStep: 'TEAM',
            steps: {},
            completedAt: null,
          }}
          account={{} as never}
          branch={{ id: 'b1' } as never}
          branches={[]}
          users={[ADMIN as AccountUser]}
          currentUserId="u-admin"
        />
      </NextIntlClientProvider>,
    );

    fireEvent.click(view.getByRole('button', { name: 'crear' }));
    await waitFor(() => expect(createOnboardingUser).toHaveBeenCalled());
    fireEvent.click(view.getByRole('button', { name: messages.onboarding.team.finish }));

    await waitFor(() =>
      expect(saveOnboardingStep).toHaveBeenCalledWith('TEAM', 'COMPLETED', 'COMPLETE'),
    );
  });
});
