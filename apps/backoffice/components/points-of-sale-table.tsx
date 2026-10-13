'use client';

import { useTranslations } from 'next-intl';
import type { Control, FieldValues, Path } from 'react-hook-form';

import {
  Badge,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { QuantityInput } from '@/components/quantity-input';
import { POINT_OF_SALE_MIN, type InvoicingBranch } from '@/lib/api/invoicing';
import { POINT_OF_SALE_MAX_LENGTH } from '@/lib/forms/points-of-sale';

interface PointsOfSaleTableProps<T extends FieldValues> {
  // A form whose `branches` holds one PointOfSaleValue per branch, in the order of `branches`.
  control: Control<T>;
  branches: readonly InvoicingBranch[];
  disabled?: boolean;
}

// Each branch's point of sale, edited in place. Facturación and the ARCA guide both render it.
export function PointsOfSaleTable<T extends FieldValues>({
  control,
  branches,
  disabled = false,
}: PointsOfSaleTableProps<T>) {
  const t = useTranslations('invoicing.settings.pointsOfSale');

  return (
    <Table>
      <TableCaption className="sr-only">{t('caption')}</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead kind="text">{t('branch')}</TableHead>
          <TableHead kind="quantityInput">{t('number')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {branches.map((branch, index) => (
          <TableRow key={branch.branchId}>
            <TableCell className="text-paragraph-sm-medium text-foreground">
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                {branch.name}
                {branch.isActive ? null : (
                  <Badge tone="neutral" size="sm">
                    {t('closed')}
                  </Badge>
                )}
              </span>
            </TableCell>
            <TableCell kind="quantityInput">
              <FormField
                control={control}
                name={`branches.${index}.pointOfSale` as Path<T>}
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <QuantityInput
                        {...field}
                        min={POINT_OF_SALE_MIN}
                        maxLength={POINT_OF_SALE_MAX_LENGTH}
                        placeholder={t('placeholder')}
                        disabled={disabled}
                        aria-label={t('inputLabel', { branch: branch.name })}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
