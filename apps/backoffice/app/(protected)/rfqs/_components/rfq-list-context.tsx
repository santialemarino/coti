'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';
import { fetchQueue } from '@/lib/api/rfqs-client';

type RfqRecordPatch = Partial<RfqRecord> | ((record: RfqRecord) => RfqRecord);

// Whether the archived orders are in the list: never asked for, on their way, in, or refused.
export type ArchivedState = 'idle' | 'loading' | 'loaded' | 'failed';

interface RfqListContextValue {
  records: RfqRecord[];
  // False for a seller assigned to no branch, who has no queue to show.
  hasQueue: boolean;
  // True when the server could not read the queue; the queue screens then fail as a page would.
  loadFailed: boolean;
  activeBranchId: string | null;
  userName: string;
  // The signed-in user's id and role; the list rows stamp an assignment with them.
  userId: string;
  isAdmin: boolean;
  // The account's invoicing switch and its «Facturar también» default, for the closing actions.
  invoicingEnabled: boolean;
  invoiceOnPayment: boolean;
  archivedState: ArchivedState;
  updateRecord: (id: string, patch: RfqRecordPatch) => void;
  // Re-reads the queue; a failed read keeps the list already on screen.
  reload: () => Promise<void>;
  // Brings the archived orders into the list, and keeps them in on every later re-read.
  requestArchived: () => void;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  hasQueue: false,
  loadFailed: false,
  activeBranchId: null,
  userName: '',
  userId: '',
  isAdmin: false,
  invoicingEnabled: false,
  invoiceOnPayment: false,
  archivedState: 'idle',
  updateRecord: () => undefined,
  reload: async () => undefined,
  requestArchived: () => undefined,
});

type RfqListProviderProps = Omit<
  RfqListContextValue,
  | 'updateRecord'
  | 'reload'
  | 'requestArchived'
  | 'loadFailed'
  | 'archivedState'
  | 'invoicingEnabled'
  | 'invoiceOnPayment'
> & {
  loadFailed?: boolean;
  invoicingEnabled?: boolean;
  invoiceOnPayment?: boolean;
  children: React.ReactNode;
};

/*
 * The server reads the open queue only; the archive is a handful of rows nobody needs until the
 * table's «Archivado» filter asks for it, and from then on every re-read brings it along.
 */
export function RfqListProvider({
  records: initialRecords,
  hasQueue,
  loadFailed: initialLoadFailed = false,
  activeBranchId,
  userName,
  userId,
  isAdmin,
  invoicingEnabled = false,
  invoiceOnPayment = false,
  children,
}: RfqListProviderProps) {
  const [records, setRecords] = useState(initialRecords);
  const [loadFailed, setLoadFailed] = useState(initialLoadFailed);
  const [archivedState, setArchivedState] = useState<ArchivedState>('idle');
  // Bumped by every newer source of truth, so a re-read that lands after one is dropped instead of
  // overwriting it — a branch switch's list, or a row a write just patched.
  const generation = useRef(0);
  const withArchived = useRef(false);
  // Whether the list on screen holds the archive, which a server refresh drops.
  const archivedIn = useRef(false);

  const reload = useCallback(async () => {
    if (!hasQueue) return;
    const ticket = ++generation.current;
    const includeArchived = withArchived.current;
    try {
      const next = await fetchQueue(includeArchived);
      if (ticket !== generation.current) {
        // A row patched meanwhile drops this read, but the archive it carried is still owed.
        if (includeArchived && !archivedIn.current) void reload();
        return;
      }
      setRecords(next);
      setLoadFailed(false);
      if (includeArchived) {
        archivedIn.current = true;
        setArchivedState('loaded');
      }
    } catch {
      // The list on screen is still the last one the server gave; only the archive can be missing,
      // and then it is asked for again only when someone retries.
      if (includeArchived && !archivedIn.current) {
        withArchived.current = false;
        setArchivedState('failed');
      }
    }
  }, [hasQueue]);

  // The server's list carries no archive, so a list that had one reads it again.
  useEffect(() => {
    generation.current += 1;
    setRecords(initialRecords);
    archivedIn.current = false;
    setLoadFailed(initialLoadFailed);
    if (withArchived.current) {
      setArchivedState('loading');
      void reload();
    }
  }, [initialRecords, initialLoadFailed, reload]);

  const updateRecord = useCallback((id: string, patch: RfqRecordPatch) => {
    generation.current += 1;
    setRecords((previous) =>
      previous.map((record) => {
        if (record.id !== id) return record;
        return typeof patch === 'function' ? patch(record) : { ...record, ...patch };
      }),
    );
  }, []);

  const requestArchived = useCallback(() => {
    if (withArchived.current) return;
    withArchived.current = true;
    setArchivedState('loading');
    void reload();
  }, [reload]);

  const value = useMemo(
    () => ({
      records,
      hasQueue,
      loadFailed,
      activeBranchId,
      userName,
      userId,
      isAdmin,
      invoicingEnabled,
      invoiceOnPayment,
      archivedState,
      updateRecord,
      reload,
      requestArchived,
    }),
    [
      activeBranchId,
      archivedState,
      hasQueue,
      invoiceOnPayment,
      invoicingEnabled,
      loadFailed,
      records,
      updateRecord,
      reload,
      requestArchived,
      userName,
      userId,
      isAdmin,
    ],
  );

  return <RfqListContext.Provider value={value}>{children}</RfqListContext.Provider>;
}

export function useRfqList() {
  return useContext(RfqListContext);
}
