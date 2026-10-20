// The recipes the conversation handler can run for a complex goal, as the API names them.
export const HANDLER_STRATEGIES = [
  'FIND_CHEAPER_ALTERNATIVES',
  'ADAPT_TO_REACH_DISCOUNT',
  'IMPROVE_QUALITY',
] as const;

export type HandlerStrategy = (typeof HANDLER_STRATEGIES)[number];

// The two ways to a lower total an account can put first, in the order the setting offers them.
export const LOWER_PRICE_STRATEGIES = [
  'FIND_CHEAPER_ALTERNATIVES',
  'ADAPT_TO_REACH_DISCOUNT',
] as const satisfies readonly HandlerStrategy[];

export type LowerPriceStrategy = (typeof LOWER_PRICE_STRATEGIES)[number];
