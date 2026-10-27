import { cn } from '@repo/ui/lib';

export interface BarItem {
  label: string;
  value: number;
  tone?: 'sent' | 'accepted' | 'neutral';
}

const TONES: Record<NonNullable<BarItem['tone']>, string> = {
  sent: 'bg-status-sent',
  accepted: 'bg-status-accepted',
  neutral: 'bg-primary',
};

// Horizontal bars keep long status names legible where a donut would not.
export function HorizontalBars({ items }: { items: BarItem[] }) {
  const max = Math.max(1, ...items.map((item) => item.value));
  return (
    <ul className="flex flex-col gap-y-4">
      {items.map((item) => (
        <li key={item.label} className="flex flex-col gap-y-1.5">
          <div className="flex items-baseline justify-between gap-x-4">
            <span className="text-paragraph-sm text-foreground">{item.label}</span>
            <span className="text-paragraph-sm-semibold tabular-nums text-foreground">
              {item.value}
            </span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-muted" aria-hidden="true">
            <div
              className={cn('h-full rounded-full', TONES[item.tone ?? 'neutral'])}
              style={{ width: `${(item.value / max) * 100}%` }}
            />
          </div>
        </li>
      ))}
    </ul>
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
