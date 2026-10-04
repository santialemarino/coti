import { RfqQueueGate } from '@/app/(protected)/rfqs/_components/rfq-queue-gate';

export default function RfqsLayout({ children }: { children: React.ReactNode }) {
  return <RfqQueueGate>{children}</RfqQueueGate>;
}
