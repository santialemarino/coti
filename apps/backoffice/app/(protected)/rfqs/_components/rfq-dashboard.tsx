'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  ArchiveIcon,
  ClipboardListIcon,
  EyeIcon,
  InboxIcon,
  LinkIcon,
  MailIcon,
  MessageCircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  SearchXIcon,
  XIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardHeader,
  CardTitle,
  Checkbox,
  Combobox,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Pagination,
  SearchInput,
  SortableTableHead,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { CreateRfqDialog } from '@/app/(protected)/rfqs/_components/create-rfq-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import {
  hasQuoteTotal,
  RfqStatusBadge,
  STATUS_ORDER,
} from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { ROUTES } from '@/config/routes';
import {
  formatRfqReference,
  type RfqChannel,
  type RfqRecord,
  type RfqStatus,
} from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

const PAGE_SIZE = 10;
const COLUMN_COUNT = 10;
const UNASSIGNED_SELLER = '__unassigned__';

const CHANNELS: readonly RfqChannel[] = ['whatsapp', 'email', 'webapp', 'manual_entry'];

const STATUS_RANK = Object.fromEntries(
  STATUS_ORDER.map((status, index) => [status, index]),
) as Record<RfqStatus, number>;

const CHANNEL_ICON: Record<RfqChannel, typeof MailIcon> = {
  whatsapp: MessageCircleIcon,
  email: MailIcon,
  webapp: LinkIcon,
  manual_entry: ClipboardListIcon,
};

type SortKey =
  | 'quoteNumber'
  | 'client'
  | 'createdAt'
  | 'channel'
  | 'seller'
  | 'branch'
  | 'itemCount'
  | 'total'
  | 'status';

type SortOrder = 'asc' | 'desc';

function unique(values: string[]): string[] {
  return [...new Set(values)].sort((a, b) => a.localeCompare(b, 'es'));
}

function compareRfqs(a: RfqRecord, b: RfqRecord, key: SortKey, order: SortOrder): number {
  const direction = order === 'asc' ? 1 : -1;
  let result: number;
  switch (key) {
    case 'quoteNumber':
      result = (a.quoteNumber ?? 0) - (b.quoteNumber ?? 0);
      break;
    case 'itemCount':
      result = a.itemCount - b.itemCount;
      break;
    case 'total':
      // No amount yet (no quote) sorts as the smallest number, not the largest.
      result = Number(a.total ?? 0) - Number(b.total ?? 0);
      break;
    case 'status':
      result = STATUS_RANK[a.status] - STATUS_RANK[b.status];
      break;
    case 'createdAt':
      result = a.createdAt.localeCompare(b.createdAt);
      break;
    default:
      result = a[key].localeCompare(b[key], 'es', { sensitivity: 'base' });
  }
  return result * direction;
}

interface RowMenuProps {
  rfq: RfqRecord;
  onArchive: (rfq: RfqRecord) => void;
}

