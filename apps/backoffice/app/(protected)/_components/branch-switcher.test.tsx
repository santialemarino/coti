import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { BranchSwitcher } from '@/app/(protected)/_components/branch-switcher';
import type { Branch } from '@/lib/api/branches';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/actions', () => ({ selectBranch: vi.fn() }));

const { selectBranch } = await import('@/app/(protected)/actions');

const BRANCHES: Branch[] = [
  {
    id: '11111111-1111-4111-8111-111111111111',
    name: 'Centro',
    address: null,
    email: null,
    defaultExpiryDays: 7,
    isActive: true,
  },
  {
    id: '22222222-2222-4222-8222-222222222222',
    name: 'Norte',
    address: null,
    email: null,
    defaultExpiryDays: 7,
    isActive: true,
  },
];

function renderSwitcher(branches: Branch[], activeBranchId: string | null = null, isAdmin = false) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <BranchSwitcher branches={branches} activeBranchId={activeBranchId} isAdmin={isAdmin} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('BranchSwitcher', () => {
  // A disabled menu is a control that explains nothing; one branch is shown as the context it is.
  it.each([
    ['a seller', false],
    ['an admin, whose "todas" reaches the same branch', true],
  ])('shows the only branch of %s as context, not as a menu', (_, isAdmin) => {
    const view = renderSwitcher([BRANCHES[0]!], BRANCHES[0]!.id, isAdmin);

    expect(view.queryByRole('combobox', { name: messages.common.branch.label })).toBeNull();
    expect(view.getByText(BRANCHES[0]!.name).parentElement?.textContent).toBe(
      `${messages.common.branch.label}: ${BRANCHES[0]!.name}`,
    );
  });

  it('lets the caller choose when several branches are reachable', async () => {
    const view = renderSwitcher(BRANCHES);
    const switcher = view.getByRole('combobox', { name: messages.common.branch.label });

    expect((switcher as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(switcher);
    fireEvent.click(await view.findByRole('option', { name: BRANCHES[1]!.name }));

    await vi.waitFor(() => expect(selectBranch).toHaveBeenCalledWith(BRANCHES[1]!.id));
  });
});
