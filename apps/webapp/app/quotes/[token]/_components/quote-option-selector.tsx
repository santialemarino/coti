'use client';

import { useTranslations } from 'next-intl';

import { Badge, Card, Label, RadioGroup, RadioGroupItem } from '@repo/ui/components';
import type { QuoteItem } from '@/lib/api/public-quotes';
import { useQuoteSelections } from './quote-selection-context';

interface QuoteOptionSelectorProps {
  items: QuoteItem[];
  currency: string;
  disabled?: boolean;
}

export function QuoteOptionSelector({
  items,
  currency,
  disabled = false,
}: QuoteOptionSelectorProps) {
  const t = useTranslations('quote.options');
  const { select, selectedAlternativeId } = useQuoteSelections();
  const itemsWithOptions = items.filter((item) =>
    item.alternatives.some((alternative) => alternative.selectable),
  );

  if (itemsWithOptions.length === 0) return null;

  return (
    <section className="flex flex-col gap-y-4" aria-labelledby="quote-options-heading">
      <div className="flex flex-col gap-y-1">
        <h2 id="quote-options-heading" className="text-heading-5 text-foreground">
          {t('heading')}
        </h2>
        <p className="text-paragraph text-foreground-muted">{t('intro')}</p>
      </div>
      {itemsWithOptions.map((item) => {
        const selected = selectedAlternativeId(item.id);
        return (
          <Card key={item.id} className="gap-y-3">
            <div className="flex flex-col gap-y-0.5">
              <h3 className="text-heading-6 text-foreground">{item.productName}</h3>
              <p className="text-paragraph-sm text-foreground-muted">
                {t('forRequest', { request: item.requestedDescription })}
              </p>
            </div>
            <RadioGroup
              value={selected ?? `base-${item.id}`}
              onValueChange={(value) =>
                select(item.id, value === `base-${item.id}` ? undefined : value)
              }
              disabled={disabled}
              aria-label={t('groupLabel', { product: item.productName })}
            >
              <Option
                id={`base-${item.id}`}
                label={t('base')}
                name={item.productName}
                unit={item.unit}
                unitPrice={item.unitPrice}
                currency={currency}
              />
              {item.alternatives
                .filter((alternative) => alternative.selectable)
                .map((alternative) => (
                  <Option
                    key={alternative.id}
                    id={alternative.id}
                    label={alternativeLabel(t, alternative.recommendationType)}
                    name={alternative.name}
                    unit={alternative.unit}
                    unitPrice={alternative.unitPrice}
                    currency={currency}
                  />
                ))}
            </RadioGroup>
          </Card>
        );
      })}
    </section>
  );
}

function Option({
  id,
  label,
  name,
  unit,
  unitPrice,
  currency,
}: {
  id: string;
  label: string;
  name: string;
  unit?: string;
  unitPrice: string;
  currency: string;
}) {
  const t = useTranslations('quote.options');
  const amount = new Intl.NumberFormat('es-AR', { style: 'currency', currency }).format(
    Number(unitPrice),
  );
  return (
    <Label
      htmlFor={id}
      className="flex cursor-pointer items-center justify-between gap-x-3 rounded-md border border-border px-3 py-2.5 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary-subtle"
    >
      <span className="flex min-w-0 items-center gap-x-2">
        <RadioGroupItem id={id} value={id} />
        <span className="flex min-w-0 flex-col gap-y-0.5">
          <span className="flex items-center gap-x-2">
            <Badge tone="neutral" size="sm">
              {label}
            </Badge>
            <span className="truncate text-paragraph-sm-medium text-foreground">{name}</span>
          </span>
          {unit ? (
            <span className="text-paragraph-xs text-foreground-muted">
              {t('perUnit', { unit })}
            </span>
          ) : null}
        </span>
      </span>
      <span className="shrink-0 text-paragraph-sm-medium text-foreground tabular-nums">
        {amount}
      </span>
    </Label>
  );
}

function alternativeLabel(
  t: ReturnType<typeof useTranslations>,
  type: QuoteItem['alternatives'][number]['recommendationType'],
): string {
  if (type === 'ECONOMY') return t('economy');
  if (type === 'PREMIUM') return t('premium');
  return t('alternative');
}
