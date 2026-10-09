import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import ARCASettingsPage from '@/app/(protected)/settings/arca/page';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getBranches } from '@/lib/api/branches';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';
import messages from '@/translations/es.json';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('@/lib/api/arca-setup', () => ({ getARCASetup: vi.fn() }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({
  requireAdmin: vi.fn(),
  getSession: vi.fn(async () => ({ role: 'ADMIN' })),
  mustVerifyEmail: vi.fn(() => false),
}));
vi.mock('@/lib/utils/page', () => ({ generatePageMetadata: vi.fn() }));
vi.mock('@/app/(protected)/_components/page-header', () => ({ PageHeader: () => null }));
vi.mock('@/app/(protected)/settings/arca/actions', () => ({
  createARCASetup: vi.fn(),
  uploadARCACertificate: vi.fn(),
  verifyARCA: vi.fn(),
  disconnectARCA: vi.fn(),
}));

const { cookies } = await import('next/headers');
const SOLE = '11111111-1111-4111-8111-111111111111';
const SECOND = '22222222-2222-4222-8222-222222222222';

function reachable(...ids: string[]) {
  vi.mocked(getBranches).mockResolvedValue(
    ids.map((id) => ({
      id,
      name: id,
      address: null,
      email: null,
      defaultExpiryDays: 7,
      isActive: true,
    })),
  );
}

async function show(selected?: string) {
  const store = cookieJar(selected ? { [BRANCH_COOKIE]: selected } : {});
  vi.mocked(cookies).mockResolvedValue(store as unknown as Awaited<ReturnType<typeof cookies>>);
  return render(
    <NextIntlClientProvider
      locale="es-AR"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      {await ARCASettingsPage()}
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getARCASetup).mockResolvedValue({
    enabled: true,
    taxId: '20329642330',
    csr: 'public request',
    hasCertificate: true,
    certificateExpiresAt: '2028-01-01',
    pointOfSale: 7,
    verifiedAt: null,
  });
});

describe('ARCA branch selection', () => {
  it('shows the point of sale for a single-branch admin without a selection cookie', async () => {
    reachable(SOLE);
    const view = await show();
    expect(
      view.getByRole('textbox', { name: new RegExp(messages.arca.pointOfSale.label) }),
    ).toHaveProperty('value', '7');
    expect(view.getByRole('button', { name: messages.arca.verify })).toBeTruthy();
    expect(view.queryByText(messages.arca.noBranch)).toBeNull();
    expect(getARCASetup).toHaveBeenCalledWith(SOLE);
  });

  it('requires a choice when an admin has several branches', async () => {
    reachable(SOLE, SECOND);
    const view = await show();
    expect(view.getByText(messages.arca.noBranch)).toBeTruthy();
    expect(view.queryByRole('button', { name: messages.arca.verify })).toBeNull();
    expect(getARCASetup).toHaveBeenCalledWith(undefined);
  });

  it('loads the point of sale of the explicitly selected branch', async () => {
    reachable(SOLE, SECOND);
    const view = await show(SECOND);
    expect(view.getByRole('button', { name: messages.arca.verify })).toBeTruthy();
    expect(getARCASetup).toHaveBeenCalledWith(SECOND);
  });
});
