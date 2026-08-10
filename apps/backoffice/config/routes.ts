// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  home: '/',
  rfqs: '/rfqs',
  rfqsDetail: (id: string) => `/rfqs/${id}`,
  clients: '/clients',
  reports: '/reports',
  administration: '/administration',
  login: '/login',
  signup: '/signup',
  forgotPassword: '/forgot-password',
  resetPassword: '/reset-password',
  verifyEmail: '/verify-email',
  sessionEnded: '/session-ended',
  changePassword: '/settings/password',
  priceSettings: '/settings/prices',
  branchSettings: '/settings/branches',
  userSettings: '/settings/users',
} as const;

// Reachable without a session. Anything else is behind the gate.
export const PUBLIC_ROUTES: readonly string[] = [
  ROUTES.login,
  ROUTES.signup,
  ROUTES.forgotPassword,
  ROUTES.resetPassword,
  ROUTES.verifyEmail,
  ROUTES.sessionEnded,
];

/*
 * The public routes a signed-in caller has no business on, so the gate sends them
 * home instead. session-ended is deliberately absent: its whole job is to clear the
 * cookies of a caller who still looks signed in, and bouncing it would loop.
 */
export const SIGNED_OUT_ONLY_ROUTES: readonly string[] = [
  ROUTES.login,
  ROUTES.signup,
  ROUTES.forgotPassword,
  ROUTES.resetPassword,
];

// verify-email is public but not signed-out-only: signup hands the caller a session, so the
// most common way to reach it is already logged in.

export const LOGIN_ROUTE = ROUTES.login;

export const NEXT_PARAM = 'next';

/*
 * Where to send the caller after they log in, accepting same-origin paths only.
 * Resolving against a throwaway origin is what makes it sound: `/\evil.com` and
 * `/\/evil.com` both survive a startsWith check, because the URL parser treats a
 * backslash as a slash and reads them as a host.
 */
export function safeNextPath(raw: string | null | undefined): string {
  if (!raw) return ROUTES.home;
  const resolved = URL.parse(raw, SAME_ORIGIN);
  if (!resolved || resolved.origin !== SAME_ORIGIN) return ROUTES.home;
  return resolved.pathname + resolved.search;
}

const SAME_ORIGIN = 'https://coti.invalid';
