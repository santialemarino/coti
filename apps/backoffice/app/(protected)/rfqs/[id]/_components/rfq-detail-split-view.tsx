'use client';

import { RfqSplitView } from '@/app/(protected)/rfqs/_components/rfq-split-view';
import { RfqDetailView } from '@/app/(protected)/rfqs/[id]/_components/rfq-detail-view';
import type { RfqDetailResponse } from '@/lib/api/rfqs';

interface RfqDetailSplitViewProps {
  detail: RfqDetailResponse;
}

export function RfqDetailSplitView({ detail }: RfqDetailSplitViewProps) {
  return (
    <RfqSplitView activeRfqId={detail.rfq.id}>
      <RfqDetailView detail={detail} />
    </RfqSplitView>
  );
}
