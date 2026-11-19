import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import messages from '@/translations/es.json';
import { QuoteContact } from './quote-contact';

function renderContact(branch: { whatsapp?: string; email?: string }) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <QuoteContact branch={{ name: 'Casa central', ...branch }} />
    </NextIntlClientProvider>,
  );
}

describe('QuoteContact', () => {
  it('links the branch WhatsApp and mailbox', () => {
    const view = renderContact({
      whatsapp: '+54 9 11 5555-0101',
      email: 'ventas@corralon.test',
    });

    expect(view.getByText(messages.quote.contact.title)).toBeTruthy();
    expect(view.getByText('+54 9 11 5555-0101').closest('a')?.getAttribute('href')).toBe(
      'https://wa.me/5491155550101',
    );
    expect(view.getByText('ventas@corralon.test').closest('a')?.getAttribute('href')).toBe(
      'mailto:ventas@corralon.test',
    );
  });

  it('shows a number without its country code but does not link it', () => {
    const view = renderContact({ whatsapp: '11 5555-0101' });

    expect(view.getByText('11 5555-0101').closest('a')).toBeNull();
  });

  it('renders nothing for a quote frozen without the branch channels', () => {
    const view = renderContact({});

    expect(view.container.textContent).toBe('');
  });
});
