'use client';

import { createContext, useContext } from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';

interface RfqListContextValue {
  records: RfqRecord[];
  activeBranchId: string | null;
  userName: string;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  activeBranchId: null,
  userName: '',
});

export function RfqListProvider({
  records,
  activeBranchId,
  userName,
  children,
}: RfqListContextValue & { children: React.ReactNode }) {
  return (
    <RfqListContext.Provider value={{ records, activeBranchId, userName }}>
      {children}
    </RfqListContext.Provider>
  );
}

export function useRfqList() {
  return useContext(RfqListContext);
}
