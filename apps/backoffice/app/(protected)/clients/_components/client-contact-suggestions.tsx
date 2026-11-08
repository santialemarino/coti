'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { UsersIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Card, EmptyState, MetaList, Skeleton } from '@repo/ui/components';
import { ClientContactSuggestionDialog } from '@/app/(protected)/clients/_components/client-contact-suggestion-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { ROUTES } from '@/config/routes';
import {
  mapContactSuggestion,
  type ContactSuggestion,
  type ContactSuggestionRaw,
} from '@/lib/api/client-contact-suggestions';

interface ClientContactSuggestionsProps {
  compact?: boolean;
}

export function ClientContactSuggestions({ compact = false }: ClientContactSuggestionsProps) {
  const { activeBranchId } = useRfqList();
  return <ScopedSuggestions key={activeBranchId ?? 'all'} compact={compact} />;
}

function ScopedSuggestions({ compact = false }: ClientContactSuggestionsProps) {
  const t = useTranslations('clientContactSuggestions');
  const pathname = usePathname();
  const { activeBranchId } = useRfqList();
  const [items, setItems] = useState<ContactSuggestion[] | null>(null);
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [selected, setSelected] = useState<ContactSuggestion | null>(null);

  useEffect(() => {
    if (!pathname.startsWith(ROUTES.clients)) return;
    const controller = new AbortController();
    const reload = async () => {
      try {
        const response = await fetch('/api/client-contact-suggestions', {
          cache: 'no-store',
          headers: activeBranchId ? { 'X-Branch-Id': activeBranchId } : {},
          signal: controller.signal,
        });
        if (!response.ok) throw new Error('Suggestion load failed');
        const data = ((await response.json()) as ContactSuggestionRaw[]).map(mapContactSuggestion);
        if (!controller.signal.aborted) {
          setItems(data);
          setError(false);
        }
      } catch {
        if (!controller.signal.aborted) setError(true);
      }
    };
    void reload();
    window.addEventListener('focus', reload);
    window.addEventListener('client-contact-suggestions-changed', reload);
    return () => {
      controller.abort();
      window.removeEventListener('focus', reload);
      window.removeEventListener('client-contact-suggestions-changed', reload);
    };
  }, [pathname, activeBranchId, revision]);

  if (error)
    return (
      <div className="flex flex-col p-6 gap-y-3">
        <p role="alert" className="text-paragraph-sm text-danger-foreground">
          {t('loadError')}
        </p>
        <Button variant="outline" onClick={() => setRevision((value) => value + 1)}>
          {t('retry')}
        </Button>
      </div>
    );
  if (items === null)
    return (
      <div className="flex flex-col p-6 gap-y-3">
        <Skeleton className="w-full h-16" />
        <Skeleton className="w-full h-16" />
      </div>
    );
  if (compact)
    return (
      <div className="flex flex-col px-6 pb-6 gap-y-3">
        <Button variant="outline" asChild>
          <Link href={ROUTES.clientSuggestions}>{t('pending', { count: items.length })}</Link>
        </Button>
        {items.map((item) => (
          <Button key={item.id} variant="ghost" asChild className="justify-start">
            <Link href={`${ROUTES.clientSuggestions}#${item.id}`}>
              <span className="truncate">{item.candidateName ?? item.taxId ?? item.value}</span>
            </Link>
          </Button>
        ))}
      </div>
    );
  return (
    <>
      <div className="flex flex-col gap-y-4">
        {items.length === 0 ? (
          <EmptyState icon={UsersIcon} title={t('empty')} />
        ) : (
          items.map((item) => (
            <Card
              key={item.id}
              id={item.id}
              className="flex-row flex-wrap items-center justify-between px-6 gap-4"
            >
              <div className="flex flex-col gap-y-1">
                <h2 className="text-paragraph-sm-medium">
                  {item.candidateName ?? item.declaredName ?? t('unknown')}
                </h2>
                <MetaList
                  items={[item.sourceName, item.taxId, item.value]}
                  className="wrap-anywhere"
                />
              </div>
              <Button
                variant="outline"
                onClick={() => {
                  setSelected(item);
                }}
              >
                {t('review')}
              </Button>
            </Card>
          ))
        )}
      </div>
      <ClientContactSuggestionDialog
        suggestion={selected}
        branchId={activeBranchId}
        onClose={() => setSelected(null)}
        onResolved={() => {
          setSelected(null);
          window.dispatchEvent(new Event('client-contact-suggestions-changed'));
        }}
      />
    </>
  );
}
