'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';
import { fetchQueue } from '@/lib/api/rfqs-client';

type RfqRecordPatch = Partial<RfqRecord> | ((record: RfqRecord) => RfqRecord);

interface RfqListContextValue {
  records: RfqRecord[];
  // False for a seller assigned to no branch, who has no queue to show.
  hasQueue: boolean;
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
  activeBranchId,
  userName,
  userId,
  isAdmin,
  children,
}: Omit<RfqListContextValue, 'updateRecord' | 'reload'> & { children: React.ReactNode }) {
  const [records, setRecords] = useState(initialRecords);

  useEffect(() => {
    setRecords(initialRecords);
  }, [initialRecords]);

  const updateRecord = useCallback((id: string, patch: RfqRecordPatch) => {
    setRecords((previous) =>
      previous.map((record) => {
        if (record.id !== id) return record;
        return typeof patch === 'function' ? patch(record) : { ...record, ...patch };
      }),
    );
  }, []);

  const reload = useCallback(async () => {
    if (!hasQueue) return;
    try {
      setRecords(await fetchQueue());
    } catch {
      // Nothing to add: the list on screen is still the last one the server gave.
    }
  }, [hasQueue]);

  const value = useMemo(
    () => ({
      records,
      hasQueue,
      activeBranchId,
      userName,
      userId,
      isAdmin,
      updateRecord,
      reload,
    }),
    [activeBranchId, hasQueue, records, updateRecord, reload, userName, userId, isAdmin],
  );

  return <RfqListContext.Provider value={value}>{children}</RfqListContext.Provider>;
}

export function useRfqList() {
  return useContext(RfqListContext);
}
