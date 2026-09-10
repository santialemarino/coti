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
  setRfqSeller: vi.fn(),
}));
vi.mock('@/lib/api/sellers', () => ({
  listSellers: vi.fn().mockResolvedValue([]),
}));

const { toast } = await import('sonner');
const { assignRfqSeller, setRfqSeller } = await import('@/lib/api/rfqs-client');
const { listSellers } = await import('@/lib/api/sellers');

const copy = messages.rfqs;
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

function tabCount(view: ReturnType<typeof render>, label: string): number {
  const group = view.getByLabelText(copy.list.tabs);
  const item = within(group).getByText(label);
  const count = (item.textContent ?? '').replace(label, '');
  return Number(count);
}

function rowOf(view: ReturnType<typeof render>, label: string) {
  const row = view.getByText(TEST_REFERENCES[label] ?? label).closest('tr');
  if (!row) throw new Error(`No row contains ${label}`);
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
    expect(view.queryByText('#03')).toBeNull();
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
    await vi.waitFor(() => expect(view.queryByText('#02')).toBeNull());
    expect(view.getByText('#01')).toBeTruthy();
  });
});

describe('RfqDashboard totals column', () => {
  it('shows a dash until the quote exists and the amount once it does', () => {
    const view = renderDashboard();

    expect(rowOf(view, copy.list.numberPending).getByText('-')).toBeTruthy();
    expect(rowOf(view, '#01').getByText('$ 100,00')).toBeTruthy();
    expect(rowOf(view, '#01').queryByText('-')).toBeNull();
  });
});

describe('RfqDashboard row actions', () => {
  it('opens the detail from the whole row without hijacking its controls', () => {
    const view = renderDashboard();
    const row = view.getByRole('link', {
      name: copy.list.openRow.replace('{id}', '#01'),
    });

    fireEvent.click(within(row).getByText('Centro'));
    expect(router.push).toHaveBeenLastCalledWith('/rfqs/2001');

    router.push.mockClear();
    fireEvent.keyDown(row, { key: 'Enter' });
    expect(router.push).toHaveBeenLastCalledWith('/rfqs/2001');

    router.push.mockClear();
    fireEvent.click(within(row).getByRole('checkbox'));
    expect(router.push).not.toHaveBeenCalled();
  });

  it('shows only detail and archive actions and opens the detail route', async () => {
    const view = renderDashboard();

    fireEvent.pointerDown(
      rowOf(view, '#01').getByRole('button', { name: copy.list.actions.more }),
      {
        button: 0,
      },
    );
    const menu = await vi.waitFor(() => view.getByRole('menu'));
    expect(within(menu).getAllByRole('menuitem')).toHaveLength(2);
    expect(within(menu).getByRole('menuitem', { name: copy.list.actions.archive })).toBeTruthy();

    fireEvent.click(within(menu).getByRole('menuitem', { name: copy.list.actions.view }));

    expect(router.push).toHaveBeenCalledWith('/rfqs/2001');
  });

  it('uses the sequence number in archive feedback', async () => {
    const view = renderDashboard();

    fireEvent.pointerDown(
      rowOf(view, '#01').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    fireEvent.click(
      await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.archive })),
    );

    expect(toast.success).toHaveBeenCalledWith(copy.list.toast.archived.replace('{id}', '#01'));
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

  it('offers Asignarme to a seller on an unassigned row and stamps the owner on the claim', async () => {
    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [unassigned]);

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    const assign = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.assign }),
    );
    fireEvent.click(assign);

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

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    const assign = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.assign }),
    );
    fireEvent.click(assign);

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

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    const assign = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.assign }),
    );
    fireEvent.click(assign);

    expect(assignRfqSeller).toHaveBeenCalledWith('2006');
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith(copy.errors.FORBIDDEN));
    expect(view.queryByText('Ana Robles')).toBeNull();
  });

  it('shows no assignable item when the order already has a seller', async () => {
    const assigned: RfqRecord = { ...unassigned, sellerId: 'other', seller: 'Otro Vendedor' };

    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [assigned]);

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    // The menu opens with the usual actions, but "Asignarme" must not appear.
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.view }));
    expect(view.queryByRole('menuitem', { name: copy.list.actions.assign })).toBeNull();
  });

  it('shows the item is disabled/non-claimable for an admin', async () => {
    const view = renderAs({ userName: 'Admin', userId: 'u1', isAdmin: true }, [unassigned]);

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    // The menu still shows the usual actions, but an admin never sees "Asignarme".
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.view }));
    expect(view.queryByRole('menuitem', { name: copy.list.actions.assign })).toBeNull();
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
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listSellers).mockImplementation(
      async (branchId) => SELLERS_BY_BRANCH[branchId ?? ''] ?? [],
    );
    vi.mocked(setRfqSeller).mockResolvedValue(quoteResponse('2006', 'b-moron', 's-moron'));
  });

  // Opens the admin row menu, then the seller submenu, and returns its content.
  async function openSellerMenu(view: ReturnType<typeof render>, rfqId: string) {
    fireEvent.pointerDown(
      rowOf(view, rfqId).getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    const trigger = await vi.waitFor(() =>
      view.getByRole('menuitem', { name: copy.list.actions.changeSeller }),
    );
    fireEvent.pointerMove(trigger, { pointerType: 'mouse' });
    fireEvent.pointerDown(trigger, { button: 0 });
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

  it('hides the owner steered items when the caller is a seller', async () => {
    const view = renderAs({ userName: 'Ana Robles', userId: 'me', isAdmin: false }, [moronOrder]);

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.assign }));
    expect(view.queryByRole('menuitem', { name: copy.list.actions.changeSeller })).toBeNull();
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
    expect(view.queryByRole('menuitem', { name: copy.list.actions.changeSeller })).toBeNull();
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

  it('keeps the three-dot menu functional next to the interactive cell', async () => {
    const view = renderAs({ userName: 'Admin', userId: 'me', isAdmin: true }, [moronOrder]);

    fireEvent.pointerDown(
      rowOf(view, '2006').getByRole('button', { name: copy.list.actions.more }),
      { button: 0 },
    );
    await vi.waitFor(() => view.getByRole('menuitem', { name: copy.list.actions.view }));
    expect(view.getByRole('menuitem', { name: copy.list.actions.changeSeller })).toBeTruthy();
  });
});
