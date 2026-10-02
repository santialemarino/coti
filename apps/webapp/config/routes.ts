// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  home: '/',
  quote: (token: string) => `/quotes/${token}`,
} as const;
