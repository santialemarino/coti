import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { ROUTES } from '@/config/routes';
import type { Onboarding } from '@/lib/api/onboarding';
import messages from '@/translations/es.json';

const push = vi.fn();
const refresh = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ push, refresh }) }));
vi.mock('@/app/(onboarding)/onboarding/actions', () => ({
  resumeOnboarding: vi.fn(async () => ({ ok: true })),
}));
vi.mock('@/app/(protected)/settings/onboarding/actions', () => ({
  setChecklistHidden: vi.fn(async () => ({ ok: true })),
}));

const { resumeOnboarding } = await import('@/app/(onboarding)/onboarding/actions');
const { setChecklistHidden } = await import('@/app/(protected)/settings/onboarding/actions');

function onboarding(overrides: Partial<Onboarding> = {}): Onboarding {
  return {
    flowVersion: 1,
    status: 'DISMISSED',
    currentStep: 'CATALOG_UPLOAD',
    steps: { BRAND: 'COMPLETED', CATALOG_UPLOAD: 'SKIPPED' },
    checklist: [
      { step: 'BRAND', done: true },
      { step: 'CATALOG_UPLOAD', done: false },
      { step: 'TEAM', done: false },
    ],
    checklistHiddenAt: null,
    completedAt: null,
    ...overrides,
  };
}

function renderChecklist(state: Onboarding, placement: 'home' | 'settings' = 'home') {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <OnboardingChecklist onboarding={state} placement={placement} />
    </NextIntlClientProvider>,
  );
}

describe('OnboardingChecklist', () => {
  beforeEach(() => vi.clearAllMocks());

  it('counts what is done and sends each pending step to the screen that does it', () => {
    renderChecklist(onboarding());

    expect(screen.getByText('1 de 3')).toBeTruthy();
    const links = screen.getAllByRole('link', { name: 'Configurar' });
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      ROUTES.catalogSettings,
      ROUTES.userSettings,
    ]);
    expect(screen.getByText('Listo')).toBeTruthy();
  });

  it('hides the home card only after the administrator confirms', async () => {
    renderChecklist(onboarding());

    fireEvent.click(screen.getByRole('button', { name: 'No mostrar más' }));
    expect(setChecklistHidden).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole('button', { name: 'Ocultar' }));

    await waitFor(() => expect(setChecklistHidden).toHaveBeenCalledWith(true));
    expect(refresh).toHaveBeenCalled();
  });

  it('brings a hidden card back from the settings page', async () => {
    renderChecklist(onboarding({ checklistHiddenAt: '2026-09-30T12:00:00Z' }), 'settings');

    expect(screen.queryByRole('button', { name: 'No mostrar más' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Volver a mostrar en el inicio' }));

    await waitFor(() => expect(setChecklistHidden).toHaveBeenCalledWith(false));
  });

  it('offers nothing to bring back while the card is still on the home screen', () => {
    renderChecklist(onboarding(), 'settings');

    expect(screen.queryByRole('button', { name: 'Volver a mostrar en el inicio' })).toBeNull();
  });

  it('resumes a dismissed wizard where it was left', async () => {
    renderChecklist(onboarding());

    fireEvent.click(screen.getByRole('button', { name: 'Retomar asistente' }));

    await waitFor(() => expect(push).toHaveBeenCalledWith(ROUTES.onboarding));
    expect(resumeOnboarding).toHaveBeenCalled();
  });

  it('does not offer the wizard again once it was finished', () => {
    renderChecklist(onboarding({ status: 'COMPLETED', currentStep: 'COMPLETE' }));

    expect(screen.queryByRole('button', { name: 'Retomar asistente' })).toBeNull();
  });
});
