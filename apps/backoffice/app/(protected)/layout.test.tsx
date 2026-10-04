import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Onboarding } from '@/lib/api/onboarding';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/_components/app-header', () => ({ AppHeader: vi.fn(() => null) }));
vi.mock('@/app/(protected)/rfqs/_components/rfq-queue-provider', () => ({
  RfqQueueProvider: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock('@/app/(protected)/rfqs/_components/rfq-queue-column', () => ({
  RfqQueueColumn: () => null,
}));
vi.mock('@/lib/api/onboarding', () => ({ getOnboarding: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getSelectedBranchId: vi.fn() }));
vi.mock('@/lib/auth/session', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/auth/session')>()),
  getSession: vi.fn(),
}));
/*
 * Thrown rather than recorded, the way the real one behaves: a no-op mock would let the layout run
 * on past the redirect, and the assertion that matters here is what it never reaches.
 */
vi.mock('next/navigation', () => ({
  unstable_rethrow: vi.fn(),
  redirect: vi.fn((path: string) => {
    throw new RedirectError(path);
  }),
}));

class RedirectError extends Error {
  constructor(readonly path: string) {
    super(`NEXT_REDIRECT ${path}`);
  }
}

const { getOnboarding } = await import('@/lib/api/onboarding');
const { getBranches } = await import('@/lib/api/branches');
const { getSession } = await import('@/lib/auth/session');
const { getSelectedBranchId } = await import('@/lib/auth/branch');
const { ROUTES } = await import('@/config/routes');
const { default: ProtectedLayout } = await import('@/app/(protected)/layout');

function session(emailVerified: boolean, role = 'ADMIN', emailVerificationRequired = true) {
  return {
    userId: 'u1',
    accountId: 'a1',
    name: 'Ana Gómez',
    email: 'ana@corralonsanmartin.test',
    emailVerified,
    role,
    emailVerificationRequired,
    mailDelivery: true,
  };
}

// Returns where the layout sent the caller, or null when it rendered instead.
async function redirectedTo(): Promise<string | null> {
  try {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        {await ProtectedLayout({ children: <p>pantalla</p> })}
      </NextIntlClientProvider>,
    );
    return null;
  } catch (error) {
    if (error instanceof RedirectError) return error.path;
    throw error;
  }
}

const FINISHED_ONBOARDING: Onboarding = {
  flowVersion: 1,
  status: 'COMPLETED',
  currentStep: 'COMPLETE',
  steps: {},
  checklist: [],
  checklistHiddenAt: null,
  completedAt: '2026-08-01T00:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
  // Answered by default so a layout that reads it out of turn fails on the assertion rather than
  // on an undefined, which reads as a crash instead of as the ordering it is about.
  vi.mocked(getOnboarding).mockResolvedValue(FINISHED_ONBOARDING);
  vi.mocked(getBranches).mockResolvedValue([{ id: 'b1', isActive: true }] as never);
  vi.mocked(getSelectedBranchId).mockResolvedValue(undefined);
});

describe('ProtectedLayout', () => {
  it('sends a caller with no session to the route that clears the cookies', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await expect(redirectedTo()).resolves.toBe(ROUTES.sessionEnded);
  });

  // With the requirement off — always the case under console mail — the confirmation screen is a
  // suggestion, and holding the caller there would strand them behind a link that cannot arrive.
  it('lets an unconfirmed caller in while the installation does not require a confirmation', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false, 'ADMIN', false));

    await expect(redirectedTo()).resolves.toBeNull();
    expect(screen.getByText('pantalla')).toBeTruthy();
  });

  /*
   * Order is the whole point and getting it wrong is silent: the onboarding read is itself closed
   * until the address is confirmed and does not catch, so asking it first throws out of the layout
   * where the confirmation screen belonged. Registration makes that the common path, not an edge.
   */
  it('sends an unconfirmed admin to confirm without reading the onboarding state', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false));

    await expect(redirectedTo()).resolves.toBe(ROUTES.verifyEmail);
    expect(getOnboarding).not.toHaveBeenCalled();
  });

  it('sends an unconfirmed seller to confirm too', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false, 'SELLER'));

    await expect(redirectedTo()).resolves.toBe(ROUTES.verifyEmail);
  });

  it('resumes onboarding for a confirmed admin who has not finished it', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true));
    vi.mocked(getOnboarding).mockResolvedValue({
      flowVersion: 1,
      status: 'IN_PROGRESS',
      currentStep: 'WELCOME',
      steps: {},
      checklist: [],
      checklistHiddenAt: null,
      completedAt: null,
    });

    await expect(redirectedTo()).resolves.toBe(ROUTES.onboarding);
  });

  it('renders the shell for a confirmed seller, who has no onboarding to read', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true, 'SELLER'));

    await expect(redirectedTo()).resolves.toBeNull();
    expect(getOnboarding).not.toHaveBeenCalled();
  });

  // Onboarding itself sends an account without an active branch to Sucursales; bouncing it back
  // from there would loop between the two for good.
  it('lets an admin with no active branch reach the app while onboarding is open', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true));
    vi.mocked(getOnboarding).mockResolvedValue({
      flowVersion: 1,
      status: 'IN_PROGRESS',
      currentStep: 'WELCOME',
      steps: {},
      checklist: [],
      checklistHiddenAt: null,
      completedAt: null,
    });
    vi.mocked(getBranches).mockResolvedValue([{ id: 'b1', isActive: false }] as never);

    await expect(redirectedTo()).resolves.toBeNull();
  });

  // A closed or unassigned branch left in the cookie made every scoped read answer 403, and the
  // error screen read the same cookie again.
  it('drops a branch cookie the caller no longer reaches', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true, 'SELLER'));
    vi.mocked(getSelectedBranchId).mockResolvedValue('b-cerrada');

    await expect(redirectedTo()).resolves.toBe(ROUTES.branchReset);
  });

  it('keeps a branch cookie the caller still reaches', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true, 'SELLER'));
    vi.mocked(getSelectedBranchId).mockResolvedValue('b1');

    await expect(redirectedTo()).resolves.toBeNull();
  });
});
