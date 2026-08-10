'use client';

import { useMemo, useState } from 'react';
import { SlidersHorizontalIcon } from 'lucide-react';
import { useLocale, useTimeZone, useTranslations } from 'next-intl';

import { Avatar, AvatarFallback, Badge, Button, SearchInput } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { STATUS_TONE } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import type { RfqMessage } from '@/lib/api/rfqs';

/* First letters of the first two words, which is what a two-slot avatar can show. */
function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase();
}

/* A short clock time for a conversation row; the full timestamp formatter is too long for the rail. */
function shortTime(iso: string, locale: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    timeZone,
  }).format(new Date(iso));
}

export function RfqSidebar({ messages }: { messages: RfqMessage[] }) {
  const t = useTranslations('rfqs');
  const tCommon = useTranslations('common');
  const locale = useLocale();
  const timeZone = useTimeZone();
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(messages[0]?.id ?? null);

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return messages;
    return messages.filter(
      (message) =>
        message.client.toLowerCase().includes(q) || message.snippet.toLowerCase().includes(q),
    );
  }, [messages, query]);

  return (
    <aside className="hidden lg:flex w-96 shrink-0 flex-col border-r border-border bg-card">
      <div className="flex items-center gap-x-2 px-4 py-3 border-b border-border">
        <SearchInput
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onClear={() => setQuery('')}
          clearLabel={tCommon('form.clearSearch')}
          placeholder={t('sidebar.search')}
          containerClassName="min-w-0 flex-1"
        />
        <Button variant="ghost" size="icon" aria-label={t('sidebar.filter')}>
          <SlidersHorizontalIcon aria-hidden="true" />
        </Button>
      </div>

      {visible.length > 0 ? (
        <ul className="flex-1 overflow-y-auto">
          {visible.map((message) => (
            <li key={message.id}>
              <button
                type="button"
                onClick={() => setSelectedId(message.id)}
                aria-current={selectedId === message.id ? 'true' : undefined}
                className={cn(
                  'flex w-full items-start gap-x-3 px-4 py-3 text-left',
                  'transition-colors duration-150 ease-out-soft',
                  selectedId === message.id ? 'bg-accent' : 'hover:bg-muted',
                )}
              >
                <Avatar size="sm">
                  <AvatarFallback>{initials(message.client)}</AvatarFallback>
                </Avatar>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-x-2">
                    <span className="truncate text-paragraph-sm-medium text-foreground">
                      {message.client}
                    </span>
                    <span className="shrink-0 text-paragraph-mini text-foreground-subtle">
                      {shortTime(message.at, locale, timeZone)}
                    </span>
                  </span>
                  <span className="block truncate text-paragraph-mini text-foreground-muted">
                    {message.snippet}
                  </span>
                </span>
                <Badge tone={STATUS_TONE[message.status]} size="sm">
                  {t(`status.${message.status}`)}
                </Badge>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="px-4 py-6 text-paragraph-sm text-foreground-muted">
          {t('sidebar.noResults')}
        </p>
      )}
    </aside>
  );
}
