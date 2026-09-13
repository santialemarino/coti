import { fireEvent, render, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqDashboard } from '@/app/(protected)/rfqs/_components/rfq-dashboard';
import { RfqListProvider } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { ApiError } from '@/lib/api/errors';
import type { RfqRecord } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

const router = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));

vi.mock('next/navigation', () => ({ useRouter: () => router }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
// The create dialog pulls in the import/manual views; it is not what these tests exercise.
vi.mock('@/app/(protected)/rfqs/_components/create-rfq-dialog', () => ({
  CreateRfqDialog: () => null,
}));
vi.mock('@/lib/api/rfqs-client', () => ({
  assignRfqSeller: vi.fn(),
  setQuoteArchived: vi.fn(),
  setRfqSeller: vi.fn(),
}));
vi.mock('@/lib/api/sellers', () => ({
  listSellers: vi.fn().mockResolvedValue([]),
}));

const { toast } = await import('sonner');
const { assignRfqSeller, setQuoteArchived, setRfqSeller } = await import('@/lib/api/rfqs-client');
const { listSellers } = await import('@/lib/api/sellers');

const copy = { ...messages.rfqs, common: messages.common };
const TEST_REFERENCES: Record<string, string> = { '2006': '#06', '2007': '#07' };

const RFQS: RfqRecord[] = [
  {
    id: '2001',
    quoteNumber: 1,
    client: 'Constructora A',
    createdAt: '2026-08-06T10:00:00.000Z',
    channel: 'whatsapp',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Centro',
    branchId: 'b1',
    quoteId: 'quote-b1',
    itemCount: 2,
    status: 'QUOTED',
    total: '100.00',
    needsFollowup: false,
  },
  {
    id: '2002',
    quoteNumber: 2,
    client: 'Ferretería B',
    createdAt: '2026-08-06T09:00:00.000Z',
    channel: 'email',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Norte',
    branchId: 'b2',
    quoteId: 'quote-b2',
    itemCount: 3,
    status: 'SENT',
    total: '200.00',
    needsFollowup: false,
  },
  {
    id: '2003',
    quoteNumber: 3,
    client: 'Obra C',
    createdAt: '2026-08-05T08:00:00.000Z',
    channel: 'manual_entry',
    seller: 'Juan Pérez',
    sellerId: 's2',
    branch: 'Centro',
    branchId: 'b1',
    quoteId: 'quote-b1',
    itemCount: 4,
    status: 'QUOTED',
    total: '300.00',
    needsFollowup: false,
  },
  {
    id: '2004',
    quoteNumber: null,
    client: 'Techos D',
    createdAt: '2026-08-05T07:00:00.000Z',
    channel: 'manual_entry',
    seller: 'Juan Pérez',
    sellerId: 's2',
    branch: 'Norte',
    branchId: 'b2',
    quoteId: 'quote-b2',
    itemCount: 5,
    status: 'RECEIVED',
    needsFollowup: false,
  },
  {
    id: '2005',
    quoteNumber: 5,
    client: 'Pinturas E',
    createdAt: '2026-08-04T06:00:00.000Z',
    channel: 'webapp',
    seller: 'María López',
    sellerId: 's1',
    branch: 'Centro',
    branchId: 'b1',
    quoteId: 'quote-b1',
    itemCount: 6,
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

// Renders the dashboard with explicit roles; useful for assign flow tests.
function renderAs(
  roles: { userName: string; userId: string; isAdmin: boolean },
  records: RfqRecord[],
) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqListProvider records={records} activeBranchId={null} {...roles}>
        <RfqDashboard initialRecords={records} activeBranchId={null} />
      </RfqListProvider>
    </NextIntlClientProvider>,
  );
}

/* Opens the status picker and reads the count the option carries, e.g. "Cotizado (2)". */
async function statusCount(view: ReturnType<typeof render>, label: string): Promise<number> {
  fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.status }));
  const option = await vi.waitFor(() =>
    view.getAllByRole('option').find((item) => item.textContent?.startsWith(label)),
  );
  if (!option) throw new Error(`No status option for ${label}`);
  const count = /\((\d+)\)/.exec(option.textContent ?? '');
  fireEvent.keyDown(option, { key: 'Escape' });
  return Number(count?.[1]);
}

