import { fireEvent, render, waitFor, within, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { UserTable } from '@/app/(protected)/settings/users/_components/user-table';
import type { Branch } from '@/lib/api/branches';
import type { AccountUser } from '@/lib/api/users';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/users/actions', () => ({
  createUser: vi.fn(),
  updateUser: vi.fn(),
  deactivateUser: vi.fn(),
  reactivateUser: vi.fn(),
  sendPasswordReset: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { createUser, deactivateUser, reactivateUser, sendPasswordReset, updateUser } =
  await import('@/app/(protected)/settings/users/actions');
const { toast } = await import('sonner');

const copy = messages.users;

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

const ME: AccountUser = {
  id: 'u1',
  name: 'Ana Gómez',
  email: 'ana@corralon.test',
  role: 'ADMIN',
  isActive: true,
  branchIds: [],
  lastLoginAt: '2026-08-01T13:05:00Z',
};
const SELLER: AccountUser = {
  id: 'u2',
  name: 'Bruno Díaz',
  email: 'bruno@corralon.test',
  role: 'SELLER',
  isActive: true,
  branchIds: [CENTRAL.id],
  lastLoginAt: null,
};
const DEACTIVATED: AccountUser = {
  id: 'u3',
  name: 'Carla Ruiz',
  email: 'carla@corralon.test',
  role: 'SELLER',
  isActive: false,
  branchIds: [MORON.id],
  lastLoginAt: '2026-07-01T09:00:00Z',
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderTable(users: AccountUser[] = [ME, SELLER], branches: Branch[] = [CENTRAL, MORON]) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <UserTable users={users} branches={branches} currentUserId={ME.id} />
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

function rowOf(view: RenderResult, name: string) {
  return view.getByRole('row', { name: new RegExp(name) });
}

function actionsOf(view: RenderResult, name: string) {
  return within(rowOf(view, name));
}

// Clicking a submit button does not reach a React form in jsdom, which implements no requestSubmit.
function submitDialog(view: RenderResult) {
  const form = dialog(view).querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form in the dialog');
  fireEvent.submit(form);
}

async function openEdit(view: RenderResult, name: string) {
  fireEvent.click(actionsOf(view, name).getByRole('button', { name: copy.edit.action }));
  await waitFor(() => expect(dialog(view)).toBeTruthy());
}

beforeEach(() => vi.clearAllMocks());

describe('UserTable listing', () => {
  it('renders a row per user with their role and reach', () => {
    const view = renderTable();

    expect(within(rowOf(view, ME.name)).getByText(messages.common.roles.ADMIN)).toBeTruthy();
    // An admin reaches every branch of the account whatever their assignments say, so listing the
    // assigned ones would misdescribe what they can do.
    expect(within(rowOf(view, ME.name)).getByText(copy.table.allBranches)).toBeTruthy();
    expect(within(rowOf(view, SELLER.name)).getByText(CENTRAL.name)).toBeTruthy();
  });

  it('names the absence of an assignment and of a first login', () => {
    const view = renderTable([{ ...SELLER, branchIds: [] }]);

    expect(view.getByText(copy.table.noBranches)).toBeTruthy();
    expect(view.getByText(copy.table.neverLoggedIn)).toBeTruthy();
  });

  it('marks each user with their state', () => {
    const view = renderTable([SELLER, DEACTIVATED]);

    expect(within(rowOf(view, SELLER.name)).getByText(copy.status.active)).toBeTruthy();
    expect(within(rowOf(view, DEACTIVATED.name)).getByText(copy.status.inactive)).toBeTruthy();
  });

  /*
   * A branch closed after the assignment was made is still in the user's branch_ids, but the API
   * refuses to accept it back, so the row lists what the account can still act on.
   */
  it('lists only the branches that are still open', () => {
    const view = renderTable([{ ...SELLER, branchIds: [CENTRAL.id, 'b-closed'] }], [CENTRAL]);

    expect(within(rowOf(view, SELLER.name)).getByText(CENTRAL.name)).toBeTruthy();
  });
});

/*
 * The API answers all three self-edit guards with 422, and a disabled control cannot fire the
 * tooltip that would explain itself — so the interface hides what it cannot do and says why.
 */
describe('UserTable and the caller’s own user', () => {
  it('offers no way to deactivate yourself, and does offer it for someone else', () => {
    const view = renderTable();

    expect(
      actionsOf(view, ME.name).queryByRole('button', { name: copy.deactivate.action }),
    ).toBeNull();
    expect(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.deactivate.action }),
    ).toBeTruthy();
  });

  it('explains the absence on a control that can still be reached', async () => {
    const view = renderTable();
    const marker = within(rowOf(view, ME.name)).getByText(copy.you);

    fireEvent.focus(marker);

    await waitFor(() => expect(view.getAllByText(copy.yourUser).length).toBeGreaterThan(0));
  });

  it('keeps editing your own profile available', () => {
    const view = renderTable();

    expect(actionsOf(view, ME.name).getByRole('button', { name: copy.edit.action })).toBeTruthy();
  });

  it('leaves the role out of your own dialog and says why', async () => {
    const view = renderTable();

    await openEdit(view, ME.name);

    const open = within(dialog(view));
    await waitFor(() => expect(open.getByText(copy.yourUser)).toBeTruthy());
    expect(open.queryByRole('radiogroup')).toBeNull();
  });

  it('offers the role again on someone else, after having edited yourself', async () => {
    const view = renderTable();

    await openEdit(view, ME.name);
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));
    await openEdit(view, SELLER.name);

    await waitFor(() => expect(within(dialog(view)).getByRole('radiogroup')).toBeTruthy());
  });
});

