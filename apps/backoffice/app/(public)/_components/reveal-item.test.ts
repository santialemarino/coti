import { describe, expect, it } from 'vitest';

import { revealItem } from '@/app/(public)/_components/reveal-item';

describe('revealItem', () => {
  it('marks the element and carries its place in the stagger', () => {
    expect(revealItem(3, 'flex')).toEqual({
      className: 'reveal-item flex',
      style: { '--reveal-index': 3 },
    });
  });
});
