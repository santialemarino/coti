import type { Promotion } from '@/lib/api/promotions';

// Argentina keeps UTC−3 all year, so the calendar day a rule starts or ends on is a fixed shift.
const ARGENTINA_OFFSET_MS = 3 * 60 * 60 * 1000;

export const PROMOTION_STATUSES = ['ACTIVE', 'SCHEDULED', 'EXPIRED', 'INACTIVE'] as const;
export type PromotionStatus = (typeof PROMOTION_STATUSES)[number];

/*
 * The calendar day (YYYY-MM-DD) an instant falls on in Argentina. Read off the instant rather than
 * formatted in the server's zone, which would show a rule ending at 23:59 as ending the next day.
 */
export function argentinaDay(iso: string): string {
  return new Date(Date.parse(iso) - ARGENTINA_OFFSET_MS).toISOString().slice(0, 10);
}

/*
 * Whether the engine would apply the rule right now. A switched-off rule reads as inactive whatever
 * its dates say; otherwise the window decides, so a rule out of range shows as such without anyone
 * having to delete or switch it off.
 */
export function promotionStatus(promotion: Promotion, now: Date): PromotionStatus {
  if (!promotion.isActive) return 'INACTIVE';
  if (promotion.validTo && Date.parse(promotion.validTo) < now.getTime()) return 'EXPIRED';
  if (promotion.validFrom && Date.parse(promotion.validFrom) > now.getTime()) return 'SCHEDULED';
  return 'ACTIVE';
}
