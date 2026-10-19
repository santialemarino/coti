'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowUpRightIcon, SearchXIcon, TagIcon, UsersIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Card,
  CardHeader,
  CardTitle,
  InlineLink,
  MultiCombobox,
  Pagination,
  SearchInput,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { ROUTES } from '@/config/routes';
import { clientDisplayName, type ClientSummary } from '@/lib/api/client-profiles';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { useFormatters } from '@/lib/i18n/formatters';

const PAGE_SIZE = 20;

interface ClientTableProps {
  clients: ClientSummary[];
}

export function ClientTable({ clients }: ClientTableProps) {
  const t = useTranslations('clients');
  const tCommon = useTranslations('common');
  const fmt = useFormatters();
  const [search, setSearch] = useState('');
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([]);
  const [page, setPage] = useState(1);

  // Any criterion change starts over at the first page, or a stale page number would point nowhere.
  useEffect(() => {
    setPage(1);
  }, [search, selectedTagIds]);

  // The tag filter offers only tags actually in use, so every option can still lead to a row.
  const tagOptions = useMemo(() => {
    const byId = new Map<string, string>();
    clients.forEach((client) => client.tags.forEach((tag) => byId.set(tag.id, tag.name)));
    return [...byId.entries()]
      .map(([value, label]) => ({ value, label }))
      .sort((a, b) => a.label.localeCompare(b.label, 'es-AR'));
  }, [clients]);

  const filtered = useMemo(() => {
    const query = search.trim().toLocaleLowerCase('es-AR');
    return clients.filter((client) => {
      // A tag filter is an OR: a client matches when it carries any of the selected tags.
      const matchesTags =
        selectedTagIds.length === 0 || client.tags.some((tag) => selectedTagIds.includes(tag.id));
      if (!matchesTags) return false;
      if (!query) return true;
      return [client.name, client.phone, client.email, ...client.tags.map((tag) => tag.name)]
        .filter(Boolean)
        .some((value) => value?.toLocaleLowerCase('es-AR').includes(query));
    });
  }, [clients, search, selectedTagIds]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE);
  const hasFilters = search.trim() !== '' || selectedTagIds.length > 0;

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <CardHeader className="flex-row items-center justify-between py-6">
        <div className="flex items-center gap-x-3">
          <CardTitle className="text-heading-3">{t('table.title')}</CardTitle>
          <Badge tone="neutral">{t('total', { total: clients.length })}</Badge>
        </div>
      </CardHeader>

      <div className="flex flex-col gap-3 border-y border-border px-6 py-5 sm:flex-row sm:items-center">
        <SearchInput
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch('')}
          clearLabel={tCommon('form.clearSearch')}
          placeholder={t('search')}
          maxLength={TEXT_FIELD_MAX_LENGTH}
          containerClassName="w-full"
        />
        <MultiCombobox
          options={tagOptions}
          values={selectedTagIds}
          onValuesChange={setSelectedTagIds}
          placeholder={t('filters.tags')}
          summaryLabel={(count) => t('filters.tagCount', { count })}
          clearLabel={t('filters.clearTags')}
          icon={<TagIcon aria-hidden="true" className="size-4" />}
          aria-label={t('filters.tags')}
          className="w-full sm:w-56"
        />
      </div>

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead kind="text">{t('table.client')}</TableHead>
            <TableHead kind="text">{t('table.contact')}</TableHead>
            <TableHead kind="text">{t('table.tags')}</TableHead>
            <TableHead kind="count">{t('table.sales')}</TableHead>
            <TableHead kind="date">{t('table.lastSale')}</TableHead>
            <TableHead kind="actionsWide">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pageItems.length === 0 ? (
            <TableEmptyRow
              colSpan={6}
              icon={hasFilters ? SearchXIcon : UsersIcon}
              title={t(hasFilters ? 'noResults.title' : 'empty.title')}
              description={t(hasFilters ? 'noResults.description' : 'empty.description')}
            />
          ) : (
            pageItems.map((client) => (
              <TableRow key={client.id}>
                <TableCell>
                  <div className="flex flex-col gap-y-0.5">
                    <span className="text-paragraph-sm-medium text-foreground">
                      {clientDisplayName(client, t('unnamed'))}
                    </span>
                    {client.name && (client.email || client.phone) ? (
                      <span className="text-paragraph-xs text-foreground-subtle">
                        {client.email ?? client.phone}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className={client.phone ? undefined : 'text-foreground-subtle'}>
                  {client.phone ?? t('noPhone')}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1.5">
                    {client.tags.length ? (
                      client.tags.map((tag) => (
                        <Badge key={tag.id} tone="outline" size="sm">
                          {tag.name}
                        </Badge>
                      ))
                    ) : (
                      <span className="text-paragraph-sm text-foreground-subtle">
                        {t('tags.none')}
                      </span>
                    )}
                  </div>
                </TableCell>
                <TableCell kind="count">
                  {t('salesCount', { total: client.acceptedQuoteCount })}
                </TableCell>
                <TableCell
                  kind="date"
                  className={cn(!client.lastAcceptedAt && 'text-foreground-subtle')}
                >
                  {client.lastAcceptedAt ? fmt.dateNumeric(client.lastAcceptedAt) : t('noSales')}
                </TableCell>
                <TableCell kind="actionsWide">
                  <div className="flex justify-center">
                    <InlineLink asChild tone="muted">
                      <Link href={ROUTES.clientDetail(client.id)}>
                        {t('viewProfile')}
                        <ArrowUpRightIcon aria-hidden="true" />
                      </Link>
                    </InlineLink>
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      {filtered.length > 0 ? (
        <div className="flex flex-col items-center justify-between gap-y-3 border-t border-border px-6 py-4 sm:flex-row">
          <p className="text-paragraph-xs text-foreground-muted">
            {t('results', {
              from: (safePage - 1) * PAGE_SIZE + 1,
              to: Math.min(safePage * PAGE_SIZE, filtered.length),
              total: filtered.length,
            })}
          </p>
          <Pagination
            page={safePage}
            pageCount={pageCount}
            onPageChange={setPage}
            labels={{
              previous: tCommon('pagination.previous'),
              next: tCommon('pagination.next'),
              page: tCommon('pagination.label'),
            }}
          />
        </div>
      ) : null}
    </Card>
  );
}
