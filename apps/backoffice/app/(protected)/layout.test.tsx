import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/app/(protected)/_components/app-header', () => ({ AppHeader: vi.fn(() => null) }));
vi.mock('@/lib/api/onboarding', () => ({ getOnboarding: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));
/*
 * Thrown rather than recorded, the way the real one behaves: a no-op mock would let the layout run
 * on past the redirect, and the assertion that matters here is what it never reaches.
 */
vi.mock('next/navigation', () => ({
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
const { getSession } = await import('@/lib/auth/session');
const { ROUTES } = await import('@/config/routes');
const { default: ProtectedLayout } = await import('@/app/(protected)/layout');

function session(emailVerified: boolean, role = 'ADMIN') {
  return {
    userId: 'u1',
    accountId: 'a1',
    name: 'Ana Gómez',
    email: 'ana@corralonsanmartin.test',
    emailVerified,
    role,
  };
}

// Returns where the layout sent the caller, or null when it rendered instead.
async function redirectedTo(): Promise<string | null> {
  try {
    render(await ProtectedLayout({ children: null }));
    return null;
  } catch (error) {
    if (error instanceof RedirectError) return error.path;
    throw error;
  }
}

const FINISHED_ONBOARDING = {
  flowVersion: 1,
  status: 'COMPLETED',
  currentStep: 'COMPLETE',
  steps: {},
  completedAt: '2026-08-01T00:00:00Z',
} as const;

beforeEach(() => {
  vi.clearAllMocks();
  // Answered by default so a layout that reads it out of turn fails on the assertion rather than
  // on an undefined, which reads as a crash instead of as the ordering it is about.
  vi.mocked(getOnboarding).mockResolvedValue(FINISHED_ONBOARDING);
});

describe('ProtectedLayout', () => {
  it('sends a caller with no session to the route that clears the cookies', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await expect(redirectedTo()).resolves.toBe(ROUTES.sessionEnded);
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
      completedAt: null,
    });

    await expect(redirectedTo()).resolves.toBe(ROUTES.onboarding);
  });

  it('renders the shell for a confirmed seller, who has no onboarding to read', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true, 'SELLER'));

    await expect(redirectedTo()).resolves.toBeNull();
    expect(getOnboarding).not.toHaveBeenCalled();
  });
});
