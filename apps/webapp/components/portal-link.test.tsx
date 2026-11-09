import { render } from '@testing-library/react';
import { expect, it, vi } from 'vitest';

import { PortalLink, PortalProvider } from '@/components/portal-link';

const route = vi.hoisted(() => ({ pathname: '/c/marko/orders' }));
vi.mock('next/navigation', () => ({ usePathname: () => route.pathname }));

it('updates supplier links when navigation reuses the root layout across portals', () => {
  const content = (
    <PortalProvider slug="marko">
      <PortalLink href="/profile">Profile</PortalLink>
    </PortalProvider>
  );
  const view = render(content);
  expect(view.getByRole('link').getAttribute('href')).toBe('/c/marko/profile');
  route.pathname = '/c/other/orders';
  view.rerender(
    <PortalProvider slug="marko">
      <PortalLink href="/profile">Profile</PortalLink>
    </PortalProvider>,
  );
  expect(view.getByRole('link').getAttribute('href')).toBe('/c/other/profile');
});
