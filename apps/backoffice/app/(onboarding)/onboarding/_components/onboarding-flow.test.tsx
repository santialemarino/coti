import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { OnboardingFlow } from '@/app/(onboarding)/onboarding/_components/onboarding-flow';
import type { Onboarding } from '@/lib/api/onboarding';
import type { AccountUser } from '@/lib/api/users';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: vi.fn(), replace: vi.fn(), push: vi.fn() }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/components/catalog-import', () => ({
  CatalogReview: () => null,
  CatalogUpload: () => null,
}));
vi.mock('@/app/(onboarding)/onboarding/actions', () => ({
  completeOnboarding: vi.fn(async () => ({ ok: true })),
  createOnboardingUser: vi.fn(),
  dismissOnboarding: vi.fn(),
  resumeOnboarding: vi.fn(),
  saveOnboardingStep: vi.fn(async () => ({ ok: true })),
  updateOnboardingBranch: vi.fn(),
  updateOnboardingBrand: vi.fn(),
}));

const { saveOnboardingStep } = await import('@/app/(onboarding)/onboarding/actions');

const copy = messages.onboarding;
const ADMIN_ID = 'u-admin';

const ADMIN: AccountUser = {
  id: ADMIN_ID,
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: 'ADMIN',
  isActive: true,
} as AccountUser;

const SELLER: AccountUser = {
  id: 'u-seller',
  name: 'Bruno Díaz',
  email: 'bruno@corralon.test',
  role: 'SELLER',
  isActive: true,
} as AccountUser;

function onboarding(overrides: Partial<Onboarding>): Onboarding {
  return {
    flowVersion: 1,
    status: 'IN_PROGRESS',
    currentStep: 'TEAM',
    steps: {},
    completedAt: null,
    ...overrides,
  };
}

function renderFlow(state: Onboarding, users: AccountUser[] = [ADMIN], catalog = messages) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={catalog}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <OnboardingFlow
        onboarding={state}
        account={{} as never}
        branch={{ id: 'b1', name: 'Centro' } as never}
        branches={[]}
        users={users}
        currentUserId={ADMIN_ID}
      />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('OnboardingFlow completion', () => {
  it('reports what was set up and what is still waiting, not a blanket success', () => {
    const view = renderFlow(
      onboarding({
        status: 'COMPLETED',
        currentStep: 'COMPLETE',
        steps: {
          BRAND: 'COMPLETED',
          FIRST_BRANCH: 'COMPLETED',
          CATALOG_UPLOAD: 'SKIPPED',
          TEAM: 'SKIPPED',
        },
      }),
    );

    expect(view.getByText(copy.complete.partialTitle)).toBeTruthy();
    expect(view.queryByText(copy.complete.readyTitle)).toBeNull();
    expect(view.getByText(copy.complete.items.brand)).toBeTruthy();
    expect(view.getByText(copy.complete.pending.catalog)).toBeTruthy();
    expect(view.getByText(copy.complete.pending.team)).toBeTruthy();
    expect(view.queryByText(copy.complete.items.catalog)).toBeNull();
  });

  it('calls the setup complete only when every step was', () => {
    const view = renderFlow(
      onboarding({
        status: 'COMPLETED',
        currentStep: 'COMPLETE',
        steps: {
          BRAND: 'COMPLETED',
          FIRST_BRANCH: 'COMPLETED',
          CATALOG_UPLOAD: 'COMPLETED',
          TEAM: 'COMPLETED',
        },
      }),
    );

    expect(view.getByText(copy.complete.readyTitle)).toBeTruthy();
  });
});

describe('OnboardingFlow team step', () => {
  // The catalog's "Coti" would read the same as a hardcoded one, so the name is changed here.
  it('names the product from the catalog rather than a hardcoded word', () => {
    const renamed = { ...messages, common: { ...messages.common, appName: 'Cotizador' } };
    const view = renderFlow(onboarding({}), [ADMIN], renamed);

    expect(view.getByRole('link', { name: 'Cotizador' })).toBeTruthy();
  });

  it('lists the admin among the users, as themselves', () => {
    const view = renderFlow(onboarding({}));

    expect(view.getByText(ADMIN.name)).toBeTruthy();
    expect(view.getByText(copy.team.you)).toBeTruthy();
  });

  it('resolves the step as skipped when nobody was added', async () => {
    const view = renderFlow(onboarding({}));
    fireEvent.click(view.getByRole('button', { name: copy.team.finish }));

    await waitFor(() =>
      expect(saveOnboardingStep).toHaveBeenCalledWith('TEAM', 'SKIPPED', 'COMPLETE'),
    );
  });

  it('resolves the step as done once someone else is on the account', async () => {
    const view = renderFlow(onboarding({}), [ADMIN, SELLER]);
    fireEvent.click(view.getByRole('button', { name: copy.team.finish }));

    await waitFor(() =>
      expect(saveOnboardingStep).toHaveBeenCalledWith('TEAM', 'COMPLETED', 'COMPLETE'),
    );
  });
});
