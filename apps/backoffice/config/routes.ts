// Every settings page lives under it, which the context column keys its menu on.
const SETTINGS_ROOT = '/settings';

// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  home: '/',
  settings: SETTINGS_ROOT,
  rfqs: '/rfqs',
  rfqsDetail: (id: string) => `/rfqs/${id}`,
  clients: '/clients',
  clientDetail: (id: string) => `/clients/${id}`,
  reports: '/reports',
  administration: '/administration',
  login: '/login',
  signup: '/signup',
  forgotPassword: '/forgot-password',
  resetPassword: '/reset-password',
  verifyEmail: '/verify-email',
  onboarding: '/onboarding',
  sessionEnded: '/session-ended',
  changePassword: `${SETTINGS_ROOT}/password`,
  emailSettings: `${SETTINGS_ROOT}/email`,
  accountSettings: `${SETTINGS_ROOT}/account`,
  arcaSettings: `${SETTINGS_ROOT}/arca`,
  priceSettings: `${SETTINGS_ROOT}/prices`,
  catalogSettings: `${SETTINGS_ROOT}/catalog`,
  branchSettings: `${SETTINGS_ROOT}/branches`,
  userSettings: `${SETTINGS_ROOT}/users`,
  onboardingSettings: `${SETTINGS_ROOT}/onboarding`,
  branchReset: '/branch-reset',
} as const;

const QUEUE_DETAIL = /^\/rfqs\/([^/]+)$/;

/*
 * Whether a path is one of the queue's screens — its landing or an order opened from it — and
 * which order it has open. The table at /rfqs lists the same orders but is not the queue.
 */
export function queueSelection(pathname: string): { inQueue: boolean; rfqId: string | null } {
  if (pathname === ROUTES.home) return { inQueue: true, rfqId: null };
  const detail = QUEUE_DETAIL.exec(pathname);
  return detail ? { inQueue: true, rfqId: detail[1] ?? null } : { inQueue: false, rfqId: null };
}

// Whether a path is one of the settings pages, their index included.
export function isSettingsPath(pathname: string): boolean {
  return pathname === SETTINGS_ROOT || pathname.startsWith(`${SETTINGS_ROOT}/`);
}

// The orders section: the queue, its table and every order, which "Pedidos" stays lit across.
export function isOrdersPath(pathname: string): boolean {
  return (
    pathname === ROUTES.home || pathname === ROUTES.rfqs || pathname.startsWith(`${ROUTES.rfqs}/`)
  );
}

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
];

// verify-email is public but not signed-out-only: signup hands the caller a session, so the
// most common way to reach it is already logged in. reset-password is neither: a mailed link
// (a recovery, an invite) has to open in whatever browser the mail is read in.

export const LOGIN_ROUTE = ROUTES.login;

export const NEXT_PARAM = 'next';

// Why a session ended, carried through session-ended to the login screen so it can say so. Only
// the known value is ever forwarded.
export const REASON_PARAM = 'reason';
export const LOCKED_REASON = 'locked';

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
