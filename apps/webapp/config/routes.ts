// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  home: '/',
  login: '/login',
  signup: '/signup',
  forgotPassword: '/forgot-password',
  resetPassword: '/reset-password',
  verifyEmail: '/verify-email',
  googleComplete: '/google-complete',
  googleStart: '/api/customer/google/start',
  orders: '/orders',
  profile: '/profile',
  verifyContact: '/verify-contact',
  order: (id: string) => `/orders/${id}`,
  newRequest: '/new-request',
  rfq: (channelId: string) => `/rfq/${channelId}`,
  rfqSubmit: (channelId: string) => `/api/public-rfqs/${channelId}`,
  quote: (token: string) => `/quotes/${token}`,
} as const;
