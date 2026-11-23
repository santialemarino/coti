import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { BranchEmailSettings } from '@/app/(protected)/settings/branches/[branchId]/channels/_components/branch-email-settings';
import { ApiError } from '@/lib/api/errors';
import { startGmailAuthorization } from '@/lib/api/gmail';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/branches/actions', () => ({ updateBranch: vi.fn() }));
vi.mock('@/lib/api/gmail', () => ({ startGmailAuthorization: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const copy = messages.branchChannels;

beforeEach(() => vi.resetAllMocks());

function renderSettings(email: string | null) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <BranchEmailSettings
        branch={{
          id: 'branch-1',
          name: 'Moron',
          email,
          address: null,
          defaultExpiryDays: 7,
          isActive: true,
        }}
      />
    </NextIntlClientProvider>,
  );
}

describe('BranchEmailSettings', () => {
  it('explains that a test address needs replacing before Gmail authorization', () => {
    renderSettings('admin@corralon.test');
    expect(screen.getByText(copy.testMailbox)).toBeTruthy();
    expect(screen.queryByRole('button', { name: copy.connectGmail })).toBeNull();
    expect(screen.getByRole('button', { name: copy.editEmail })).toBeTruthy();
  });

  it('requires an email address before offering Gmail authorization', () => {
    renderSettings(null);
    expect(screen.getByText(copy.emailRequired)).toBeTruthy();
    expect(screen.queryByRole('button', { name: copy.connectGmail })).toBeNull();
  });

  it('explains missing Coti integration setup after an authorization attempt', async () => {
    vi.mocked(startGmailAuthorization).mockRejectedValue(new ApiError('NOT_CONFIGURED', 503));
    renderSettings('pedidos@gmail.com');
    expect(screen.getByText(copy.emailRegistered)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: copy.connectGmail }));
    expect(await screen.findByText(messages.branches.gmail.errors.NOT_CONFIGURED)).toBeTruthy();
    expect(startGmailAuthorization).toHaveBeenCalledWith('branch-1', 'pedidos@gmail.com');
  });
});
