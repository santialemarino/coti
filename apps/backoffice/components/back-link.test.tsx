import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { BackLink } from '@/components/back-link';

describe('BackLink', () => {
  it('names the way back and leads to the list it came from', () => {
    render(<BackLink href="/settings" label="Volver a la configuración" />);

    const link = screen.getByRole('link', { name: 'Volver a la configuración' });
    expect(link.getAttribute('href')).toBe('/settings');
  });
});
