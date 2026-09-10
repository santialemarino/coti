'use client';

import { useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  PendingButton,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import type {
  CreateDiscountBody,
  DiscountActionType,
  DiscountScope,
  QuoteDiscountResponse,
  QuoteItemResponse,
} from '@/lib/api/rfqs';
import { decimalToMoneyInput, maskMoneyInput, moneyInputToDecimal } from '@/lib/forms/money-input';

interface DiscountDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // The discount being edited; null opens the add form.
  initial: QuoteDiscountResponse | null;
  // The version's lines, used to pick what an ITEM/ITEM_SET discount covers.
  items: QuoteItemResponse[];
  onSave: (body: CreateDiscountBody) => Promise<void>;
}

/*
 * Add or edit a seller-typed discount. Two knobs compose the rule the backend recomputes
 * the money amount from: how it acts (a flat amount or a percentage) and what it covers
 * (the version total, one line, or a picked set). The backend enforces the same shape again —
 * a required description, a value above zero, at most 100% — and refuses an amount over the
 * scope base, so the form gates on the fields and surfaces the backend's refusal as-is.
 */
export function DiscountDialog({
  open,
  onOpenChange,
  initial,
  items,
  onSave,
}: DiscountDialogProps) {
  const t = useTranslations('rfqs.detail.items.discounts');
  const isEditing = initial !== null;
  const [name, setName] = useState('');
  const [actionType, setActionType] = useState<DiscountActionType>('FIXED_AMOUNT');
  const [value, setValue] = useState('');
  const [scope, setScope] = useState<DiscountScope>('TOTAL');
  const [linkedItemIds, setLinkedItemIds] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      const stored = initial?.action_value ?? initial?.amount ?? '';
      setName(initial?.description ?? initial?.promotion_name ?? '');
      setActionType(initial?.action_type ?? 'FIXED_AMOUNT');
      setValue(
        stored && (initial?.action_type ?? 'FIXED_AMOUNT') === 'FIXED_AMOUNT'
          ? decimalToMoneyInput(stored)
          : stored,
      );
      setScope(initial?.scope ?? 'TOTAL');
      setLinkedItemIds(initial?.item_ids ?? []);
    }
  }, [open, initial]);

  const coversItems = scope !== 'TOTAL';
  // An amount is money and carries the Argentine grouping; a percentage is a plain rate.
  const isAmount = actionType === 'FIXED_AMOUNT';
  const parsedValue = Number.parseFloat(isAmount ? moneyInputToDecimal(value) : value);
  const validValue =
    Number.isFinite(parsedValue) && parsedValue > 0 && (isAmount || parsedValue <= 100);
  const hasLinkedItems = linkedItemIds.length > 0;
  const canSave = name.trim().length > 0 && validValue && (!coversItems || hasLinkedItems);

  function toggleItem(itemId: string) {
    setLinkedItemIds((prev) =>
      prev.includes(itemId) ? prev.filter((id) => id !== itemId) : [...prev, itemId],
    );
  }

  async function handleSave() {
    if (!canSave) return;
    setSaving(true);
    try {
      const body: CreateDiscountBody = {
        description: name.trim(),
        action_type: actionType,
        value: String(parsedValue),
        scope,
      };
      if (coversItems) {
        body.item_ids = linkedItemIds;
      }
      await onSave(body);
    } finally {
      setSaving(false);
    }
  }

  function handleFormKey(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter' && canSave) {
      void handleSave();
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!saving}>
        <DialogHeader>
          <DialogTitle>{isEditing ? t('editTitle') : t('addTitle')}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-y-4">
          <div className="flex flex-col gap-y-1.5">
            <Label htmlFor="discount-name" required>
              {t('nameLabel')}
            </Label>
            <Input
              id="discount-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={handleFormKey}
              placeholder={t('namePlaceholder')}
              autoFocus
            />
          </div>

          <div className="flex flex-col gap-y-1.5">
            <Label required>{t('typeLabel')}</Label>
            <ToggleGroup
              type="single"
              variant="segmented"
              size="sm"
              value={actionType}
              onValueChange={(next) => {
                if (!next) return;
                setActionType(next as DiscountActionType);
                // The two read the same digits differently, so a switch clears rather than
                // reinterpreting "10" as ten pesos when it was ten percent.
                setValue('');
              }}
              className="w-full"
            >
              <ToggleGroupItem value="FIXED_AMOUNT" className="flex-1">
                {t('fixedAmount')}
              </ToggleGroupItem>
              <ToggleGroupItem value="PERCENTAGE" className="flex-1">
                {t('percentage')}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>

          <div className="flex flex-col gap-y-1.5">
            <Label htmlFor="discount-value" required>
              {t(actionType === 'PERCENTAGE' ? 'valueLabel' : 'amountLabel')}
            </Label>
            {isAmount ? (
              <Input
                id="discount-value"
                type="text"
                inputMode="decimal"
                prefix="$"
                value={value}
                onChange={(event) => setValue(maskMoneyInput(event.target.value))}
                onKeyDown={handleFormKey}
                placeholder="0,00"
              />
            ) : (
              <Input
                id="discount-value"
                type="number"
                inputMode="decimal"
                min={0.01}
                step={0.01}
                max={100}
                suffix="%"
                value={value}
                onChange={(event) => setValue(event.target.value)}
                onKeyDown={handleFormKey}
                placeholder="0"
              />
            )}
          </div>

          <div className="flex flex-col gap-y-1.5">
            <Label required>{t('scopeLabel')}</Label>
            <ToggleGroup
              type="single"
              variant="segmented"
              size="sm"
              value={scope}
              onValueChange={(next) => {
                if (next) setScope(next as DiscountScope);
              }}
              className="w-full"
            >
              <ToggleGroupItem value="TOTAL" className="flex-1">
                {t('scope.TOTAL')}
              </ToggleGroupItem>
              <ToggleGroupItem value="ITEM" className="flex-1">
                {t('scope.ITEM')}
              </ToggleGroupItem>
              <ToggleGroupItem value="ITEM_SET" className="flex-1">
                {t('scope.ITEM_SET')}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>

          {coversItems && (
            <div className="flex flex-col gap-y-1.5">
              <Label required>{t('itemsLabel')}</Label>
              <div className="flex max-h-44 flex-col gap-y-1 overflow-y-auto rounded-lg border border-border p-2">
                {items.map((item) => {
                  const checked = linkedItemIds.includes(item.id);
                  return (
                    <label
                      key={item.id}
                      className="flex cursor-pointer items-center gap-x-2 rounded-md px-2 py-1.5 text-paragraph-sm transition-colors hover:bg-sunken"
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={() => toggleItem(item.id)}
                        aria-label={item.requested_description}
                      />
                      <span className="min-w-0 flex-1 truncate text-foreground">
                        {item.requested_description}
                      </span>
                      <span className="shrink-0 tabular-nums text-foreground-muted">
                        {item.quantity} {item.unit ?? ''}
                      </span>
                    </label>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            onClick={() => onOpenChange(false)}
          >
            {t('cancel')}
          </Button>
          <PendingButton
            type="button"
            pending={saving}
            pendingLabel={t('saving')}
            disabled={!canSave}
            onClick={handleSave}
          >
            {isEditing ? t('save') : t('add')}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
