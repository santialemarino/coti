import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { confirmContactVerification } from '@/app/profile/actions';
import { ContactEmailConfirmation } from '@/app/verify-contact/_components/contact-email-confirmation';
import messages from '@/translations/es.json';

const router = vi.hoisted(() => ({ replace: vi.fn(), refresh: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => router }));
vi.mock('@/app/profile/actions', () => ({ confirmContactVerification: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

it('takes the customer to «Mis datos» once the contact is confirmed', async () => {
  vi.mocked(confirmContactVerification).mockResolvedValue({});
  window.history.replaceState(null, '', '/c/corralon-san-martin/verify-contact');
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ContactEmailConfirmation challenge="challenge" token="token" />
    </NextIntlClientProvider>,
  );

  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.confirmContact }));

  await waitFor(() =>
    expect(router.replace).toHaveBeenCalledWith('/c/corralon-san-martin/profile'),
  );
  expect(confirmContactVerification).toHaveBeenCalledWith('challenge', 'token');
  expect(router.refresh).not.toHaveBeenCalled();
});
