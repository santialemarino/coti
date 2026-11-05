import { cn } from '@repo/ui/lib';

export type StatusTone = 'draft' | 'quoted' | 'sent' | 'changeRequested' | 'accepted' | 'rejected';

const STATUS_TONES: Record<StatusTone, { fill: string; dot: string }> = {
  draft: { fill: 'var(--status-draft-base)', dot: 'bg-status-draft' },
  quoted: { fill: 'var(--status-quoted-base)', dot: 'bg-status-quoted' },
  sent: { fill: 'var(--status-sent-base)', dot: 'bg-status-sent' },
  changeRequested: {
    fill: 'var(--status-change-requested-base)',
    dot: 'bg-status-change-requested',
  },
  accepted: { fill: 'var(--status-accepted-base)', dot: 'bg-status-accepted' },
  rejected: { fill: 'var(--status-rejected-base)', dot: 'bg-status-rejected' },
};

export function statusTone(status: string): StatusTone {
  switch (status) {
    case 'QUOTED':
      return 'quoted';
    case 'SENT':
      return 'sent';
    case 'CHANGE_REQUESTED':
      return 'changeRequested';
    case 'ACCEPTED':
      return 'accepted';
    case 'REJECTED':
      return 'rejected';
    default:
      return 'draft';
  }
}

export interface PieItem {
  key: string;
  label: string;
  value: number;
  tone: StatusTone;
}

function pointOnCircle(angle: number, radius: number) {
  const radians = ((angle - 90) * Math.PI) / 180;
  return [50 + radius * Math.cos(radians), 50 + radius * Math.sin(radians)];
}

function slicePath(start: number, end: number) {
  const [startX, startY] = pointOnCircle(start, 44);
  const [endX, endY] = pointOnCircle(end, 44);
  const largeArc = end - start > 180 ? 1 : 0;
  return `M 50 50 L ${startX} ${startY} A 44 44 0 ${largeArc} 1 ${endX} ${endY} Z`;
}