function rowOf(view: ReturnType<typeof render>, label: string) {
  const row = view.getByText(TEST_REFERENCES[label] ?? label).closest('tr');
  if (!row) throw new Error(`No row contains ${label}`);
  return within(row);
}

/* Narrows the list to one seller through the seller dropdown. */
async function pickSeller(view: ReturnType<typeof render>, name: string) {
  fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.seller }));
  const option = await vi.waitFor(() =>
    view.getAllByRole('option').find((item) => item.textContent === name),
  );
  if (!option) throw new Error(`${name} option never appeared`);
  fireEvent.click(option);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RfqDashboard status filter counts', () => {
  it('counts the whole list when no filter is active', async () => {
    const view = renderDashboard();

    expect(await statusCount(view, copy.status.QUOTED)).toBe(2);
    expect(await statusCount(view, copy.status.SENT)).toBe(1);
    expect(await statusCount(view, copy.status.RECEIVED)).toBe(1);
    expect(await statusCount(view, copy.status.GENERATED)).toBe(1);
  });

  // The counts answer "how many match what I'm looking at", so a seller filter narrows them too
  // instead of leaving stale global numbers next to the filtered rows.
  it('recounts within the active filters instead of staying global', async () => {
    const view = renderDashboard();

    await pickSeller(view, 'María López');

    await vi.waitFor(async () => expect(await statusCount(view, copy.status.QUOTED)).toBe(1));
    expect(await statusCount(view, copy.status.RECEIVED)).toBe(0);
    expect(view.queryByText('#03')).toBeNull();
  });

  it('combines the status filter with the other filters', async () => {
    const view = renderDashboard();

    await pickSeller(view, 'María López');
    await vi.waitFor(async () => expect(await statusCount(view, copy.status.QUOTED)).toBe(1));

    fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.status }));
    const quoted = await vi.waitFor(() =>
      view.getAllByRole('option').find((item) => item.textContent?.startsWith(copy.status.QUOTED)),
    );
    if (!quoted) throw new Error('Cotizado option never appeared');
    fireEvent.click(quoted);

    await vi.waitFor(() => expect(view.queryByText('#02')).toBeNull());
    expect(view.getByText('#01')).toBeTruthy();
  });
});

describe('RfqDashboard archived filter', () => {
  const ARCHIVED: RfqRecord = {
    ...(RFQS[0] as RfqRecord),
    id: 'arch-1',
    quoteNumber: 77,
    quoteId: 'quote-arch',
    client: 'Cliente archivado',
    archived: true,
  };

  /*
   * Archivado is a value in the status picker, so picking it on its own means the archive — not
   * the whole queue with the archive added to it.
   */
  it('hides archived orders until the filter asks for them, then shows only those', async () => {
    const view = renderAs({ userName: 'Ana', userId: 'me', isAdmin: true }, [...RFQS, ARCHIVED]);

    expect(view.queryByText('#77')).toBeNull();

    fireEvent.click(view.getByRole('combobox', { name: copy.list.filters.status }));
    const option = await vi.waitFor(() =>
      view
        .getAllByRole('option')
        .find((item) => item.textContent?.startsWith(copy.status.ARCHIVED)),
    );
    if (!option) throw new Error('Archivados option never appeared');
    fireEvent.click(option);

    await vi.waitFor(() => expect(view.getByText('#77')).toBeTruthy());
    expect(view.queryByText('#01')).toBeNull();
  });
});

