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
}: {
  items: PieItem[];
  summary: string;
  ariaLabel: string;
  label: string;
  valueLabel: string;
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
            const percent = total === 0 ? 0 : Math.round((item.value / total) * 100);
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
                  <span className="font-semibold">{item.value}</span>
                  <span className="ml-2 text-paragraph-xs text-foreground-subtle">{percent}%</span>
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
              <td>{item.value}</td>
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
            <span className="grid size-6 shrink-0 place-items-center rounded-full bg-accent text-paragraph-xs font-semibold tabular-nums text-accent-foreground">
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

export interface TimelinePoint {
  date: string;
  sent: number;
  accepted: number;
}

// Two thin bars per day: the tolerance is the gap, not a border, so the pair never touches.
export function TimelineChart({
  points,
  sentLabel,
  acceptedLabel,
  dateLabel,
  ariaLabel,
}: {
  points: TimelinePoint[];
  sentLabel: string;
  acceptedLabel: string;
  dateLabel: string;
  ariaLabel: string;
}) {
  const max = Math.max(1, ...points.flatMap((point) => [point.sent, point.accepted]));
  return (
    <div className="flex flex-col gap-y-4">
      <div className="flex items-center gap-x-4 text-paragraph-xs text-foreground-muted">
        <span className="inline-flex items-center gap-x-1.5">
          <span className="size-2 rounded-full bg-status-sent" aria-hidden="true" />
          {sentLabel}
        </span>
        <span className="inline-flex items-center gap-x-1.5">
          <span className="size-2 rounded-full bg-status-accepted" aria-hidden="true" />
          {acceptedLabel}
        </span>
      </div>
      <div className="overflow-x-auto">
        <div
          role="img"
          aria-label={ariaLabel}
          style={{ minWidth: Math.max(420, points.length * 8) }}
          className="flex h-40 items-end gap-x-1 border-b border-border pb-px"
        >
          {points.map((point) => (
            <div
              key={point.date}
              className="flex h-full min-w-0 flex-1 items-end justify-center gap-x-0.5"
            >
              <span
                title={`${point.date}: ${point.sent} ${sentLabel}`}
                className="w-1/3 max-w-3 rounded-t-sm bg-status-sent"
                style={{ height: point.sent === 0 ? 0 : `${(point.sent / max) * 100}%` }}
              />
              <span
                title={`${point.date}: ${point.accepted} ${acceptedLabel}`}
                className="w-1/3 max-w-3 rounded-t-sm bg-status-accepted"
                style={{ height: point.accepted === 0 ? 0 : `${(point.accepted / max) * 100}%` }}
              />
            </div>
          ))}
        </div>
      </div>
      <table className="sr-only">
        <caption>{ariaLabel}</caption>
        <thead>
          <tr>
            <th scope="col">{dateLabel}</th>
            <th scope="col">{sentLabel}</th>
            <th scope="col">{acceptedLabel}</th>
          </tr>
        </thead>
        <tbody>
          {points.map((point) => (
            <tr key={point.date}>
              <th scope="row">{point.date}</th>
              <td>{point.sent}</td>
              <td>{point.accepted}</td>
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
}: {
  low: number;
  medium: number;
  high: number;
  labels: { low: string; medium: string; high: string };
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
            <span className="font-semibold tabular-nums text-foreground">{segment.value}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
