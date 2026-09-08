'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';

type RfqRecordPatch = Partial<RfqRecord> | ((record: RfqRecord) => RfqRecord);

interface RfqListContextValue {
  records: RfqRecord[];
  activeBranchId: string | null;
  userName: string;
  updateRecord: (id: string, patch: RfqRecordPatch) => void;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  activeBranchId: null,
  userName: '',
  updateRecord: () => undefined,
});

export function RfqListProvider({
  records: initialRecords,
  activeBranchId,
  userName,
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
    () => ({ records, activeBranchId, userName, updateRecord }),
    [activeBranchId, records, updateRecord, userName],
  );

  return <RfqListContext.Provider value={value}>{children}</RfqListContext.Provider>;
}

export function useRfqList() {
  return useContext(RfqListContext);
}
