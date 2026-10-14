import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { SettingsNav } from '@/app/(protected)/_components/settings-nav';
import { ROUTES } from '@/config/routes';

const navigation = vi.hoisted(() => ({ pathname: '/settings/invoicing' }));
vi.mock('next/navigation', () => ({ usePathname: () => navigation.pathname }));

const ITEMS = [
  { href: ROUTES.accountSettings, label: 'Cuenta' },
  { href: ROUTES.invoicingSettings, label: 'Facturación' },
];

function current(pathname: string) {
  navigation.pathname = pathname;
  render(<SettingsNav title="Configuración" items={ITEMS} />);
  return screen
    .getAllByRole('link')
    .filter((link) => link.getAttribute('aria-current') === 'page')
    .map((link) => link.textContent);
}

describe('SettingsNav', () => {
  it('marks the section that is open', () => {
    expect(current(ROUTES.invoicingSettings)).toEqual(['Facturación']);
  });

  // The ARCA connection lives inside Facturación, so the section stays lit there.
  it('keeps a section lit on a page inside it', () => {
    expect(current(ROUTES.arcaSettings)).toEqual(['Facturación']);
  });

  it('does not light a section whose path only starts the same', () => {
    expect(current('/settings/accounts')).toEqual([]);
  });
});