function RowMenu({ rfq, onArchive }: RowMenuProps) {
  const router = useRouter();
  const t = useTranslations('rfqs');

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t('list.actions.more')}>
          <MoreHorizontalIcon aria-hidden="true" className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => router.push(ROUTES.rfqsDetail(rfq.id))}>
          <EyeIcon aria-hidden="true" />
          {t('list.actions.view')}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem tone="danger" onSelect={() => onArchive(rfq)}>
          <ArchiveIcon aria-hidden="true" />
          {t('list.actions.archive')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function RfqDashboard({
  initialRecords,
  activeBranchId,
  activeRfqId,
}: {
  initialRecords: RfqRecord[];
  activeBranchId: string | null;
  activeRfqId?: string | null;
}) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('rfqs');
  const tCommon = useTranslations('common');
  const { userName } = useRfqList();

  const [records, setRecords] = useState<RfqRecord[]>(initialRecords);

  /* Sync local state when the server re-fetches (e.g. after a new RFQ is created). */
  useEffect(() => {
    setRecords(initialRecords);
  }, [initialRecords]);

  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<RfqStatus | 'all'>('all');
  const [channelFilter, setChannelFilter] = useState<RfqChannel | 'all'>('all');
  const [branchFilter, setBranchFilter] = useState<string | 'all'>('all');
  const [sellerFilter, setSellerFilter] = useState<string | 'all'>('all');
  const [sortBy, setSortBy] = useState<SortKey>('createdAt');
  const [sortOrder, setSortOrder] = useState<SortOrder>('desc');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [createOpen, setCreateOpen] = useState(false);

  // Any criterion change starts over at the first page, or a stale page number would point nowhere.
  useEffect(() => {
    setPage(1);
  }, [query, statusFilter, channelFilter, branchFilter, sellerFilter, sortBy, sortOrder]);

  const branches = useMemo(() => unique(records.map((rfq) => rfq.branch)), [records]);
  const sellers = useMemo(() => unique(records.map((rfq) => rfq.seller)), [records]);

  /*
   * Everything except the status tab. The tab counts recount within the active criteria — a seller
   * filter narrows the tabs too — and clicking one then narrows that same set further.
   */
  const filteredBase = useMemo(() => {
    if (!records) return [];
    const needle = query.trim().toLowerCase();
    return records
      .filter((rfq) => {
        if (channelFilter !== 'all' && rfq.channel !== channelFilter) return false;
        if (branchFilter !== 'all' && rfq.branch !== branchFilter) return false;
        if (sellerFilter === UNASSIGNED_SELLER) {
          if (rfq.seller.trim() !== '') return false;
        } else if (sellerFilter !== 'all' && rfq.seller !== sellerFilter) {
          return false;
        }
        if (needle) {
          const haystack =
            `${formatRfqReference(rfq.quoteNumber) ?? ''} ${rfq.client} ${rfq.seller} ${rfq.branch}`.toLowerCase();
          if (!haystack.includes(needle)) return false;
        }
        return true;
      })
      .sort((a, b) => {
        // The inbox contract is follow-ups first; keep that as the primary key whatever
        // other column the seller is sorting by.
        const byFollowup = Number(b.needsFollowup) - Number(a.needsFollowup);
        if (byFollowup !== 0) return byFollowup;
        return compareRfqs(a, b, sortBy, sortOrder);
      });
  }, [records, query, channelFilter, branchFilter, sellerFilter, sortBy, sortOrder]);

  const statusCounts = useMemo(() => {
    const counts = new Map<RfqStatus, number>();
    filteredBase.forEach((rfq) => counts.set(rfq.status, (counts.get(rfq.status) ?? 0) + 1));
    return counts;
  }, [filteredBase]);

  const filtered = useMemo(() => {
    if (statusFilter === 'all') return filteredBase;
    return filteredBase.filter((rfq) => rfq.status === statusFilter);
  }, [filteredBase, statusFilter]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE);

  const activeFilterCount =
    (channelFilter !== 'all' ? 1 : 0) +
    (branchFilter !== 'all' ? 1 : 0) +
    (sellerFilter !== 'all' ? 1 : 0);
  const hasCriteria = query.trim() !== '' || activeFilterCount > 0 || statusFilter !== 'all';

  function clearFilters() {
    setQuery('');
    setChannelFilter('all');
    setBranchFilter('all');
    setSellerFilter('all');
    setStatusFilter('all');
  }

  function handleSort(column: SortKey) {
    if (sortBy === column) {
      setSortOrder((previous) => (previous === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortBy(column);
      setSortOrder('asc');
    }
  }

  function toggleSelected(id: string) {
    setSelected((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function togglePageSelection() {
    setSelected((previous) => {
      const next = new Set(previous);
      const allSelected = pageItems.length > 0 && pageItems.every((rfq) => next.has(rfq.id));
      pageItems.forEach((rfq) => {
        if (allSelected) next.delete(rfq.id);
        else next.add(rfq.id);
      });
      return next;
    });
  }

  function archiveOne(rfq: RfqRecord) {
    // Archivado is a flag, not a state: the row stays and shows the grey pill over its real status.
    setRecords((previous) =>
      previous
        ? previous.map((item) => (item.id === rfq.id ? { ...item, archived: true } : item))
        : previous,
    );
    setSelected((previous) => {
      const next = new Set(previous);
      next.delete(rfq.id);
      return next;
    });
    toast.success(
      t('list.toast.archived', {
        id: formatRfqReference(rfq.quoteNumber) ?? t('list.numberPending'),
      }),
    );
  }

  function archiveSelected() {
    const ids = [...selected];
    if (ids.length === 0) return;
    setRecords((previous) =>
      previous
        ? previous.map((item) => (selected.has(item.id) ? { ...item, archived: true } : item))
        : previous,
    );
    setSelected(new Set());
    toast.success(t('list.toast.archivedMany', { count: ids.length }));
  }

  const pageAllSelected = pageItems.length > 0 && pageItems.every((rfq) => selected.has(rfq.id));
  const pageSomeSelected = pageItems.some((rfq) => selected.has(rfq.id)) && !pageAllSelected;

  return (
    <>
      <div className="pb-6">
        <h1 className="text-heading-2">{t('greeting', { name: userName })}</h1>
      </div>
      <Card className="gap-y-0 overflow-hidden py-0">
        <CardHeader className="flex-row items-center justify-between py-6">
          <CardTitle className="text-heading-3">{t('list.title')}</CardTitle>
          <div className="flex items-center gap-x-3">
            <Badge tone="neutral">{t('list.resultsTotal', { total: records.length })}</Badge>
            <Button onClick={() => setCreateOpen(true)}>
              <PlusIcon aria-hidden="true" />
              {t('list.create')}
            </Button>
          </div>
        </CardHeader>

        <div className="flex flex-col gap-y-4 border-y border-border px-6 py-6">
          <SearchInput
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onClear={() => setQuery('')}
            clearLabel={tCommon('form.clearSearch')}
            placeholder={t('list.search')}
            containerClassName="w-full"
          />
          <div className="flex flex-wrap items-center gap-x-4 gap-y-4">
            <Combobox
              options={[
                { value: 'all', label: t('list.filters.allChannel') },
                ...CHANNELS.map((channel) => ({
                  value: channel,
                  label: t(`channels.${channel}`),
                })),
              ]}
              value={channelFilter}
              onValueChange={(value) => setChannelFilter(value as RfqChannel | 'all')}
              placeholder={t('list.filters.channel')}
              aria-label={t('list.filters.channel')}
              className="min-w-36 flex-1"
            />
            <Combobox
              options={[
                { value: 'all', label: t('list.filters.allBranch') },
                ...branches.map((branch) => ({ value: branch, label: branch })),
              ]}
              value={branchFilter}
              onValueChange={(value) => setBranchFilter(value === 'all' ? 'all' : value)}
              placeholder={t('list.filters.branch')}
              aria-label={t('list.filters.branch')}
              className="min-w-36 flex-1"
            />
            <Combobox
              options={[
                { value: 'all', label: t('list.filters.allSeller') },
                { value: UNASSIGNED_SELLER, label: t('list.filters.unassigned') },
                ...sellers.map((seller) => ({ value: seller, label: seller })),
              ]}
              value={sellerFilter}
              onValueChange={(value) =>
                setSellerFilter(
                  value === 'all' ? 'all' : value === UNASSIGNED_SELLER ? UNASSIGNED_SELLER : value,
                )
              }
              placeholder={t('list.filters.seller')}
              aria-label={t('list.filters.seller')}
              className="min-w-36 flex-1"
            />
            {activeFilterCount > 0 ? (
              <Button variant="ghost" size="sm" onClick={clearFilters} className="flex-none">
                <XIcon aria-hidden="true" />
                {t('list.filters.clear')}
              </Button>
            ) : null}
          </div>

          <ToggleGroup
            type="single"
            value={statusFilter}
            onValueChange={(value) => value && setStatusFilter(value as RfqStatus | 'all')}
            variant="pills"
            size="sm"
            aria-label={t('list.tabs')}
            className="flex-wrap gap-y-2"
          >
            <ToggleGroupItem value="all">{t('list.all')}</ToggleGroupItem>
            {STATUS_ORDER.map((status) => (
              <ToggleGroupItem key={status} value={status}>
                {t(`status.${status}`)}
                <span className="ml-1 text-foreground-subtle">{statusCounts.get(status) ?? 0}</span>
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>

        {selected.size > 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b border-border bg-accent px-6 py-4">
            <p className="text-paragraph-sm-medium text-foreground">
              {t('list.selected', { count: selected.size })}
            </p>
            <div className="flex items-center gap-x-2">
              <Button variant="outline" size="sm" onClick={() => setSelected(new Set())}>
                {t('list.bulk.clear')}
              </Button>
              <Button variant="outline" size="sm" onClick={archiveSelected}>
                <ArchiveIcon aria-hidden="true" />
                {t('list.bulk.archive')}
              </Button>
            </div>
          </div>
        ) : null}

        <Table className="[&_th]:h-12 [&_th]:px-4 [&_th:has([role=checkbox])]:pr-0 [&_td]:px-4 [&_td]:py-3.5 [&_td:last-child]:border-l [&_td:last-child]:border-border [&_td:last-child]:pl-6">
          <TableCaption className="sr-only">{t('list.caption')}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">
                <Checkbox
                  checked={pageSomeSelected ? 'indeterminate' : pageAllSelected}
                  onCheckedChange={togglePageSelection}
                  aria-label={t('list.selectAll')}
                  disabled={pageItems.length === 0}
                />
              </TableHead>
              <SortableTableHead
                label={t('list.columns.id')}
                column="quoteNumber"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
                className="w-48"
              />
              <SortableTableHead
                label={t('list.columns.date')}
                column="createdAt"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableTableHead
                label={t('list.columns.channel')}
                column="channel"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableTableHead
                label={t('list.columns.seller')}
                column="seller"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableTableHead
                label={t('list.columns.branch')}
                column="branch"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableTableHead
                label={t('list.columns.items')}
                column="itemCount"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <SortableTableHead
                label={t('list.columns.total')}
                column="total"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
                className="text-right"
              />
              <SortableTableHead
                label={t('list.columns.status')}
                column="status"
                sortBy={sortBy}
                sortOrder={sortOrder}
                onSort={handleSort}
              />
              <TableHead className="border-l border-border pl-6 text-right">
                {t('list.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableEmptyRow
                colSpan={COLUMN_COUNT}
                icon={hasCriteria ? SearchXIcon : InboxIcon}
                title={t(hasCriteria ? 'list.noResults.title' : 'list.empty.title')}
                description={hasCriteria ? t('list.noResults.description') : undefined}
              />
            ) : (
              pageItems.map((rfq) => {
                const ChannelIcon = CHANNEL_ICON[rfq.channel];
                const reference = formatRfqReference(rfq.quoteNumber);
                return (
                  <TableRow
                    key={rfq.id}
                    role="link"
                    tabIndex={0}
                    aria-label={t('list.openRow', {
                      id: reference ?? t('list.numberPending'),
                    })}
                    onClick={(event) => {
                      if ((event.target as HTMLElement).closest('button, a, input')) return;
                      router.push(ROUTES.rfqsDetail(rfq.id));
                    }}
                    onKeyDown={(event) => {
                      if (event.target !== event.currentTarget || event.key !== 'Enter') return;
                      event.preventDefault();
                      router.push(ROUTES.rfqsDetail(rfq.id));
                    }}
                    data-state={selected.has(rfq.id) ? 'selected' : undefined}
                    className={cn(
                      'cursor-pointer outline-none active:bg-accent focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
                      rfq.id === activeRfqId && 'bg-accent',
                      rfq.needsFollowup && 'bg-warning-subtle',
                    )}
                  >
                    <TableCell>
                      <Checkbox
                        checked={selected.has(rfq.id)}
                        onCheckedChange={() => toggleSelected(rfq.id)}
                        aria-label={t('list.selectRow', {
                          id: reference ?? t('list.numberPending'),
                        })}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="w-full text-left">
                        <span className="block truncate text-paragraph-sm-medium text-foreground">
                          {reference ?? t('list.numberPending')}
                        </span>
                        <span className="block truncate text-paragraph-mini text-foreground-muted">
                          {rfq.client}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{fmt.date(rfq.createdAt)}</TableCell>
                    <TableCell>
                      <span className="inline-flex items-center gap-x-2 whitespace-nowrap text-foreground-subtle">
                        <ChannelIcon aria-hidden="true" className="size-4" />
                        {t(`channels.${rfq.channel}`)}
                      </span>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      {rfq.seller ? (
                        rfq.seller
                      ) : (
                        <span className="text-foreground-subtle">{t('list.unassigned')}</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{rfq.branch}</TableCell>
                    <TableCell>{t('list.items', { count: rfq.itemCount })}</TableCell>
                    <TableCell className="whitespace-nowrap text-right tabular-nums">
                      {hasQuoteTotal(rfq.status) && rfq.total != null ? (
                        fmt.currency(rfq.total)
                      ) : (
                        <span className="text-foreground-subtle" aria-hidden="true">
                          -
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      <RfqStatusBadge
                        status={rfq.status}
                        processing={rfq.processing}
                        archived={rfq.archived}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <RowMenu rfq={rfq} onArchive={archiveOne} />
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>

        {filtered.length > 0 ? (
          <div className="flex flex-col items-center justify-between gap-y-3 border-t border-border px-6 py-4 sm:flex-row">
            <p className="text-paragraph-xs text-foreground-muted">
              {t('list.results', {
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
      <CreateRfqDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => router.refresh()}
        activeBranchId={activeBranchId}
      />
    </>
  );
}
