'use client';

import { useTranslations } from 'next-intl';

import type { HandlerAdaptation } from '@/lib/api/rfqs';
import type { HandlerStrategy } from '@/lib/constants/conversation';
import { useFormatters } from '@/lib/i18n/formatters';

interface HandlerRecipeNoteProps {
  strategies?: HandlerStrategy[];
  adaptation?: HandlerAdaptation | null;
}

/*
 * What a recipe did on its way to the customer's goal: each quantity raised to reach a promotion,
 * whether that raised the total, and the strategies it went through.
 */
export function HandlerRecipeNote({ strategies = [], adaptation }: HandlerRecipeNoteProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.handlerRecipe');

  if (strategies.length === 0 && !adaptation) return null;

  return (
    <div className="flex flex-col gap-y-1 text-paragraph-sm text-foreground-muted">
      {adaptation ? (
        <ul className="flex flex-col gap-y-0.5">
          {adaptation.lines.map((line) => (
            <li key={line.item_id} className="text-foreground">
              {t('raised', {
                item: line.description,
                from: fmt.value(Number(line.from_quantity)),
                to: fmt.value(Number(line.to_quantity)),
                promotion: line.promotion_name,
              })}
            </li>
          ))}
        </ul>
      ) : null}
      {adaptation?.total_raised ? (
        <p className="text-warning-foreground">
          {t('totalRaised', {
            before: fmt.currency(adaptation.total_before),
            after: fmt.currency(adaptation.total_after),
          })}
        </p>
      ) : null}
      {strategies.length > 0 ? (
        <p>
          {t('strategies', {
            list: fmt.list(strategies.map((strategy) => t(`strategy.${strategy}`))),
          })}
        </p>
      ) : null}
    </div>
  );
}