describe('RfqDashboard bulk archive', () => {
  // Twelve rows, so the selection spans two pages at a page size of ten.
  const MANY: RfqRecord[] = Array.from({ length: 12 }, (_, index) => ({
    ...(RFQS[0] as RfqRecord),
    id: `bulk-${index}`,
    quoteNumber: 100 + index,
    quoteId: `quote-bulk-${index}`,
    client: `Cliente ${index}`,
  }));

  /*
   * A selection survives paging, so acting on the visible page only would archive part of it and
   * clear the rest without saying so.
   */
  it('archives every selected order, including the ones off the current page', async () => {
    vi.mocked(setQuoteArchived).mockResolvedValue(undefined);
    const view = renderAs({ userName: 'Ana', userId: 'me', isAdmin: true }, MANY);

    fireEvent.click(view.getByLabelText(copy.list.selectAll));
    fireEvent.click(view.getByRole('button', { name: copy.common.pagination.next }));
    fireEvent.click(view.getByLabelText(copy.list.selectAll));
    fireEvent.click(view.getByRole('button', { name: copy.list.bulk.archive }));

    await vi.waitFor(() => expect(setQuoteArchived).toHaveBeenCalledTimes(MANY.length));
  });
});

describe('RfqDashboard branch column', () => {
  /*
   * The header switcher already says which branch is in view, so repeating it on every row is a
   * column of the same word. It comes back the moment the switcher is on "todas las sucursales".
   */
  it('drops the branch column while one branch is selected and restores it for all', () => {
    expect(renderDashboard().queryByText('Centro')).toBeNull();

    const all = renderAs({ userName: 'Ana', userId: 'me', isAdmin: true }, RFQS);
    expect(all.getAllByText('Centro').length).toBeGreaterThan(0);
  });
});

describe('RfqDashboard totals column', () => {
  it('shows a dash until the quote exists and the amount once it does', () => {
    const view = renderDashboard();

    expect(rowOf(view, copy.list.numberPending).getByText('—')).toBeTruthy();
    expect(rowOf(view, '#01').getByText('$ 100,00')).toBeTruthy();
    expect(rowOf(view, '#01').queryByText('—')).toBeNull();
  });
});

describe('RfqDashboard row actions', () => {
  it('opens the detail from the whole row without hijacking its controls', () => {
    const view = renderDashboard();
    const row = view.getByRole('link', {
      name: copy.list.openRow.replace('{id}', '#01'),
    });

    fireEvent.click(within(row).getByText('Constructora A'));
    expect(router.push).toHaveBeenLastCalledWith('/rfqs/2001');

    router.push.mockClear();
    fireEvent.keyDown(row, { key: 'Enter' });
    expect(router.push).toHaveBeenLastCalledWith('/rfqs/2001');

    router.push.mockClear();
    fireEvent.click(within(row).getByRole('checkbox'));
    expect(router.push).not.toHaveBeenCalled();
  });

  // Archiving is inline: the row already opens the detail, so a menu here would hold one item.
  it('archives from the row and names the order by its sequence number', async () => {
    vi.mocked(setQuoteArchived).mockResolvedValue(undefined);
    const view = renderDashboard();

    fireEvent.click(rowOf(view, '#01').getByRole('button', { name: copy.list.actions.archive }));

    await vi.waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith(copy.list.toast.archived.replace('{id}', '#01')),
    );
    /*
     * Named branch, not the header switcher's: it sits on "todas las sucursales" by default and a
     * quote write that arrives without one is refused, which is how this shipped reporting success
     * for a 422.
     */
    expect(setQuoteArchived).toHaveBeenCalledWith('quote-b1', true, 'b1');
    // Archived rows leave the queue until the status filter asks for them back.
    expect(view.queryByText('#01')).toBeNull();
  });

  // The flag only flips once the backend confirms it, or the row comes back on the next refresh.
  it('keeps the row and warns when the archive write fails', async () => {
    vi.mocked(setQuoteArchived).mockRejectedValue(new ApiError('INTERNAL', 500));
    const view = renderDashboard();

    fireEvent.click(rowOf(view, '#01').getByRole('button', { name: copy.list.actions.archive }));

    await vi.waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(view.getByText('#01')).toBeTruthy();
  });
});

