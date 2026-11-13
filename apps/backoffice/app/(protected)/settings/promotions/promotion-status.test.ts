import { describe, expect, it } from 'vitest';

import {
  argentinaDay,
  promotionStatus,
} from '@/app/(protected)/settings/promotions/promotion-status';
import type { Promotion } from '@/lib/api/promotions';

const NOW = new Date('2026-11-12T15:00:00Z');

function promotion(overrides: Partial<Promotion> = {}): Promotion {
  return {
    id: 'p1',
    branchId: null,
    name: 'Cemento 10 %',
    description: null,
    conditionType: 'PER_ITEM',
    actionType: 'PERCENTAGE',
    actionValue: '10',
    minTotal: null,
    validFrom: null,
    validTo: null,
    isActive: true,
    isExclusive: false,
    priority: 0,
    conditions: [],
    tiers: [],
    ...overrides,
  };
}

describe('promotionStatus', () => {
  it('reads an open-ended active rule as in force', () => {
    expect(promotionStatus(promotion(), NOW)).toBe('ACTIVE');
  });

  it('reads the window when the rule is on', () => {
    expect(promotionStatus(promotion({ validFrom: '2026-11-13T03:00:00Z' }), NOW)).toBe(
      'SCHEDULED',
    );
    expect(promotionStatus(promotion({ validTo: '2026-11-12T02:59:59Z' }), NOW)).toBe('EXPIRED');
  });

  it('reads a switched-off rule as inactive whatever its dates say', () => {
    expect(
      promotionStatus(promotion({ isActive: false, validTo: '2026-01-01T00:00:00Z' }), NOW),
    ).toBe('INACTIVE');
  });
});

describe('argentinaDay', () => {
  it('names the Argentine calendar day, not the UTC one', () => {
    // 23:59:59 in Buenos Aires is already the next day in UTC.
    expect(argentinaDay('2026-11-30T23:59:59-03:00')).toBe('2026-11-30');
    expect(argentinaDay('2026-12-01T00:00:00-03:00')).toBe('2026-12-01');
  });
});
