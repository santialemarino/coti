import { fireEvent, render, waitFor, within, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { BranchTable } from '@/app/(protected)/settings/branches/_components/branch-table';
import type { Branch } from '@/lib/api/branches';
import { EXPIRY_MAX_DAYS, EXPIRY_MIN_DAYS } from '@/lib/constants/branch';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/branches/actions', () => ({
  createBranch: vi.fn(),
  updateBranch: vi.fn(),
  closeBranch: vi.fn(),
  reopenBranch: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { closeBranch, createBranch, reopenBranch, updateBranch } =
  await import('@/app/(protected)/settings/branches/actions');
const { toast } = await import('sonner');

const copy = messages.branches;

const CENTRAL: Branch = {
  id: 'b1',
  name: 'Casa Central',
  address: 'Av. Siempre Viva 742',
  defaultExpiryDays: 7,
  isActive: true,
};
const MORON: Branch = {
  id: 'b2',
  name: 'Morón',
  address: null,
  defaultExpiryDays: 30,
  isActive: true,
};
const CLOSED: Branch = {
  id: 'b3',
  name: 'Villa Bosch',
  address: 'Av. Márquez 1520',
  defaultExpiryDays: 5,
  isActive: false,
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderTable(branches: Branch[] = [CENTRAL, MORON]) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <BranchTable branches={branches} />
    </NextIntlClientProvider>,
  );
}

function dialog(view: RenderResult): HTMLElement {
  const found = view.baseElement.querySelector('[role="dialog"]');
  if (!(found instanceof HTMLElement)) throw new Error('no dialog on screen');
  return found;
}

function field(view: RenderResult, name: string): HTMLInputElement {
  const input = view.baseElement.querySelector(`input[name="${name}"]`);
  if (!(input instanceof HTMLInputElement)) throw new Error(`no ${name} field on screen`);
  return input;
}

function rowActions(view: RenderResult, name: string) {
  const row = view.getByRole('row', { name: new RegExp(name) });
  return {
    edit: within(row).getByRole('button', { name: copy.edit.action }),
    close: within(row).getByRole('button', { name: copy.close.action }),
  };
}

function rowOf(view: RenderResult, name: string) {
  return view.getByRole('row', { name: new RegExp(name) });
}

// Clicking a submit button does not reach a React form in jsdom, which implements no requestSubmit.
function submitDialog(view: RenderResult) {
  const form = dialog(view).querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form in the dialog');
  fireEvent.submit(form);
}

beforeEach(() => vi.clearAllMocks());

describe('BranchTable listing', () => {
  it('renders a row per branch with its expiry pluralised', () => {
    const view = renderTable();

    expect(view.getByText(CENTRAL.name)).toBeTruthy();
    expect(view.getByText('7 días')).toBeTruthy();
    expect(view.getByText('30 días')).toBeTruthy();
  });

  // A branch with no address must read as one, not as an empty cell that looks like a render bug.
  it('names the absence of an address', () => {
    const view = renderTable();

    expect(view.getByText(copy.table.noAddress)).toBeTruthy();
  });

  it('offers to add the first one when the account has none', () => {
    const view = renderTable([]);

    expect(view.getByText(copy.empty.title)).toBeTruthy();
    expect(view.getByRole('button', { name: copy.add })).toBeTruthy();
  });
});

describe('BranchTable dialogs', () => {
  /*
   * The mode is an explicit prop rather than something derived from whether a branch is selected:
   * the dialog holds its last mode so the copy survives the close animation, so a create opened
   * after an edit would otherwise still be wearing the edit copy.
   */
  it('wears the create copy after having been opened to edit', async () => {
    const view = renderTable();

    fireEvent.click(rowActions(view, CENTRAL.name).edit);
    await waitFor(() => expect(within(dialog(view)).getByText(copy.edit.title)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
  });

  // The dialog outlives every branch it edits, so without a reset on open the second row opened
  // would still be showing the first row's values.
  it('shows the values of the row it was opened on, each time', async () => {
    const view = renderTable();

    fireEvent.click(rowActions(view, CENTRAL.name).edit);
    await waitFor(() => expect(field(view, 'name').value).toBe(CENTRAL.name));
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));

    fireEvent.click(rowActions(view, MORON.name).edit);
    await waitFor(() => expect(field(view, 'name').value).toBe(MORON.name));
    expect(field(view, 'defaultExpiryDays').value).toBe(String(MORON.defaultExpiryDays));
  });

  it('sends an edit to the branch it was opened on', async () => {
    vi.mocked(updateBranch).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(rowActions(view, MORON.name).edit);
    await waitFor(() => expect(field(view, 'name').value).toBe(MORON.name));
    fireEvent.change(field(view, 'name'), { target: { value: 'Morón Centro' } });
    submitDialog(view);

    await waitFor(() => expect(updateBranch).toHaveBeenCalledOnce());
    expect(updateBranch).toHaveBeenCalledWith(MORON.id, {
      name: 'Morón Centro',
      address: '',
      defaultExpiryDays: String(MORON.defaultExpiryDays),
    });
    expect(createBranch).not.toHaveBeenCalled();
  });

  /*
   * A schema message carries its numbers as ICU placeholders, so the range the caller reads comes
   * from the constants rather than from the copy. Only a rendered form shows whether they resolved:
   * the schema test asserts the key, and the key is right either way.
   */
  it('renders a validation message with nothing left to interpolate', async () => {
    const view = renderTable();

    fireEvent.click(rowActions(view, MORON.name).edit);
    await waitFor(() => expect(field(view, 'name').value).toBe(MORON.name));
    fireEvent.change(field(view, 'defaultExpiryDays'), { target: { value: '400' } });
    submitDialog(view);

    const expected = copy.defaultExpiryDays.outOfRange
      .replace('{min}', String(EXPIRY_MIN_DAYS))
      .replace('{max}', String(EXPIRY_MAX_DAYS));
    const message = await waitFor(() => within(dialog(view)).getByText(expected));
    expect(message.textContent).not.toMatch(/[{}]/);
  });

  it('creates rather than edits when opened from the add button', async () => {
    vi.mocked(createBranch).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
    fireEvent.change(field(view, 'name'), { target: { value: 'Villa Bosch' } });
    submitDialog(view);

    await waitFor(() => expect(createBranch).toHaveBeenCalledOnce());
    expect(updateBranch).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.created));
  });
});

