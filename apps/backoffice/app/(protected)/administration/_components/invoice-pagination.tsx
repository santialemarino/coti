'use client';

import { usePathname, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { Pagination } from '@repo/ui/components';

interface InvoicePaginationProps {
  page: number;
  pageCount: number;
}

// The history is paged on the server, so a page change is a navigation to `?page=`.
export function InvoicePagination({ page, pageCount }: InvoicePaginationProps) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useTranslations('common.pagination');

  return (
    <Pagination
      page={page}
      pageCount={pageCount}
      onPageChange={(next) => router.push(`${pathname}?page=${next}`)}
      labels={{ previous: t('previous'), next: t('next'), page: t('label') }}
    />
  );
}