export function StatusPieChart({
  items,
  summary,
  ariaLabel,
  label,
  valueLabel,
  formatValue,
  formatShare,
}: {
  items: PieItem[];
  summary: string;
  ariaLabel: string;
  label: string;
  valueLabel: string;
  formatValue: (value: number) => string;
  formatShare: (ratio: number) => string;
}) {
  const total = items.reduce((sum, item) => sum + item.value, 0);
  const visible = items.filter((item) => item.value > 0);
  const soleSlice = visible.length === 1 ? visible[0] : undefined;
  let angle = 0;

  return (
    <figure className="flex flex-col items-center gap-6 sm:flex-row sm:items-center">
      <div className="shrink-0" role="img" aria-label={ariaLabel}>
        <svg viewBox="0 0 100 100" aria-hidden="true" className="size-40 sm:size-44">
          {soleSlice ? (
            <circle cx="50" cy="50" r="44" style={{ fill: STATUS_TONES[soleSlice.tone].fill }} />
          ) : (
            visible.map((item) => {
              const start = angle;
              angle += (item.value / total) * 360;
              return (
                <path
                  key={item.key}
                  d={slicePath(start, angle)}
                  style={{ fill: STATUS_TONES[item.tone].fill }}
                  stroke="var(--card)"
                  strokeWidth="1.5"
                  strokeLinejoin="round"
                />
              );
            })
          )}
        </svg>
      </div>
      <div className="flex w-full min-w-0 flex-col gap-y-3">
        <p className="text-paragraph-xs text-foreground-subtle">{summary}</p>
        <ul className="flex flex-col gap-y-2.5">
          {items.map((item) => {
            const share = total === 0 ? 0 : item.value / total;
            return (
              <li
                key={item.key}
                className="flex items-center justify-between gap-x-4 text-paragraph-sm"
              >
                <span className="inline-flex min-w-0 items-center gap-x-2">
                  <span
                    className={cn('size-2.5 shrink-0 rounded-full', STATUS_TONES[item.tone].dot)}
                    aria-hidden="true"
                  />
                  <span className="truncate text-foreground-muted">{item.label}</span>
                </span>
                <span className="shrink-0 tabular-nums text-foreground">
                  <span className="text-paragraph-sm-semibold">{formatValue(item.value)}</span>
                  <span className="ml-2 text-paragraph-xs text-foreground-subtle">
                    {formatShare(share)}
                  </span>
                </span>
              </li>
            );
          })}
        </ul>
      </div>
      <table className="sr-only">
        <caption>{ariaLabel}</caption>
        <thead>
          <tr>
            <th scope="col">{label}</th>
            <th scope="col">{valueLabel}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.key}>
              <th scope="row">{item.label}</th>
              <td>{formatValue(item.value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}

export interface RankingItem {
  key: string;
  label: string;
  value: string;
  magnitude: number;
}

export function RankingList({
  items,
  tone = 'primary',
}: {
  items: RankingItem[];
  tone?: 'primary' | 'accepted';
}) {
  const max = Math.max(1, ...items.map((item) => item.magnitude));
  return (
    <ol className="divide-y divide-border">
      {items.map((item, index) => (
        <li key={item.key} className="flex flex-col gap-y-2 px-6 py-3.5">
          <div className="flex items-center gap-x-3">
            <span className="grid size-6 shrink-0 place-items-center rounded-full bg-accent text-paragraph-xs-semibold tabular-nums text-accent-foreground">
              {index + 1}
            </span>
            <span className="min-w-0 flex-1 truncate text-paragraph-sm text-foreground">
              {item.label}
            </span>
            <span className="shrink-0 text-paragraph-sm-semibold tabular-nums text-foreground">
              {item.value}
            </span>
          </div>
          <div className="ml-9 h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden="true">
            <div
              className={cn(
                'h-full rounded-full',
                tone === 'accepted' ? 'bg-status-accepted' : 'bg-primary',
              )}
              style={{ width: `${(item.magnitude / max) * 100}%` }}
            />
          </div>
        </li>
      ))}
    </ol>
  );
}

export interface TrendPoint {
  date: string;
  rfqs: number;
  sent: number;
  accepted: number;
  conversionRate: number | null;
}

interface TrendLabels {
  rfqs: string;
  sent: string;
  accepted: string;
  conversion: string;
  bucket: string;
}

const TREND_SERIES = [
  { key: 'rfqs', className: 'bg-status-draft' },
  { key: 'sent', className: 'bg-status-sent' },
  { key: 'accepted', className: 'bg-status-accepted' },
] as const;

// Every bucket's order, send and acceptance volume, with the cohort conversion under it as a line.
export function TrendChart({
  points,
  labels,
  ariaLabel,
  scrollLabel,
  formatBucket,
  formatValue,
  formatShare,
}: {
  points: TrendPoint[];
  labels: TrendLabels;
  ariaLabel: string;
  // Names the scrollable region apart from the chart, so a screen reader does not hear it twice.
  scrollLabel: string;
  formatBucket: (date: string) => string;
  formatValue: (value: number) => string;
  formatShare: (ratio: number) => string;
}) {
  const max = Math.max(1, ...points.flatMap((point) => [point.rfqs, point.sent, point.accepted]));
  const minWidth = Math.max(420, points.length * 14);
  const axisIndexes = [...new Set([0, Math.floor((points.length - 1) / 2), points.length - 1])];
  // Each point sits over its bucket's bar group, so the line and the bars read as one axis.
  const linePoints = points.map((point, index) =>
    point.conversionRate === null
      ? null
      : {
          x: ((index + 0.5) / points.length) * 100,
          y: 100 - point.conversionRate * 100,
          title: `${formatBucket(point.date)}: ${formatShare(point.conversionRate)}`,
        },
  );
  // Buckets that sent nothing break the line rather than drawing a fake zero.
  const segments = linePoints.reduce<{ x: number; y: number; title: string }[][]>((runs, point) => {
    if (point === null) return runs.at(-1)?.length === 0 ? runs : [...runs, []];
    const last = runs.at(-1);
    if (last) last.push(point);
    else runs.push([point]);
    return runs;
  }, []);

  return (
    <div className="flex flex-col gap-y-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-paragraph-xs text-foreground-muted">
        {TREND_SERIES.map((series) => (
          <span key={series.key} className="inline-flex items-center gap-x-1.5">
            <span className={cn('size-2 rounded-full', series.className)} aria-hidden="true" />
            {labels[series.key]}
          </span>
        ))}
      </div>
      {/* Focusable so a keyboard can scroll a long period; the ring is the only cue it scrolls. */}
      <div
        tabIndex={0}
        role="group"
        aria-label={scrollLabel}
        className="overflow-x-auto rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
      >
        <div
          role="img"
          aria-label={ariaLabel}
          style={{ minWidth }}
          className="flex flex-col gap-y-2"
        >
          <div className="flex h-40 items-end gap-x-1 border-b border-border pb-px">
            {points.map((point) => (
              <div
                key={point.date}
                className="flex h-full min-w-0 flex-1 items-end justify-center gap-x-0.5"
              >
                {TREND_SERIES.map((series) => (
                  <span
                    key={series.key}
                    title={`${formatBucket(point.date)}: ${formatValue(point[series.key])} ${labels[series.key]}`}
                    className={cn('w-1/4 max-w-3 rounded-t-sm', series.className)}
                    style={{ height: `${(point[series.key] / max) * 100}%` }}
                  />
                ))}
              </div>
            ))}
          </div>
          <div className="relative h-4 text-paragraph-xs text-foreground-subtle">
            {axisIndexes.map((index) => {
              const point = points[index];
              if (!point) return null;
              const align =
                index === 0
                  ? 'left-0'
                  : index === points.length - 1
                    ? 'right-0'
                    : 'left-1/2 -translate-x-1/2';
              return (
                <span key={point.date} className={cn('absolute whitespace-nowrap', align)}>
                  {formatBucket(point.date)}
                </span>
              );
            })}
          </div>
          <div className="flex flex-col gap-y-1">
            <span className="text-paragraph-xs text-foreground-muted">{labels.conversion}</span>
            <div className="relative h-16">
              <svg
                viewBox="0 0 100 100"
                preserveAspectRatio="none"
                aria-hidden="true"
                className="absolute inset-0 size-full overflow-visible"
              >
                <line
                  x1="0"
                  y1="100"
                  x2="100"
                  y2="100"
                  stroke="var(--border)"
                  strokeWidth="1"
                  vectorEffect="non-scaling-stroke"
                />
                {segments.map((segment) =>
                  segment.length > 1 ? (
                    <polyline
                      key={`${segment[0]?.x}-${segment.length}`}
                      points={segment.map((point) => `${point.x},${point.y}`).join(' ')}
                      fill="none"
                      stroke="var(--status-accepted-base)"
                      strokeWidth="2"
                      strokeLinejoin="round"
                      vectorEffect="non-scaling-stroke"
                    />
                  ) : null,
                )}
              </svg>
              {linePoints.map((point, index) =>
                point ? (
                  <span
                    key={points[index]?.date}
                    title={point.title}
                    aria-hidden="true"
                    className="absolute size-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-status-accepted"
                    style={{ left: `${point.x}%`, top: `${point.y}%` }}
                  />
                ) : null,
              )}
            </div>
          </div>
        </div>
      </div>
      <table className="sr-only">
        <caption>{ariaLabel}</caption>
        <thead>
          <tr>
            <th scope="col">{labels.bucket}</th>
            <th scope="col">{labels.rfqs}</th>
            <th scope="col">{labels.sent}</th>
            <th scope="col">{labels.accepted}</th>
            <th scope="col">{labels.conversion}</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point) => (
            <tr key={point.date}>
              <th scope="row">{formatBucket(point.date)}</th>
              <td>{formatValue(point.rfqs)}</td>
              <td>{formatValue(point.sent)}</td>
              <td>{formatValue(point.accepted)}</td>
              <td>{point.conversionRate === null ? '—' : formatShare(point.conversionRate)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function ConfidenceBar({
  low,
  medium,
  high,
  labels,
  formatValue,
}: {
  low: number;
  medium: number;
  high: number;
  labels: { low: string; medium: string; high: string };
  formatValue: (value: number) => string;
}) {
  const total = low + medium + high;
  const segments = [
    { key: 'low', value: low, label: labels.low, className: 'bg-status-rejected' },
    { key: 'medium', value: medium, label: labels.medium, className: 'bg-warning' },
    { key: 'high', value: high, label: labels.high, className: 'bg-success' },
  ];
  return (
    <div className="flex flex-col gap-y-3">
      <div className="flex h-3 overflow-hidden rounded-full bg-muted" aria-hidden="true">
        {segments.map((segment) =>
          segment.value === 0 ? null : (
            <span
              key={segment.key}
              className={segment.className}
              style={{ width: `${(segment.value / total) * 100}%` }}
            />
          ),
        )}
      </div>
      <ul className="flex flex-wrap gap-x-5 gap-y-2">
        {segments.map((segment) => (
          <li
            key={segment.key}
            className="inline-flex items-center gap-x-1.5 text-paragraph-xs text-foreground-muted"
          >
            <span className={cn('size-2 rounded-full', segment.className)} aria-hidden="true" />
            {segment.label}
            <span className="text-paragraph-xs-semibold tabular-nums text-foreground">
              {formatValue(segment.value)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
