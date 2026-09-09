'use client';

import { createContext, useContext } from 'react';

import type { RfqRecord } from '@/lib/api/rfqs';

interface RfqListContextValue {
  records: RfqRecord[];
  activeBranchId: string | null;
  userName: string;
  // The signed-in user's id and role; the list rows stamp an assignment with them.
  userId: string;
  isAdmin: boolean;
}

const RfqListContext = createContext<RfqListContextValue>({
  records: [],
  activeBranchId: null,
  userName: '',
  userId: '',
  isAdmin: false,
});

export function RfqListProvider({
  records,
  activeBranchId,
  userName,
  userId,
  isAdmin,
  children,
}: RfqListContextValue & { children: React.ReactNode }) {
  return (
    <RfqListContext.Provider value={{ records, activeBranchId, userName, userId, isAdmin }}>
      {children}
    </RfqListContext.Provider>
  );
}

export function useRfqList() {
  return useContext(RfqListContext);
}
