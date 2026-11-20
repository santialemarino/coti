// Coti's plans as the landing quotes them: ARS per month, as decimal strings like every amount.
export interface Plan {
  key: 'uno' | 'pro' | 'red';
  monthlyPrice: string;
  quotesPerMonth: number;
  branches: number;
  // Red alone grows past its included branches, each one adding a fee and quotations to the pool.
  extraBranch?: { monthlyPrice: string; quotesPerMonth: number };
}

export const PLANS: readonly Plan[] = [
  { key: 'uno', monthlyPrice: '150000', quotesPerMonth: 300, branches: 1 },
  { key: 'pro', monthlyPrice: '270000', quotesPerMonth: 900, branches: 1 },
  {
    key: 'red',
    monthlyPrice: '390000',
    quotesPerMonth: 1500,
    branches: 2,
    extraBranch: { monthlyPrice: '120000', quotesPerMonth: 750 },
  },
];

// What each quotation past a plan's monthly allowance costs, the same on every plan.
export const EXTRA_QUOTE_PRICE = '900';
