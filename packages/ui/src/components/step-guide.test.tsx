import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { StepGuide, type StepGuideStep } from './step-guide';

const LABELS = { done: 'Completado', current: 'Paso actual', pending: 'Pendiente' };

function guide(steps: StepGuideStep[]) {
  render(<StepGuide steps={steps} statusLabels={LABELS} />);
  return screen.getAllByRole('listitem');
}

describe('StepGuide', () => {
  it('marks only the first current step as the position', () => {
    const items = guide([
      { id: 'a', title: 'Generar', status: 'done' },
      { id: 'b', title: 'Copiar', status: 'current' },
      { id: 'c', title: 'Autorizar', status: 'current' },
      { id: 'd', title: 'Puntos de venta', status: 'pending' },
    ]);

    expect(items.map((item) => item.getAttribute('aria-current'))).toEqual([
      null,
      'step',
      null,
      null,
    ]);
  });

  it('names each state in words, not only in colour', () => {
    guide([
      { id: 'a', title: 'Generar', status: 'done' },
      { id: 'b', title: 'Copiar', status: 'pending' },
    ]);

    expect(screen.getByRole('heading', { name: 'Generar. Completado' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Copiar. Pendiente' })).toBeTruthy();
  });

  it('titles each step one level under the page heading unless told otherwise', () => {
    const { rerender } = render(
      <StepGuide statusLabels={LABELS} steps={[{ id: 'a', title: 'Generar', status: 'done' }]} />,
    );
    expect(screen.getByRole('heading', { level: 2 })).toBeTruthy();

    rerender(
      <StepGuide
        headingLevel={3}
        statusLabels={LABELS}
        steps={[{ id: 'a', title: 'Generar', status: 'done' }]}
      />,
    );
    expect(screen.getByRole('heading', { level: 3 })).toBeTruthy();
  });

  it('renders what each step asks for', () => {
    render(
      <StepGuide
        statusLabels={LABELS}
        steps={[
          { id: 'a', title: 'Generar', status: 'current', children: <button>Generar</button> },
        ]}
      />,
    );

    expect(screen.getByRole('button', { name: 'Generar' })).toBeTruthy();
  });
});
