'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowUpRightIcon, SearchXIcon, UsersIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Card,
  CardHeader,
  CardTitle,
  InlineLink,
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
import { clientDisplayName, type ClientSummary } from '@/lib/api/client-profiles';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { useFormatters } from '@/lib/i18n/formatters';

interface ClientTableProps {
  clients: ClientSummary[];
}

export function ClientTable({ clients }: ClientTableProps) {
  const t = useTranslations('clients');
  const tCommon = useTranslations('common');
  const fmt = useFormatters();
  const [search, setSearch] = useState('');
  const filtered = useMemo(() => {
    const query = search.trim().toLocaleLowerCase('es-AR');
    if (!query) return clients;
    return clients.filter((client) =>
      [client.name, client.phone, client.email, ...client.tags.map((tag) => tag.name)]
        .filter(Boolean)
        .some((value) => value?.toLocaleLowerCase('es-AR').includes(query)),
    );
  }, [clients, search]);

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <CardHeader className="flex-row items-center justify-between py-6">
        <div className="flex items-center gap-x-3">
          <CardTitle className="text-heading-3">{t('title')}</CardTitle>
          <Badge tone="neutral">{t('total', { total: clients.length })}</Badge>
        </div>
      </CardHeader>

      <div className="border-y border-border px-6 py-5">
        <SearchInput
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch('')}
          clearLabel={tCommon('form.clearSearch')}
          placeholder={t('search')}
          maxLength={TEXT_FIELD_MAX_LENGTH}
          containerClassName="w-full"
        />
      </div>

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('table.client')}</TableHead>
            <TableHead>{t('table.contact')}</TableHead>
            <TableHead>{t('table.tags')}</TableHead>
            <TableHead>{t('table.sales')}</TableHead>
            <TableHead>{t('table.lastSale')}</TableHead>
            <TableHead className="text-right">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {filtered.length === 0 ? (
            <TableEmptyRow
              colSpan={6}
              icon={search ? SearchXIcon : UsersIcon}
              title={t(search ? 'noResults.title' : 'empty.title')}
              description={t(search ? 'noResults.description' : 'empty.description')}
            />
          ) : (
            filtered.map((client) => (
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
                <TableCell>{t('salesCount', { total: client.acceptedQuoteCount })}</TableCell>
                <TableCell className={client.lastAcceptedAt ? undefined : 'text-foreground-subtle'}>
                  {client.lastAcceptedAt ? fmt.date(client.lastAcceptedAt) : t('noSales')}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end">
                    <InlineLink asChild tone="muted">
                      <Link href={`/clients/${client.id}`}>
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
    </Card>
  );
}
