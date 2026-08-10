'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ComponentProps } from 'react';
import { useRouter } from 'next/navigation';
import {
  ArchiveIcon,
  AudioLinesIcon,
  EyeIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  ImageIcon,
  InboxIcon,
  LinkIcon,
  MailIcon,
  MessageCircleIcon,
  MoreHorizontalIcon,
  PackageSearchIcon,
  PencilIcon,
  PlusIcon,
  RefreshCcwIcon,
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
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
  Pagination,
  SearchInput,
  Skeleton,
  SortableTableHead,
  StatusScreen,
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
import { RfqStatusBadge, STATUS_ORDER } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { ROUTES } from '@/config/routes';
import {
  fetchRfqs,
  type RfqChannel,
  type RfqPriority,
  type RfqRecord,
  type RfqStatus,
} from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

const PAGE_SIZE = 10;
const COLUMN_COUNT = 11;

const CHANNELS: readonly RfqChannel[] = [
  'whatsapp',
  'email',
  'audio',
  'photo',
  'pdf',
  'excel',
  'link',
];

const PRIORITIES: readonly RfqPriority[] = ['high', 'normal', 'low'];

const PRIORITY_RANK: Record<RfqPriority, number> = { high: 0, normal: 1, low: 2 };

const STATUS_RANK = Object.fromEntries(
  STATUS_ORDER.map((status, index) => [status, index]),
) as Record<RfqStatus, number>;

const PRIORITY_TONE: Record<RfqPriority, ComponentProps<typeof Badge>['tone']> = {
  high: 'danger',
  normal: 'neutral',
  low: 'outline',
};

const CHANNEL_ICON: Record<RfqChannel, typeof MailIcon> = {
  whatsapp: MessageCircleIcon,
  email: MailIcon,
  audio: AudioLinesIcon,
  photo: ImageIcon,
  pdf: FileTextIcon,
  excel: FileSpreadsheetIcon,
  link: LinkIcon,
};

type SortKey =
  | 'id'
  | 'client'
  | 'createdAt'
  | 'channel'
  | 'seller'
  | 'branch'
  | 'itemCount'
  | 'total'
  | 'priority'
  | 'status';

type SortOrder = 'asc' | 'desc';

function unique(values: string[]): string[] {
  return [...new Set(values)].sort((a, b) => a.localeCompare(b, 'es'));
}

function compareRfqs(a: RfqRecord, b: RfqRecord, key: SortKey, order: SortOrder): number {
  const direction = order === 'asc' ? 1 : -1;
  let result: number;
  switch (key) {
    case 'itemCount':
      result = a.itemCount - b.itemCount;
      break;
    case 'total':
      result = Number(a.total) - Number(b.total);
      break;
    case 'priority':
      result = PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority];
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

function TableSkeleton({ rows }: { rows: number }) {
  return (
    <div className="flex flex-col border-t border-border">
      {Array.from({ length: rows }, (_, index) => (
        <div
          key={index}
          className={cn(
            'flex items-center gap-x-6 px-6 py-3.5',
            index > 0 && 'border-t border-border',
          )}
        >
          <Skeleton className="size-4 rounded-sm" />
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-4 w-16" />
          <Skeleton className="h-4 w-24" />
          <Skeleton className="ml-auto h-5 w-16 rounded-full" />
          <Skeleton className="h-5 w-20 rounded-full" />
          <Skeleton className="size-8 rounded-md" />
        </div>
      ))}
    </div>
  );
}

function PriorityBadge({ priority }: { priority: RfqPriority }) {
  const t = useTranslations('rfqs');
  return <Badge tone={PRIORITY_TONE[priority]}>{t(`priority.${priority}`)}</Badge>;
}

interface RowMenuProps {
  rfq: RfqRecord;
  onChangeStatus: (rfq: RfqRecord, status: RfqStatus) => void;
  onArchive: (rfq: RfqRecord) => void;
}

// The per-row contextual menu: view, edit, move through the statuses and archive.
function RowMenu({ rfq, onChangeStatus, onArchive }: RowMenuProps) {
  const t = useTranslations('rfqs');
  const router = useRouter();

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
        <DropdownMenuItem onSelect={() => toast.info(t('list.toast.comingSoon'))}>
          <PencilIcon aria-hidden="true" />
          {t('list.actions.edit')}
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <RefreshCcwIcon aria-hidden="true" />
            {t('list.actions.changeStatus')}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {STATUS_ORDER.map((status) => (
              <DropdownMenuItem key={status} onSelect={() => onChangeStatus(rfq, status)}>
                {t(`status.${status}`)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem tone="danger" onSelect={() => onArchive(rfq)}>
          <ArchiveIcon aria-hidden="true" />
          {t('list.actions.archive')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function RfqDashboard() {
  const t = useTranslations('rfqs');
  const tCommon = useTranslations('common');
  const fmt = useFormatters();
  const router = useRouter();

  const [records, setRecords] = useState<RfqRecord[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<RfqStatus | 'all'>('all');
  const [channelFilter, setChannelFilter] = useState<RfqChannel | 'all'>('all');
  const [branchFilter, setBranchFilter] = useState<string | 'all'>('all');
  const [sellerFilter, setSellerFilter] = useState<string | 'all'>('all');
  const [priorityFilter, setPriorityFilter] = useState<RfqPriority | 'all'>('all');
  const [sortBy, setSortBy] = useState<SortKey>('createdAt');
  const [sortOrder, setSortOrder] = useState<SortOrder>('desc');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [createOpen, setCreateOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await fetchRfqs();
      setRecords(data);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Any criterion change starts over at the first page, or a stale page number would point nowhere.
  useEffect(() => {
    setPage(1);
  }, [
    query,
    statusFilter,
    channelFilter,
    branchFilter,
    sellerFilter,
    priorityFilter,
    sortBy,
    sortOrder,
  ]);

  const statusCounts = useMemo(() => {
    const counts = new Map<RfqStatus, number>();
    records?.forEach((rfq) => counts.set(rfq.status, (counts.get(rfq.status) ?? 0) + 1));
    return counts;
  }, [records]);

  const branches = useMemo(() => unique(records?.map((rfq) => rfq.branch) ?? []), [records]);
  const sellers = useMemo(() => unique(records?.map((rfq) => rfq.seller) ?? []), [records]);

  const filtered = useMemo(() => {
    if (!records) return [];
    const needle = query.trim().toLowerCase();
    return records
      .filter((rfq) => {
        if (statusFilter !== 'all' && rfq.status !== statusFilter) return false;
        if (channelFilter !== 'all' && rfq.channel !== channelFilter) return false;
        if (branchFilter !== 'all' && rfq.branch !== branchFilter) return false;
        if (sellerFilter !== 'all' && rfq.seller !== sellerFilter) return false;
        if (priorityFilter !== 'all' && rfq.priority !== priorityFilter) return false;
        if (needle) {
          const haystack = `${rfq.id} ${rfq.client} ${rfq.seller} ${rfq.branch}`.toLowerCase();
          if (!haystack.includes(needle)) return false;
        }
        return true;
      })
      .sort((a, b) => compareRfqs(a, b, sortBy, sortOrder));
  }, [
    records,
    query,
    statusFilter,
    channelFilter,
    branchFilter,
    sellerFilter,
    priorityFilter,
    sortBy,
    sortOrder,
  ]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE);

  const activeFilterCount =
    (channelFilter !== 'all' ? 1 : 0) +
    (branchFilter !== 'all' ? 1 : 0) +
    (sellerFilter !== 'all' ? 1 : 0) +
    (priorityFilter !== 'all' ? 1 : 0);
  const hasCriteria = query.trim() !== '' || activeFilterCount > 0 || statusFilter !== 'all';

  function clearFilters() {
    setQuery('');
    setChannelFilter('all');
    setBranchFilter('all');
    setSellerFilter('all');
    setPriorityFilter('all');
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

  function updateStatus(rfq: RfqRecord, status: RfqStatus) {
    setRecords((previous) =>
      previous
        ? previous.map((item) => (item.id === rfq.id ? { ...item, status } : item))
        : previous,
    );
    toast.success(t('list.toast.statusChanged', { id: rfq.id, status: t(`status.${status}`) }));
  }

  function archiveOne(rfq: RfqRecord) {
    setRecords((previous) => (previous ? previous.filter((item) => item.id !== rfq.id) : previous));
    setSelected((previous) => {
      const next = new Set(previous);
      next.delete(rfq.id);
      return next;
    });
    toast.success(t('list.toast.archived', { id: rfq.id }));
  }

  function archiveSelected() {
    const ids = [...selected];
    if (ids.length === 0) return;
    setRecords((previous) =>
      previous ? previous.filter((item) => !selected.has(item.id)) : previous,
    );
    setSelected(new Set());
    toast.success(t('list.toast.archivedMany', { count: ids.length }));
  }

  const pageAllSelected = pageItems.length > 0 && pageItems.every((rfq) => selected.has(rfq.id));
  const pageSomeSelected = pageItems.some((rfq) => selected.has(rfq.id)) && !pageAllSelected;

  if (error) {
    return (
      <Card className="gap-y-0 overflow-hidden py-0">
        <StatusScreen
          icon={PackageSearchIcon}
          tone="danger"
          title={t('list.error.title')}
          description={t('list.error.description')}
        >
          <Button onClick={load}>
            <RefreshCcwIcon aria-hidden="true" />
            {t('list.error.retry')}
          </Button>
        </StatusScreen>
      </Card>
    );
  }

  return (
    <>
      <Card className="gap-y-0 overflow-hidden py-0">
        <CardHeader className="flex-row items-center justify-between py-6">
          <CardTitle className="text-heading-4">{t('list.title')}</CardTitle>
          <div className="flex items-center gap-x-3">
            {records ? (
              <Badge tone="neutral">{t('list.resultsTotal', { total: records.length })}</Badge>
            ) : null}
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
                ...sellers.map((seller) => ({ value: seller, label: seller })),
              ]}
              value={sellerFilter}
              onValueChange={(value) => setSellerFilter(value === 'all' ? 'all' : value)}
              placeholder={t('list.filters.seller')}
              aria-label={t('list.filters.seller')}
              className="min-w-36 flex-1"
            />
            <Combobox
              options={[
                { value: 'all', label: t('list.filters.allPriority') },
                ...PRIORITIES.map((priority) => ({
                  value: priority,
                  label: t(`priority.${priority}`),
                })),
              ]}
              value={priorityFilter}
              onValueChange={(value) => setPriorityFilter(value as RfqPriority | 'all')}
              placeholder={t('list.filters.priority')}
              aria-label={t('list.filters.priority')}
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

        {loading ? (
          <TableSkeleton rows={8} />
        ) : (
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
                  column="id"
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
                  label={t('list.columns.priority')}
                  column="priority"
                  sortBy={sortBy}
                  sortOrder={sortOrder}
                  onSort={handleSort}
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
                  description={t(
                    hasCriteria ? 'list.noResults.description' : 'list.empty.description',
                  )}
                />
              ) : (
                pageItems.map((rfq) => {
                  const ChannelIcon = CHANNEL_ICON[rfq.channel];
                  return (
                    <TableRow
                      key={rfq.id}
                      data-state={selected.has(rfq.id) ? 'selected' : undefined}
                    >
                      <TableCell>
                        <Checkbox
                          checked={selected.has(rfq.id)}
                          onCheckedChange={() => toggleSelected(rfq.id)}
                          aria-label={t('list.selectRow', { id: rfq.id })}
                        />
                      </TableCell>
                      <TableCell>
                        <button
                          type="button"
                          onClick={() => router.push(ROUTES.rfqsDetail(rfq.id))}
                          className="group/order w-full text-left outline-none"
                        >
                          <span className="block truncate text-paragraph-sm-medium text-foreground transition-colors duration-150 ease-out-soft group-focus-visible/order:text-primary group-hover/order:text-primary">
                            #{rfq.id}
                          </span>
                          <span className="block truncate text-paragraph-mini text-foreground-muted">
                            {rfq.client}
                          </span>
                        </button>
                      </TableCell>
                      <TableCell className="whitespace-nowrap">{fmt.date(rfq.createdAt)}</TableCell>
                      <TableCell>
                        <span className="inline-flex items-center gap-x-2 whitespace-nowrap text-foreground-subtle">
                          <ChannelIcon aria-hidden="true" className="size-4" />
                          {t(`channels.${rfq.channel}`)}
                        </span>
                      </TableCell>
                      <TableCell className="whitespace-nowrap">{rfq.seller}</TableCell>
                      <TableCell className="whitespace-nowrap">{rfq.branch}</TableCell>
                      <TableCell>{t('list.items', { count: rfq.itemCount })}</TableCell>
                      <TableCell className="whitespace-nowrap text-right tabular-nums">
                        {fmt.currency(rfq.total)}
                      </TableCell>
                      <TableCell>
                        <PriorityBadge priority={rfq.priority} />
                      </TableCell>
                      <TableCell>
                        <RfqStatusBadge status={rfq.status} />
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end">
                          <RowMenu rfq={rfq} onChangeStatus={updateStatus} onArchive={archiveOne} />
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        )}

        {!loading && filtered.length > 0 ? (
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
      <CreateRfqDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}