/*
 * Tests for the "Asignarme" (self-assign) flow. These exercise the new
 * seller can assign an unassigned order: opening the row menu and selecting
 * the "Asignarme" action calls the backend, stamping the seller onto the
 * quote and showing a success toast; conflicts and for-bidden are also handled.
 */
describe('RfqDashboard claiming an unassigned order', () => {
  const unassigned: RfqRecord = {
    id: '2006',
    quoteNumber: 6,
    client: 'Obra F',
    createdAt: '2026-08-03T05:00:00.000Z',
    channel: 'whatsapp',
    seller: '',
    sellerId: null,
    branch: 'Centro',
    branchId: 'b1',
    quoteId: 'quote-b1',
    itemCount: 1,
    status: 'RECEIVED',
    needsFollowup: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(assignRfqSeller).mockResolvedValue({
      id: 'q6',
      branch_id: 'b1',
      client_id: null,
      rfq_id: '2006',
      seller_id: 'me',
      current_version_id: null,
      current_status: 'GENERATED',
      expires_at: null,
      archived_at: null,
      needs_followup: false,
      followup_flagged_at: null,
      created_at: '2026-08-03T05:00:00.000Z',
      updated_at: '2026-08-03T05:00:00.000Z',
    });
  });

  // The seller cell is the one claim surface now that the row menu is gone.
  async function claimFromCell(view: ReturnType<typeof render>) {
    fireEvent.pointerDown(rowOf(view, '2006').getByRole('button', { name: copy.list.unassigned }), {
      button: 0,
    });
    const assign = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.assign }),
    );
    fireEvent.click(assign);
  }

  it('offers Asignarme to a seller on an unassigned row and stamps the owner on the claim', async () => {
    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [unassigned]);

    await claimFromCell(view);

    expect(assignRfqSeller).toHaveBeenCalledWith('2006');
    // The row only stamps the owner once the backend confirms the claim, so wait for it.
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Ana Robles')).toBeTruthy());
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.assigned.replace('{id}', '#06').replace('{name}', 'Ana Robles'),
    );
  });

  it('keeps the row and warns when a peer claimed the order first', async () => {
    vi.mocked(assignRfqSeller).mockRejectedValue(new ApiError('CONFLICT', 409));

    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [unassigned]);

    await claimFromCell(view);

    expect(assignRfqSeller).toHaveBeenCalledWith('2006');
    // The conflict surfaces as a toast once the backend answers.
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.CONFLICT));
    // The row still shows the original seller-less state.
    expect(view.queryByText('Ana Robles')).toBeNull();
    expect(rowOf(view, '2006').getByText(copy.list.unassigned)).toBeTruthy();
  });

  it('warns that only sellers can claim when the backend refuses', async () => {
    vi.mocked(assignRfqSeller).mockRejectedValue(new ApiError('FORBIDDEN', 403));

    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [unassigned]);

    await claimFromCell(view);

    expect(assignRfqSeller).toHaveBeenCalledWith('2006');
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.FORBIDDEN));
    expect(view.queryByText('Ana Robles')).toBeNull();
  });

  // A seller on someone else's order has nothing to claim, so the cell is plain text.
  it('offers no claim surface when the order already has a seller', () => {
    const assigned: RfqRecord = { ...unassigned, sellerId: 'other', seller: 'Otro Vendedor' };

    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [assigned]);

    expect(rowOf(view, '2006').queryByRole('button', { name: 'Otro Vendedor' })).toBeNull();
    expect(rowOf(view, '2006').getByText('Otro Vendedor')).toBeTruthy();
  });
});

const quoteResponse = (rfqId: string, branchId: string, sellerId: string | null) => ({
  id: 'q6',
  branch_id: branchId,
  client_id: null,
  rfq_id: rfqId,
  seller_id: sellerId,
  current_version_id: null,
  current_status: 'GENERATED',
  expires_at: null,
  archived_at: null,
  needs_followup: false,
  followup_flagged_at: null,
  created_at: '2026-08-03T05:00:00.000Z',
  updated_at: '2026-08-03T05:00:00.000Z',
});