describe('UserTable dialogs', () => {
  /*
   * The mode is snapshotted rather than derived from whether a user is selected: the dialog holds
   * what it last showed so it survives the close animation, so a create opened after an edit would
   * otherwise still be wearing the edit copy.
   */
  it('wears the create copy after having been opened to edit', async () => {
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(within(dialog(view)).getByText(copy.edit.title)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
  });

  // The dialog outlives every user it edits, so without a reset on open the second row opened would
  // still be showing the first row's values.
  it('shows the values of the row it was opened on, each time', async () => {
    const view = renderTable([ME, SELLER]);

    await openEdit(view, ME.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(ME.name));
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    expect(field(view, 'email').value).toBe(SELLER.email);
  });

  /*
   * A password is set once, at creation. The API's update body carries none, so a field on the edit
   * dialog would collect something nobody sends.
   */
  it('asks for an initial password when creating and never when editing', async () => {
    const view = renderTable();

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
    expect(view.baseElement.querySelector('input[name="password"]')).toBeTruthy();

    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.cancel }));
    await openEdit(view, SELLER.name);

    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    expect(view.baseElement.querySelector('input[name="password"]')).toBeNull();
  });

  it('sends an edit to the user it was opened on', async () => {
    vi.mocked(updateUser).mockResolvedValue({ ok: true });
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    fireEvent.change(field(view, 'name'), { target: { value: 'Bruno D. Díaz' } });
    submitDialog(view);

    await waitFor(() => expect(updateUser).toHaveBeenCalledOnce());
    expect(updateUser).toHaveBeenCalledWith(SELLER.id, {
      name: 'Bruno D. Díaz',
      email: SELLER.email,
      role: SELLER.role,
      branchIds: [CENTRAL.id],
      password: '',
    });
    expect(createUser).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.updated));
  });

  it('creates rather than edits when opened from the add button', async () => {
    vi.mocked(createUser).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
    fireEvent.change(field(view, 'name'), { target: { value: 'Dana López' } });
    fireEvent.change(field(view, 'email'), { target: { value: 'dana@corralon.test' } });
    fireEvent.change(field(view, 'password'), { target: { value: 'Coti-1234-larga' } });
    submitDialog(view);

    await waitFor(() => expect(createUser).toHaveBeenCalledOnce());
    expect(updateUser).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.created));
  });

  /*
   * An address already in use is the one rejection that belongs to a field. As a toast or a standing
   * Callout it would sit somewhere other than the input the caller has to fix, and the dialog covers
   * the list anyway.
   */
  it('puts a refused address on the email field, and nowhere else', async () => {
    vi.mocked(updateUser).mockResolvedValue({ error: 'EMAIL_TAKEN' });
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    submitDialog(view);

    await waitFor(() =>
      expect(within(dialog(view)).getByText(copy.errors.EMAIL_TAKEN)).toBeTruthy(),
    );
    expect(view.getAllByText(copy.errors.EMAIL_TAKEN)).toHaveLength(1);
  });

  it('puts every other refusal on the list', async () => {
    vi.mocked(updateUser).mockResolvedValue({ error: 'NOT_FOUND' });
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    submitDialog(view);

    await waitFor(() => expect(view.getByText(copy.errors.NOT_FOUND)).toBeTruthy());
  });

  /*
   * The form is reset when the dialog opens, never on every render: the dialog resets itself when
   * its assignments change, so a fresh empty array for the create case would wipe whatever the
   * caller had typed the next time anything re-rendered the page under them.
   */
  it('keeps what the caller typed when the page re-renders under it', async () => {
    const view = renderTable();

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());
    fireEvent.change(field(view, 'name'), { target: { value: 'Dana López' } });
    view.rerender(
      <NextIntlClientProvider
        locale="es"
        messages={messages}
        timeZone="America/Argentina/Buenos_Aires"
      >
        <UserTable users={[ME, SELLER]} branches={[CENTRAL, MORON]} currentUserId={ME.id} />
      </NextIntlClientProvider>,
    );

    expect(field(view, 'name').value).toBe('Dana López');
  });

  /*
   * `PendingButton`'s `pending` prop is optional and defaults to false, so a form that passes the
   * label and forgets to wire the state type-checks cleanly and silently never shows it.
   */
  it('shows the pending state on the button that is saving', async () => {
    let release = () => {};
    vi.mocked(updateUser).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ ok: true }))),
    );
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    // Held by node, not re-queried by name: both labels are on the button while they crossfade, so
    // its accessible name is neither one of them for a moment.
    const submit = dialog(view).querySelector('button[type="submit"]');
    if (!(submit instanceof HTMLButtonElement)) throw new Error('no submit button in the dialog');
    submitDialog(view);

    await waitFor(() => expect(submit.getAttribute('aria-busy')).toBe('true'));
    expect(submit).toHaveProperty('disabled', true);
    expect(submit.textContent).toContain(copy.edit.submitting);

    release();
    await waitFor(() => expect(updateUser).toHaveBeenCalledOnce());
  });
});

