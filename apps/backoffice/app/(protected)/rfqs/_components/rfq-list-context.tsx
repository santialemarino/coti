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
  updateRecord: (id: string, patch: RfqRecordPatch) => void;
  // Re-reads the queue; a failed read keeps the list already on screen.
  reload: () => Promise<void>;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  hasQueue: false,
  loadFailed: false,
  activeBranchId: null,
  userName: '',
  userId: '',
  isAdmin: false,
  updateRecord: () => undefined,
  reload: async () => undefined,
});

export function RfqListProvider({
  records: initialRecords,
  hasQueue,
  loadFailed: initialLoadFailed = false,
  activeBranchId,
  userName,
  userId,
  isAdmin,
  children,
}: Omit<RfqListContextValue, 'updateRecord' | 'reload' | 'loadFailed'> & {
  loadFailed?: boolean;
  children: React.ReactNode;
}) {
  const [records, setRecords] = useState(initialRecords);
  const [loadFailed, setLoadFailed] = useState(initialLoadFailed);
  // Bumped by every newer source of truth, so a re-read that lands after one is dropped instead of
  // overwriting it — a branch switch's list, or a row a write just patched.
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    setRecords(initialRecords);
    setLoadFailed(initialLoadFailed);
  }, [initialRecords, initialLoadFailed]);

  const updateRecord = useCallback((id: string, patch: RfqRecordPatch) => {
    generation.current += 1;
    setRecords((previous) =>
      previous.map((record) => {
        if (record.id !== id) return record;
        return typeof patch === 'function' ? patch(record) : { ...record, ...patch };
      }),
    );
  }, []);

  const reload = useCallback(async () => {
    if (!hasQueue) return;
    const ticket = ++generation.current;
    try {
      const next = await fetchQueue();
      if (ticket !== generation.current) return;
      setRecords(next);
      setLoadFailed(false);
    } catch {
      // Nothing to add: the list on screen is still the last one the server gave.
    }
  }, [hasQueue]);

  const value = useMemo(
    () => ({
      records,
      hasQueue,
      loadFailed,
      activeBranchId,
      userName,
      userId,
      isAdmin,
      updateRecord,
      reload,
    }),
    [
      activeBranchId,
      hasQueue,
      loadFailed,
      records,
      updateRecord,
      reload,
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