describe('BranchTable closing a branch', () => {
  it('confirms with the name of the branch it is about to close', async () => {
    const view = renderTable();

    fireEvent.click(rowActions(view, MORON.name).close);

    await waitFor(() =>
      expect(within(dialog(view)).getByText(new RegExp(MORON.name))).toBeTruthy(),
    );
  });

  /*
   * The refusal belongs to the list, not to a dialog that is about to disappear: an account needing
   * one active branch is a fact about the whole account, and a message inside a closing overlay is
   * gone before it can be read.
   */
  it('puts the last-active refusal on the page and shuts the dialog', async () => {
    vi.mocked(closeBranch).mockResolvedValue({ error: 'LAST_ACTIVE_BRANCH' });
    const view = renderTable([CENTRAL]);

    fireEvent.click(rowActions(view, CENTRAL.name).close);
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.close.confirm }));

    await waitFor(() => expect(view.getByText(messages.errors.LAST_ACTIVE_BRANCH)).toBeTruthy());
    await waitFor(() => expect(view.baseElement.querySelector('[role="dialog"]')).toBeNull());
  });

  it('reports the close once it succeeds', async () => {
    vi.mocked(closeBranch).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(rowActions(view, MORON.name).close);
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.close.confirm }));

    await waitFor(() => expect(closeBranch).toHaveBeenCalledWith(MORON.id));
    // A confirmation of something just done is transient, so it is a toast rather than a standing
    // Callout on the list — which is what the refusal above is.
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.closed));
  });

  /*
   * One transition per action. A shared one only reports that something is running, so closing a
   * branch would put the save dialog's button into its pending state at the same time.
   */
  it('shows the pending state only on the action that is running', async () => {
    let release = () => {};
    vi.mocked(closeBranch).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ ok: true }))),
    );
    const view = renderTable();

    fireEvent.click(rowActions(view, MORON.name).close);
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    // Held by node, not re-queried by name: both labels are on the button while they crossfade, so
    // its accessible name is neither one of them for a moment.
    const confirm = within(dialog(view)).getByRole('button', { name: copy.close.confirm });
    fireEvent.click(confirm);

    await waitFor(() => expect(confirm.getAttribute('aria-busy')).toBe('true'));
    expect(confirm).toHaveProperty('disabled', true);
    expect(confirm.textContent).toContain(copy.close.confirming);

    release();
    await waitFor(() => expect(closeBranch).toHaveBeenCalledOnce());
  });
});

describe('BranchTable closed branches', () => {
  it('marks each branch with its state', () => {
    const view = renderTable([MORON, CLOSED]);

    expect(within(rowOf(view, MORON.name)).getByText(copy.status.active)).toBeTruthy();
    expect(within(rowOf(view, CLOSED.name)).getByText(copy.status.closed)).toBeTruthy();
  });

  /*
   * Closing and reopening are the same axis, so a row offers exactly one of them. Offering both
   * would let an admin close a branch that is already closed and read the result as a change.
   */
  it('offers reopening on a closed row and closing on an active one, never both', () => {
    const view = renderTable([MORON, CLOSED]);

    const active = within(rowOf(view, MORON.name));
    expect(active.getByRole('button', { name: copy.close.action })).toBeTruthy();
    expect(active.queryByRole('button', { name: copy.reopen.action })).toBeNull();

    const closed = within(rowOf(view, CLOSED.name));
    expect(closed.getByRole('button', { name: copy.reopen.action })).toBeTruthy();
    expect(closed.queryByRole('button', { name: copy.close.action })).toBeNull();
  });

  // Reopening is not destructive, so it acts directly rather than asking for a confirmation.
  it('reopens without a confirmation and says so', async () => {
    vi.mocked(reopenBranch).mockResolvedValue({ ok: true });
    const view = renderTable([MORON, CLOSED]);

    fireEvent.click(
      within(rowOf(view, CLOSED.name)).getByRole('button', {
        name: copy.reopen.action,
      }),
    );

    await waitFor(() => expect(reopenBranch).toHaveBeenCalledOnce());
    expect(reopenBranch).toHaveBeenCalledWith(CLOSED);
    expect(view.baseElement.querySelector('[role="dialog"]')).toBeNull();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.reopened));
  });

  it('puts a refused reopen on the list', async () => {
    vi.mocked(reopenBranch).mockResolvedValue({ error: 'NOT_FOUND' });
    const view = renderTable([MORON, CLOSED]);

    fireEvent.click(
      within(rowOf(view, CLOSED.name)).getByRole('button', {
        name: copy.reopen.action,
      }),
    );

    await waitFor(() => expect(view.getByText(copy.errors.NOT_FOUND)).toBeTruthy());
  });
});