describe('UserTable branch assignment', () => {
  it('offers a checkbox per branch it was given', async () => {
    const view = renderTable();

    fireEvent.click(view.getByRole('button', { name: copy.add }));
    await waitFor(() => expect(within(dialog(view)).getByText(copy.create.title)).toBeTruthy());

    const group = within(dialog(view)).getByRole('group', { name: copy.branches.label });
    expect(within(group).getAllByRole('checkbox')).toHaveLength(2);
    expect(within(group).getByText(CENTRAL.name)).toBeTruthy();
    expect(within(group).getByText(MORON.name)).toBeTruthy();
  });

  it('opens with the branches the user already holds ticked', async () => {
    vi.mocked(updateUser).mockResolvedValue({ ok: true });
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    fireEvent.click(within(dialog(view)).getByRole('checkbox', { name: MORON.name }));
    submitDialog(view);

    await waitFor(() => expect(updateUser).toHaveBeenCalledOnce());
    expect(vi.mocked(updateUser).mock.calls[0]?.[1].branchIds).toEqual([CENTRAL.id, MORON.id]);
  });

  it('sends an empty list when every branch is unticked', async () => {
    vi.mocked(updateUser).mockResolvedValue({ ok: true });
    const view = renderTable();

    await openEdit(view, SELLER.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    fireEvent.click(within(dialog(view)).getByRole('checkbox', { name: CENTRAL.name }));
    submitDialog(view);

    await waitFor(() => expect(updateUser).toHaveBeenCalledOnce());
    expect(vi.mocked(updateUser).mock.calls[0]?.[1].branchIds).toEqual([]);
  });

  /*
   * An assignment to a branch that has since closed cannot be sent back, so saving drops it. The
   * caller did not ask to lose it and the checkbox group cannot show it, so the dialog says so.
   */
  it('warns that a closed assignment is about to be dropped, and drops it', async () => {
    vi.mocked(updateUser).mockResolvedValue({ ok: true });
    const stale = { ...SELLER, branchIds: [CENTRAL.id, 'b-closed'] };
    const view = renderTable([stale], [CENTRAL, MORON]);

    await openEdit(view, stale.name);
    await waitFor(() => expect(field(view, 'name').value).toBe(stale.name));
    expect(within(dialog(view)).getByText(/cerrada/)).toBeTruthy();
    submitDialog(view);

    await waitFor(() => expect(updateUser).toHaveBeenCalledOnce());
    expect(vi.mocked(updateUser).mock.calls[0]?.[1].branchIds).toEqual([CENTRAL.id]);
  });

  it('says nothing about closed branches when there are none', async () => {
    const view = renderTable();

    await openEdit(view, SELLER.name);

    await waitFor(() => expect(field(view, 'name').value).toBe(SELLER.name));
    expect(within(dialog(view)).queryByText(/cerrada/)).toBeNull();
  });
});

describe('UserTable deactivating and reactivating', () => {
  it('confirms with the name of the user it is about to deactivate', async () => {
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.deactivate.action }),
    );

    await waitFor(() =>
      expect(within(dialog(view)).getByText(new RegExp(SELLER.name))).toBeTruthy(),
    );
  });

  it('reports the deactivation once it succeeds', async () => {
    vi.mocked(deactivateUser).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.deactivate.action }),
    );
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.deactivate.confirm }));

    await waitFor(() => expect(deactivateUser).toHaveBeenCalledWith(SELLER.id));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.deactivated));
  });

  /*
   * The refusal belongs to the list, not to a dialog that is about to disappear: a message inside a
   * closing overlay is gone before it can be read.
   */
  it('puts a refused deactivation on the page and shuts the dialog', async () => {
    vi.mocked(deactivateUser).mockResolvedValue({ error: 'SELF_DEACTIVATION' });
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.deactivate.action }),
    );
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.deactivate.confirm }));

    await waitFor(() => expect(view.getByText(messages.errors.SELF_DEACTIVATION)).toBeTruthy());
    await waitFor(() => expect(view.baseElement.querySelector('[role="dialog"]')).toBeNull());
  });

  /*
   * Deactivating and reactivating are the same axis, so a row offers exactly one of them — and a
   * deactivated user gets no recovery link, which the API refuses anyway.
   */
  it('offers reactivating on a deactivated row and nothing else on that axis', () => {
    const view = renderTable([SELLER, DEACTIVATED]);

    const off = actionsOf(view, DEACTIVATED.name);
    expect(off.getByRole('button', { name: copy.reactivate.action })).toBeTruthy();
    expect(off.queryByRole('button', { name: copy.deactivate.action })).toBeNull();
    expect(off.queryByRole('button', { name: copy.passwordReset.action })).toBeNull();

    const on = actionsOf(view, SELLER.name);
    expect(on.queryByRole('button', { name: copy.reactivate.action })).toBeNull();
    expect(on.getByRole('button', { name: copy.passwordReset.action })).toBeTruthy();
  });

  // Reactivating is not destructive, so it acts directly rather than asking for a confirmation.
  it('reactivates without a confirmation, keeping only the branches still open', async () => {
    vi.mocked(reactivateUser).mockResolvedValue({ ok: true });
    const stale = { ...DEACTIVATED, branchIds: [MORON.id, 'b-closed'] };
    const view = renderTable([stale], [CENTRAL, MORON]);

    fireEvent.click(
      actionsOf(view, stale.name).getByRole('button', { name: copy.reactivate.action }),
    );

    await waitFor(() => expect(reactivateUser).toHaveBeenCalledOnce());
    expect(reactivateUser).toHaveBeenCalledWith({
      id: stale.id,
      name: stale.name,
      email: stale.email,
      role: stale.role,
      branchIds: [MORON.id],
    });
    expect(view.baseElement.querySelector('[role="dialog"]')).toBeNull();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.reactivated));
  });
});

