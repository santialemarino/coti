'use client';

import { RfqSplitView } from '@/app/(protected)/rfqs/_components/rfq-split-view';
import { RfqDetailView } from '@/app/(protected)/rfqs/[id]/_components/rfq-detail-view';
import type { RfqDetailResponse } from '@/lib/api/rfqs';

interface RfqDetailSplitViewProps {
  detail: RfqDetailResponse;
  /* Whether the order's branch has no mailbox for a customer's reply to reach. */
  branchMissesEmail: boolean;
}

export function RfqDetailSplitView({ detail, branchMissesEmail }: RfqDetailSplitViewProps) {
  return (
    <RfqSplitView activeRfqId={detail.rfq.id}>
      <RfqDetailView detail={detail} branchMissesEmail={branchMissesEmail} />
    </RfqSplitView>
  );
}
