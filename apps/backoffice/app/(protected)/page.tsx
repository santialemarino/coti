import { RfqQueueEmpty } from '@/app/(protected)/_components/rfq-queue-empty';
import { RfqQueueProvider } from '@/app/(protected)/rfqs/_components/rfq-queue-provider';
import { RfqSplitView } from '@/app/(protected)/rfqs/_components/rfq-split-view';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('home');

/*
 * Home is the queue with nothing selected. Triaging orders is what the day is, so landing anywhere
 * else costs a click before the work starts.
 */
export default function HomePage() {
  return (
    <RfqQueueProvider>
      <RfqSplitView activeRfqId={null}>
        <RfqQueueEmpty />
      </RfqSplitView>
    </RfqQueueProvider>
  );
}
