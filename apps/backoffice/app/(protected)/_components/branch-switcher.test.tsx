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
    defaultExpiryDays: 7,
    isActive: true,
  },
  {
    id: '22222222-2222-4222-8222-222222222222',
    name: 'Norte',
    address: null,
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
  it('shows the only reachable branch as the selected context without requiring a choice', () => {
    const view = renderSwitcher([BRANCHES[0]!], BRANCHES[0]!.id);
    const switcher = view.getByRole('combobox', { name: messages.common.branch.label });

    expect(switcher.textContent).toContain(BRANCHES[0]!.name);
    expect((switcher as HTMLButtonElement).disabled).toBe(true);
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
