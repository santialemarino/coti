import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Callout } from './callout';

describe('Callout', () => {
  it('renders the action beside the message', () => {
    render(
      <Callout title="Falta el correo" action={<a href="/settings/branches">Configurar</a>}>
        Sin él, el cliente no puede responder.
      </Callout>,
    );

    const action = screen.getByRole('link', { name: 'Configurar' });
    expect(action.closest('[data-slot="callout-action"]')).not.toBeNull();
    expect(screen.getByText('Sin él, el cliente no puede responder.')).not.toBeNull();
  });

  it('leaves no empty action box when there is nothing to do', () => {
    const { container } = render(<Callout>Solo información.</Callout>);

    expect(container.querySelector('[data-slot="callout-action"]')).toBeNull();
  });
});
