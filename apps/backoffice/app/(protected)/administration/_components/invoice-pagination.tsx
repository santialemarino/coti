'use client';

import { usePathname, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { Pagination } from '@repo/ui/components';
import type { AdministrationView } from '@/lib/constants/administration';

interface InvoicePaginationProps {
  page: number;
  pageCount: number;
  view?: AdministrationView;
}

// Both views are paged on the server, so a page change is a navigation to `?page=` in the same view.
export function InvoicePagination({ page, pageCount, view = 'history' }: InvoicePaginationProps) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useTranslations('common.pagination');

  return (
    <Pagination
      page={page}
      pageCount={pageCount}
      onPageChange={(next) =>
        router.push(
          view === 'history' ? `${pathname}?page=${next}` : `${pathname}?view=${view}&page=${next}`,
        )
      }
      labels={{ previous: t('previous'), next: t('next'), page: t('label') }}
    />
  );
}
