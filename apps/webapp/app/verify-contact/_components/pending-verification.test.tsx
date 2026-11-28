import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { requestContactVerification } from '@/app/profile/actions';
import { PendingVerification } from '@/app/verify-contact/_components/pending-verification';
import type { CustomerContact } from '@/lib/api/customer-profile-model';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/app/profile/actions', () => ({
  requestContactVerification: vi.fn(),
  confirmContactVerification: vi.fn(),
  readContactVerification: vi.fn(),
}));

const copy = messages.customerVerification.pending;
const proofCopy = messages.customerProfile.whatsappProof;
const email: CustomerContact = { kind: 'EMAIL', value: 'obra@cliente.test', primary: true };
const phone: CustomerContact = { kind: 'WHATSAPP', value: '+5491155550000', primary: false };

beforeEach(() => vi.clearAllMocks());

function renderPending(props: Partial<React.ComponentProps<typeof PendingVerification>> = {}) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <PendingVerification
        contacts={[email, phone]}
        next="/quotes/tok-1"
        supplierName="Corralón San Martín"
        ownershipUnproved={false}
        {...props}
      />
    </NextIntlClientProvider>,
  );
}

it('says a link is on its way only when this flow sent it', () => {
  renderPending({ sentEmail: email.value });
  expect(screen.getByText(copy.emailSent)).toBeTruthy();
  expect(screen.getByRole('button', { name: copy.resend })).toBeTruthy();
});

it('shows the WhatsApp message to send at once for a proof this flow started', () => {
  renderPending({
    sentProof: {
      id: 'c-1',
      code: '123456',
      link: 'https://wa.me/5491100000000?text=VERIFICAR%20123456',
    },
  });
  expect(screen.getByText(copy.phoneSent)).toBeTruthy();
  expect(screen.getByRole('dialog')).toBeTruthy();
  expect(screen.getByText('VERIFICAR 123456')).toBeTruthy();
  expect(screen.getByRole('link', { name: proofCopy.open }).getAttribute('href')).toBe(
    'https://wa.me/5491100000000?text=VERIFICAR%20123456',
  );
});

it('offers to send them when nothing went out yet', () => {
  renderPending();
  expect(screen.getByText(copy.email)).toBeTruthy();
  expect(screen.queryByText(copy.emailSent)).toBeNull();
  expect(screen.getByRole('button', { name: copy.send })).toBeTruthy();
  expect(screen.getByText(copy.phone)).toBeTruthy();
});

it('sends the link with the page to return to, then says it is on its way', async () => {
  vi.mocked(requestContactVerification).mockResolvedValue({ id: 'c-2' });
  renderPending();
  fireEvent.click(screen.getByRole('button', { name: copy.send }));
  expect(await screen.findByText(copy.emailSent)).toBeTruthy();
  expect(requestContactVerification).toHaveBeenCalledWith('EMAIL', email.value, '/quotes/tok-1');
  expect(screen.getByRole('button', { name: copy.resend })).toBeTruthy();
});

it('guides an earlier profile owner to the contacts that profile held instead of a dead end', () => {
  renderPending({ contacts: [], ownershipUnproved: true });
  expect(
    screen.getByText(copy.earlierProfile.replace('{supplier}', 'Corralón San Martín')),
  ).toBeTruthy();
  expect(screen.getByRole('link', { name: copy.earlierProfileAction }).getAttribute('href')).toBe(
    '/profile',
  );
});

it('offers WhatsApp to a customer with no contact at all, since the number they write from is confirmed', () => {
  renderPending({ contacts: [] });
  expect(screen.getByText(copy.whatsappAny)).toBeTruthy();
  expect(screen.getByText(copy.phone)).toBeTruthy();
});