describe('UserTable mailing a recovery link', () => {
  // The confirmation names the address, because that is what the caller cannot verify from the row
  // once the mail is gone.
  it('confirms with the address the link is going to', async () => {
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.passwordReset.action }),
    );

    await waitFor(() =>
      expect(within(dialog(view)).getByText(new RegExp(SELLER.email))).toBeTruthy(),
    );
  });

  it('reports the link once it is sent', async () => {
    vi.mocked(sendPasswordReset).mockResolvedValue({ ok: true });
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.passwordReset.action }),
    );
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.passwordReset.confirm }));

    await waitFor(() => expect(sendPasswordReset).toHaveBeenCalledWith(SELLER.id));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.passwordResetSent));
  });

  it('puts the mail allowance running out on the list', async () => {
    vi.mocked(sendPasswordReset).mockResolvedValue({ error: 'RATE_LIMITED' });
    const view = renderTable();

    fireEvent.click(
      actionsOf(view, SELLER.name).getByRole('button', { name: copy.passwordReset.action }),
    );
    await waitFor(() => expect(dialog(view)).toBeTruthy());
    fireEvent.click(within(dialog(view)).getByRole('button', { name: copy.passwordReset.confirm }));

    await waitFor(() =>
      expect(view.getByText(copy.passwordReset.errors.RATE_LIMITED)).toBeTruthy(),
    );
  });
});
