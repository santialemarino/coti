import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { PublicRfqForm } from '@/app/rfq/[channelId]/_components/public-rfq-form';
import messages from '@/translations/es.json';

vi.mock('motion/react', async (original) => ({
  ...(await original<typeof import('motion/react')>()),
  useReducedMotion: () => true,
}));

const destination = {
  supplierName: 'Corralón Norte',
  branchName: 'Morón',
  maxTextCharacters: 1000,
  maxFileSize: 1024,
  acceptedContentTypes: ['text/plain'],
};
const channelId = '12345678-1234-4234-8234-123456789012';
const copy = messages.rfq;

function renderForm() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <PublicRfqForm channelId={channelId} destination={destination} />
    </NextIntlClientProvider>,
  );
}

async function fillContact(view: ReturnType<typeof renderForm>) {
  fireEvent.change(view.getByLabelText(copy.fields.taxId.label, { exact: false }), {
    target: { value: '20123456786' },
  });
  fireEvent.change(view.getByLabelText(copy.fields.name.label, { exact: false }), {
    target: { value: 'Obra Norte' },
  });
  fireEvent.change(view.getByLabelText(copy.fields.email.label, { exact: false }), {
    target: { value: 'contacto@example.com' },
  });
  fireEvent.click(view.getByRole('button', { name: copy.contact.continue }));
  await waitFor(() => expect(view.getByRole('heading', { name: copy.order.title })).toBeTruthy());
}

describe('stateless public RFQ form', () => {
  it('does not submit an order when Continue is pressed twice', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const view = renderForm();
    fireEvent.change(view.getByLabelText(copy.fields.taxId.label, { exact: false }), {
      target: { value: '20123456786' },
    });
    fireEvent.change(view.getByLabelText(copy.fields.name.label, { exact: false }), {
      target: { value: 'Obra Norte' },
    });
    fireEvent.change(view.getByLabelText(copy.fields.email.label, { exact: false }), {
      target: { value: 'contacto@example.com' },
    });
    const next = view.getByRole('button', { name: copy.contact.continue });
    fireEvent.click(next);
    fireEvent.click(next);
    await waitFor(() => expect(view.getByRole('heading', { name: copy.order.title })).toBeTruthy());
    expect(fetchMock).not.toHaveBeenCalled();
    expect(view.queryByText(copy.validation.text.required)).toBeNull();
  });

  it('submits only the chosen file mode with the current contact declarations', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ received: true }) });
    vi.stubGlobal('fetch', fetchMock);
    const view = renderForm();
    await fillContact(view);
    fireEvent.click(view.getByRole('radio', { name: copy.order.fileMode }));
    const input = view.container.querySelector('input[type="file"]')!;
    fireEvent.change(input, {
      target: { files: [new File(['cemento'], 'pedido.txt', { type: 'text/plain' })] },
    });
    fireEvent.click(await view.findByRole('button', { name: copy.submit }));
    await waitFor(() => expect(view.getByText(copy.received.title)).toBeTruthy());
    const body = fetchMock.mock.calls[0]![1].body as FormData;
    expect(body.get('text')).toBeNull();
    expect(body.get('tax_id')).toBe('20123456786');
    expect((body.get('file') as File).name).toBe('pedido.txt');
  });

  it('keeps contact data only within the current submission and starts empty after receipt', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ received: true }) });
    vi.stubGlobal('fetch', fetchMock);
    const view = renderForm();
    await fillContact(view);
    fireEvent.click(view.getByRole('button', { name: copy.order.editContact }));
    await waitFor(() =>
      expect(view.getByLabelText(copy.fields.name.label, { exact: false })).toHaveProperty(
        'value',
        'Obra Norte',
      ),
    );
    fireEvent.click(view.getByRole('button', { name: copy.contact.continue }));
    await waitFor(() => expect(view.getByRole('textbox', { name: /^Pedido/ })).toBeTruthy());
    fireEvent.change(view.getByRole('textbox', { name: /^Pedido/ }), {
      target: { value: '10 bolsas de cemento' },
    });
    fireEvent.click(await view.findByRole('button', { name: copy.submit }));
    await waitFor(() => expect(view.getByText(copy.received.title)).toBeTruthy());
    const body = JSON.parse(fetchMock.mock.calls[0]![1].body);
    expect(body).toMatchObject({
      tax_id: '20123456786',
      name: 'Obra Norte',
      email: 'contacto@example.com',
      text: '10 bolsas de cemento',
    });
    expect(body.client_id).toBeUndefined();
    fireEvent.click(view.getByRole('button', { name: copy.received.another }));
    await waitFor(() =>
      expect(view.getByLabelText(copy.fields.name.label, { exact: false })).toHaveProperty(
        'value',
        '',
      ),
    );
    view.unmount();
    const fresh = renderForm();
    expect(fresh.getByLabelText(copy.fields.taxId.label, { exact: false })).toHaveProperty(
      'value',
      '',
    );
  });

  it('reuses a retry identifier after an uncertain network outcome', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('connection lost'))
      .mockResolvedValue({ ok: true, json: async () => ({ received: true }) });
    vi.stubGlobal('fetch', fetchMock);
    const view = renderForm();
    await fillContact(view);
    fireEvent.change(view.getByRole('textbox', { name: /^Pedido/ }), {
      target: { value: '10 bolsas' },
    });
    fireEvent.click(await view.findByRole('button', { name: copy.submit }));
    await waitFor(() => expect(view.getByText(copy.errors.UNREACHABLE)).toBeTruthy());
    fireEvent.click(await view.findByRole('button', { name: copy.submit }));
    await waitFor(() => expect(view.getByText(copy.received.title)).toBeTruthy());
    expect(JSON.parse(fetchMock.mock.calls[0]![1].body).request_id).toBe(
      JSON.parse(fetchMock.mock.calls[1]![1].body).request_id,
    );
  });
});
