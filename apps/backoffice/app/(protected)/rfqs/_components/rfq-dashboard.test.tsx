import { fireEvent, render, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import type { RfqRecord } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
// The create dialog pulls in the import/manual views; it is not what these tests exercise.
vi.mock('@/app/(protected)/rfqs/_components/create-rfq-dialog', () => ({
  CreateRfqDialog: () => null,
}));
// QUOTE_GENERATION_MS shortened so the generation step lands without fake timers.
vi.mock('@/lib/api/rfqs', () => ({
  QUOTE_GENERATION_MS: 50,
}));

const { toast } = await import('sonner');

const copy = messages.rfqs;

const RFQS: RfqRecord[] = [
  {
    id: '2001',
    client: 'Constructora A',
    createdAt: '2026-08-06T10:00:00.000Z',
    channel: 'whatsapp',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Centro',
    branchId: 'b1',
    itemCount: 2,
    priority: 'high',
    status: 'QUOTED',
    total: '100.00',
    needsFollowup: false,
  },
  {
    id: '2002',
    client: 'Ferretería B',
    createdAt: '2026-08-06T09:00:00.000Z',
    channel: 'email',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Norte',
    branchId: 'b2',
    itemCount: 3,
    priority: 'normal',
    status: 'SENT',
    total: '200.00',
    needsFollowup: false,
  },
  {
    id: '2003',
    client: 'Obra C',
    createdAt: '2026-08-05T08:00:00.000Z',
    channel: 'manual_entry',
    seller: 'Juan Pérez',
    sellerId: 's2',
    branch: 'Centro',
    branchId: 'b1',
    itemCount: 4,
    priority: 'normal',
    status: 'QUOTED',
    total: '300.00',
    needsFollowup: false,
  },
  {
    id: '2004',
    client: 'Techos D',
    createdAt: '2026-08-05T07:00:00.000Z',
    channel: 'manual_entry',
    seller: 'Juan Pérez',
    sellerId: 's2',
    branch: 'Norte',
    branchId: 'b2',
    itemCount: 5,
    priority: 'low',
    status: 'RECEIVED',
    needsFollowup: false,
  },
  {
    id: '2005',
    client: 'Pinturas E',
    createdAt: '2026-08-04T06:00:00.000Z',
    channel: 'webapp',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Centro',
    branchId: 'b1',
    itemCount: 6,
    priority: 'high',
    status: 'GENERATED',
    needsFollowup: false,
  },
];

function renderDashboard(records: RfqRecord[] = RFQS) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqDashboard
        initialRecords={records}
        activeBranchId="b0000000-0000-4000-8000-000000000001"
      />
    </NextIntlClientProvider>,
  );
}

// The tabs name themselves after the status and append the count to the same text node, so the
// count is the item's text minus the label. Scoped to the toggle group because the rows repeat it.
function tabCount(view: ReturnType<typeof render>, label: string): number {
  const group = view.getByLabelText(copy.list.tabs);
  const item = within(group).getByText(label);
  const count = (item.textContent ?? '').replace(label, '');
  return Number(count);
}

function rowOf(view: ReturnType<typeof render>, id: string) {
  const row = view.getByRole('row', { name: new RegExp(id) });
  return within(row);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RfqDashboard status tab counts', () => {
  it('counts the whole list when no filter is active', () => {
    const view = renderDashboard();

    expect(tabCount(view, copy.status.QUOTED)).toBe(2);
    expect(tabCount(view, copy.status.SENT)).toBe(1);
    expect(tabCount(view, copy.status.RECEIVED)).toBe(1);
    expect(tabCount(view, copy.status.GENERATED)).toBe(1);
  });

  // The tabs are counters of "how many match what I'm looking at", so a seller filter narrows them
  // too instead of leaving stale global numbers next to the filtered rows.
  it('recounts within the active filters instead of staying global', async () => {
    const view = renderDashboard();

    fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.seller }));
    const maria = await vi.waitFor(() =>
      view.getAllByRole('option').find((option) => option.textContent === 'María López'),
    );
    if (!maria) throw new Error('María López option never appeared');
    fireEvent.click(maria);

    await vi.waitFor(() => expect(tabCount(view, copy.status.QUOTED)).toBe(1));
    expect(tabCount(view, copy.status.SENT)).toBe(1);
    expect(tabCount(view, copy.status.RECEIVED)).toBe(0);
    expect(tabCount(view, copy.status.GENERATED)).toBe(1);
    expect(view.queryByText('#2003')).toBeNull();
  });

  it('combines the status tab with the other filters', async () => {
    const view = renderDashboard();

    fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.seller }));
    const maria = await vi.waitFor(() =>
      view.getAllByRole('option').find((option) => option.textContent === 'María López'),
    );
    if (!maria) throw new Error('María López option never appeared');
    fireEvent.click(maria);
    await vi.waitFor(() => expect(tabCount(view, copy.status.QUOTED)).toBe(1));

    fireEvent.click(within(view.getByLabelText(copy.list.tabs)).getByText(copy.status.QUOTED));
    await vi.waitFor(() => expect(view.queryByText('#2002')).toBeNull());
    expect(view.getByText('#2001')).toBeTruthy();
  });
});

describe('RfqDashboard totals column', () => {
  it('shows a dash until the quote exists and the amount once it does', () => {
    const view = renderDashboard();

    expect(rowOf(view, '2004').getByText('-')).toBeTruthy();
    expect(rowOf(view, '2001').getByText('$ 100,00')).toBeTruthy();
    expect(rowOf(view, '2001').queryByText('-')).toBeNull();
  });
});

describe('RfqDashboard marking a pedido as QUOTED', () => {
  it('runs the generation spinner and then lands on the pill', async () => {
    const view = renderDashboard();

    fireEvent.pointerDown(
      rowOf(view, '2005').getByRole('button', { name: copy.list.actions.more }),
      {
        button: 0,
      },
    );
    const changeStatus = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.changeStatus }),
    );
    fireEvent.click(changeStatus);
    const quoted = await vi.waitFor(() => view.getByRole('menuitem', { name: copy.status.QUOTED }));
    fireEvent.click(quoted);

    await vi.waitFor(() =>
      expect(rowOf(view, '2005').getByText(copy.processing.quote)).toBeTruthy(),
    );

    await vi.waitFor(() => expect(rowOf(view, '2005').getByText(copy.status.QUOTED)).toBeTruthy(), {
      timeout: 3000,
    });
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.quoteGenerated.replace('{id}', '2005'),
    );
  });
});
