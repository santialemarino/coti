import { RfqQueueProvider } from '@/app/(protected)/rfqs/_components/rfq-queue-provider';

export default function RfqsLayout({ children }: { children: React.ReactNode }) {
  return <RfqQueueProvider>{children}</RfqQueueProvider>;
}
