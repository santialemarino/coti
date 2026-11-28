import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it, vi } from 'vitest';

import { updateClientTagRule } from '@/app/(protected)/settings/client-tags/actions';
import messages from '@/translations/es.json';
import { ClientTagRuleForm } from './client-tag-rule-form';

vi.mock('@/app/(protected)/settings/client-tags/actions', () => ({
  updateClientTagRule: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

it('validates the threshold and saves the switch and rolling window together', async () => {
  vi.mocked(updateClientTagRule).mockResolvedValue(null);
  const t = messages.clientTagSettings;
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ClientTagRuleForm rule={{ kind: 'ABANDONER', enabled: true, minimum: 3, windowDays: 0 }} />
    </NextIntlClientProvider>,
  );

  const minimum = view.getByRole('spinbutton', { name: new RegExp(t.minimum) });
  fireEvent.change(minimum, { target: { value: '0' } });
  fireEvent.click(view.getByRole('button', { name: t.save }));
  expect(await view.findByText(t.invalidMinimum)).toBeTruthy();
  expect(updateClientTagRule).not.toHaveBeenCalled();

  fireEvent.change(minimum, { target: { value: '5' } });
  fireEvent.change(view.getByRole('spinbutton', { name: new RegExp(t.windowDays) }), {
    target: { value: '30' },
  });
  fireEvent.click(view.getByRole('switch', { name: t.enabled }));
  fireEvent.click(view.getByRole('button', { name: t.save }));

  await waitFor(() =>
    expect(updateClientTagRule).toHaveBeenCalledWith({
      kind: 'ABANDONER',
      enabled: false,
      minimum: 5,
      windowDays: 30,
    }),
  );
});