/*
 * Sellers per branch. A Morón order must only ever be offered Morón sellers and a Villa Bosch
 * order only Villa Bosch sellers; the picklist is scoped to the order's own branch_id.
 */
const SELLERS_BY_BRANCH: Record<string, Array<{ id: string; name: string }>> = {
  'b-moron': [{ id: 's-moron', name: 'Vero Morón' }],
  'b-villa': [{ id: 's-villa', name: 'Diego Villa' }],
};

/*
 * Tests for the admin steering of an order's owner (change seller / clear / self).
 * The admin menu opens with a "Cambiar vendedor" submenu fed by the order's own branch — never
 * the operator's active branch — selecting a seller PUTs /api/rfqs/:id/seller and stamps the
 * row on success, and the backend refuses any cross-branch pick.
 */
describe('RfqDashboard admin steering the seller', () => {
  const moronOrder: RfqRecord = {
    id: '2006',
    quoteNumber: 6,
    client: 'Obra F',
    createdAt: '2026-08-03T05:00:00.000Z',
    channel: 'whatsapp',
    seller: '',
    sellerId: null,
    branch: 'Morón',
    branchId: 'b-moron',
    quoteId: 'quote-b-moron',
    itemCount: 1,
    status: 'RECEIVED',
    needsFollowup: false,
  };
  const villaOrder: RfqRecord = {
    ...moronOrder,
    id: '2007',
    quoteNumber: 7,
    branch: 'Villa Bosch',
    branchId: 'b-villa',
    quoteId: 'quote-b-villa',
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listSellers).mockImplementation(
      async (branchId) => SELLERS_BY_BRANCH[branchId ?? ''] ?? [],
    );
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 's-moron'));
  });

  // Opens the owner picklist from the seller cell, which is where it lives.
  async function openSellerMenu(
    view: ReturnType<typeof render>,
    rfqId: string,
    name: string = copy.list.unassigned,
  ) {
    fireEvent.pointerDown(rowOf(view, rfqId).getByRole('button', { name }), { button: 0 });
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.assign }));
    return view;
  }

  it('shows only Morón sellers for a Morón order, scoped to its branch', async () => {
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
    );

    const moron = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' }));
    expect(moron).toBeTruthy();
    // Villa Bosch sellers never appear, and the fetch is the order's branch, not the operator's.
    expect(view.queryByRole('menuitem', { name: 'Diego Villa' })).toBeNull();
    expect(listSellers).toHaveBeenCalledWith('b-moron');
  });

  it('shows only Villa Bosch sellers for a Villa Bosch order and assigns one', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2007', 'b-villa', 's-villa'));
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [villaOrder]),
      '2007',
    );

    const villa = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Diego Villa' }));
    expect(villa).toBeTruthy();
    expect(view.queryByRole('menuitem', { name: 'Vero Morón' })).toBeNull();
    expect(listSellers).toHaveBeenCalledWith('b-villa');

    fireEvent.click(villa);
    expect(setRfqSeller).toHaveBeenCalledWith('2007', 's-villa');
    await vi.waitFor(() => expect(rowOf(view, '2007').getByText('Diego Villa')).toBeTruthy());
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.sellerAssigned.replace('{id}', '#07').replace('{seller}', 'Diego Villa'),
    );
  });

  it('assigns to a Morón seller of the order branch and stamps the row when confirmed', async () => {
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
    );

    const moron = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' }));
    fireEvent.click(moron);

    expect(setRfqSeller).toHaveBeenCalledWith('2006', 's-moron');
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Vero Morón')).toBeTruthy());
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.sellerAssigned.replace('{id}', '#06').replace('{seller}', 'Vero Morón'),
    );
  });

  it('assigns to the admin themselves and stamps their own name', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 'me'));
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
    );

    fireEvent.click(view.getByRole('menuitem', { name: copy.list.actions.assign }));

    expect(setRfqSeller).toHaveBeenCalledWith('2006', 'me');
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Admin')).toBeTruthy());
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.sellerAssigned.replace('{id}', '#06').replace('{seller}', 'Admin'),
    );
  });

  it('clears the owner via the unassign item', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', null));
    const assigned: RfqRecord = { ...moronOrder, sellerId: 's-moron', seller: 'Vero Morón' };
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [assigned]),
      '2006',
      'Vero Morón',
    );

    fireEvent.click(view.getByRole('menuitem', { name: copy.list.actions.unassignSeller }));

    expect(setRfqSeller).toHaveBeenCalledWith('2006', null);
    await vi.waitFor(() =>
      expect(rowOf(view, '2006').getByText(copy.list.unassigned)).toBeTruthy(),
    );
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.sellerUnassigned.replace('{id}', '#06'),
    );
  });

  it('surfaces an invalid seller pick as a toast and keeps the row', async () => {
    vi.mocked(setRfqSeller).mockRejectedValue(new ApiError('INVALID_INPUT', 422));
    const view = await openSellerMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
    );

    const moron = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' }));
    fireEvent.click(moron);

    expect(setRfqSeller).toHaveBeenCalledWith('2006', 's-moron');
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.INVALID_INPUT));
    expect(rowOf(view, '2006').getByText(copy.list.unassigned)).toBeTruthy();
  });

  it('offers a seller only the claim, never the branch picklist', async () => {
    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [moronOrder]);

    fireEvent.pointerDown(rowOf(view, '2006').getByRole('button', { name: copy.list.unassigned }), {
      button: 0,
    });
    const menu = await vi.waitFor(() => view.getByRole('menu'));
    expect(within(menu).getAllByRole('menuitem')).toHaveLength(1);
    expect(within(menu).getByRole('menuitem', { name: copy.list.actions.assign })).toBeTruthy();
    // A seller never pulls the branch picklist.
    expect(listSellers).not.toHaveBeenCalled();
  });
});

