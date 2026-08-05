import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Button } from './button';

describe('Button', () => {
  /*
   * The default that stops a button dropped into a form from submitting it. It did not hold
   * once, and every form in the product relies on it now.
   */
  it('defaults to type="button"', () => {
    render(<Button>Guardar</Button>);
    expect(screen.getByRole('button', { name: 'Guardar' }).getAttribute('type')).toBe('button');
  });

  it('lets a real submit opt in', () => {
    render(<Button type="submit">Guardar</Button>);
    expect(screen.getByRole('button', { name: 'Guardar' }).getAttribute('type')).toBe('submit');
  });

  // Forcing type="button" onto an anchor would be invalid HTML, so asChild passes it through.
  it('stamps no type on an asChild anchor', () => {
    render(
      <Button asChild>
        <a href="/login">Ingresar</a>
      </Button>,
    );
    expect(screen.getByRole('link', { name: 'Ingresar' }).getAttribute('type')).toBeNull();
  });
});
