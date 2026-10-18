// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  home: '/',
  rfq: (channelId: string) => `/rfq/${channelId}`,
  rfqSubmit: (channelId: string) => `/api/public-rfqs/${channelId}`,
  quote: (token: string) => `/quotes/${token}`,
} as const;