/*
 * Tests for the clickable Seller column: an admin steers the owner straight from the cell (same
 * items as the row menu), and a seller claims an unassigned order from it. Rows the caller cannot
 * touch stay plain text, and the three-dot menu keeps its own entry points untouched.
 */
describe('RfqDashboard seller column as the assignment surface', () => {
  const moronOrder: RfqRecord = {
    id: '2006',
    quoteNumber: 6,
    client: 'Obra F',
    createdAt: '2026-08-03T05:00:00.000Z',
    channel: 'whatsapp',
    seller: '',
    sellerId: null,
    branch: 'Morón',
    branchId: 'b-moron',
    quoteId: 'quote-b-moron',
    itemCount: 1,
    status: 'RECEIVED',
    needsFollowup: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listSellers).mockImplementation(
      async (branchId) => SELLERS_BY_BRANCH[branchId ?? ''] ?? [],
    );
  });

  // Opens the seller cell's own menu: pointer-down on the cell is what drops it.
  async function openSellerCellMenu(view: ReturnType<typeof render>, rfqId: string, name: string) {
    fireEvent.pointerDown(rowOf(view, rfqId).getByRole('button', { name }), { button: 0 });
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.assign }));
    return view;
  }

  it('opens the seller picklist from the cell of an unassigned order, scoped to its branch', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 's-moron'));
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
      copy.list.unassigned,
    );

    const moron = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' }));
    expect(moron).toBeTruthy();
    expect(view.queryByRole('menuitem', { name: 'Diego Villa' })).toBeNull();
    expect(listSellers).toHaveBeenCalledWith('b-moron');
  });

  it('reassigns an owned order to another seller of the branch from the cell', async () => {
    vi.mocked(listSellers).mockResolvedValue([
      { id: 's-moron', name: 'Vero Morón' },
      { id: 's-moron2', name: 'Caro Morón' },
    ]);
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 's-moron2'));
    const assigned: RfqRecord = { ...moronOrder, sellerId: 's-moron', seller: 'Vero Morón' };
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [assigned]),
      '2006',
      'Vero Morón',
    );

    // The current owner is offered but disabled; the other Morón seller is the move.
    const vero = await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' }));
    expect(vero.getAttribute('data-disabled')).not.toBeNull();
    fireEvent.click(view.getByRole('menuitem', { name: 'Caro Morón' }));

    expect(setRfqSeller).toHaveBeenCalledWith('2006', 's-moron2');
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Caro Morón')).toBeTruthy());
    expect(toast.success).toHaveBeenCalledWith(
      copy.list.toast.sellerAssigned.replace('{id}', '#06').replace('{seller}', 'Caro Morón'),
    );
  });

  it('lets the admin assign the order to themselves from the cell', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 'me'));
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
      copy.list.unassigned,
    );

    fireEvent.click(view.getByRole('menuitem', { name: copy.list.actions.assign }));

    expect(setRfqSeller).toHaveBeenCalledWith('2006', 'me');
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Admin')).toBeTruthy());
  });

  it('clears the owner from the cell', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', null));
    const assigned: RfqRecord = { ...moronOrder, sellerId: 's-moron', seller: 'Vero Morón' };
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [assigned]),
      '2006',
      'Vero Morón',
    );

    fireEvent.click(view.getByRole('menuitem', { name: copy.list.actions.unassignSeller }));

    expect(setRfqSeller).toHaveBeenCalledWith('2006', null);
    await vi.waitFor(() =>
      expect(rowOf(view, '2006').getByText(copy.list.unassigned)).toBeTruthy(),
    );
  });

  it('lets a seller claim an unassigned order from the cell, without admin powers', async () => {
    vi.mocked(assignRfqSeller).mockResolvedValue({
      id: 'q6',
      branch_id: 'b-moron',
      client_id: null,
      rfq_id: '2006',
      seller_id: 'me',
      current_version_id: null,
      current_status: 'RECEIVED',
      expires_at: null,
      archived_at: null,
      needs_followup: false,
      followup_flagged_at: null,
      created_at: '2026-08-03T05:00:00.000Z',
      updated_at: '2026-08-03T05:00:00.000Z',
    });
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [moronOrder]),
      '2006',
      copy.list.unassigned,
    );

    // The cell menu for a seller offers only the claim: no picklist, no steer, no unassign.
    expect(view.getByRole('menuitem', { name: copy.list.actions.assign })).toBeTruthy();
    expect(view.queryByRole('menuitem', { name: copy.list.actions.unassignSeller })).toBeNull();
    expect(view.queryByRole('menuitem', { name: 'Vero Morón' })).toBeNull();
    expect(listSellers).not.toHaveBeenCalled();

    fireEvent.click(view.getByRole('menuitem', { name: copy.list.actions.assign }));
    expect(assignRfqSeller).toHaveBeenCalledWith('2006');
    await vi.waitFor(() => expect(rowOf(view, '2006').getByText('Ana Robles')).toBeTruthy());
  });

  it('leaves the cell as plain text for a seller on someone else\u0027s order', () => {
    const assigned: RfqRecord = { ...moronOrder, sellerId: 's-other', seller: 'Otro Vendedor' };
    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [assigned]);

    const cell = rowOf(view, '2006');
    expect(cell.getByText('Otro Vendedor')).toBeTruthy();
    expect(cell.queryByRole('button', { name: 'Otro Vendedor' })).toBeNull();
  });

  /*
   * The owner menu is portalled, so React sends its click up this row's React tree even though the
   * DOM node is nowhere near it. Picking an owner must not also open the order.
   */
  it('does not open the detail when the cell menu is used', async () => {
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 's-moron'));
    const view = await openSellerCellMenu(
      renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]),
      '2006',
      copy.list.unassigned,
    );

    fireEvent.click(await vi.waitFor(() => view.getByRole('menuitem', { name: 'Vero Morón' })));

    await vi.waitFor(() => expect(setRfqSeller).toHaveBeenCalledWith('2006', 's-moron'));
    expect(router.push).not.toHaveBeenCalled();
  });
});
