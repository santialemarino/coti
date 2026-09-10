'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';

type RfqRecordPatch = Partial<RfqRecord> | ((record: RfqRecord) => RfqRecord);

interface RfqListContextValue {
  records: RfqRecord[];
  activeBranchId: string | null;
  userName: string;
  // The signed-in user's id and role; the list rows stamp an assignment with them.
  userId: string;
  isAdmin: boolean;
  updateRecord: (id: string, patch: RfqRecordPatch) => void;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  activeBranchId: null,
  userName: '',
  userId: '',
  isAdmin: false,
  updateRecord: () => undefined,
});

export function RfqListProvider({
  records: initialRecords,
  activeBranchId,
  userName,
  userId,
  isAdmin,
  children,
}: Omit<RfqListContextValue, 'updateRecord'> & { children: React.ReactNode }) {
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

  const value = useMemo(
    () => ({ records, activeBranchId, userName, userId, isAdmin, updateRecord }),
    [activeBranchId, records, updateRecord, userName, userId, isAdmin],
  );

  return <RfqListContext.Provider value={value}>{children}</RfqListContext.Provider>;
}

export function useRfqList() {
  return useContext(RfqListContext);
}
