// Every settings page lives under it, which the context column keys its menu on.
const SETTINGS_ROOT = '/settings';

// Every link and redirect reads from here, so a route rename is one edit.
export const ROUTES = {
  // Where a seller lands: the queue. The root belongs to the public site.
  home: '/inbox',
  landing: '/',
  // The landing's sections, linked from the footer on every public page.
  landingSection: (section: 'steps' | 'features' | 'faq') => `/#${section}`,
  privacy: '/privacy',
  terms: '/terms',
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

/*
 * The sign-in flows, reachable without a session. Which of them send a signed-in caller away is
 * SIGNED_OUT_ONLY_ROUTES; the rest have a reason to open for one (below).
 */
export const AUTH_ROUTES: readonly string[] = [
  ROUTES.login,
  ROUTES.signup,
  ROUTES.forgotPassword,
  ROUTES.resetPassword,
  ROUTES.verifyEmail,
  ROUTES.sessionEnded,
];

// The public site: the same pages for everyone, signed in or not.
export const PUBLIC_ROUTES: readonly string[] = [ROUTES.landing, ROUTES.privacy, ROUTES.terms];

/*
 * The auth routes a signed-in caller has no business on, so the gate sends them on instead.
 * verify-email is not one: signup hands the caller a session, so the most common way to reach it is
 * already logged in. Nor is reset-password: a mailed link (a recovery, an invite) has to open in
 * whatever browser the mail is read in. session-ended exists for a caller who still looks signed in.
 */
export const SIGNED_OUT_ONLY_ROUTES: readonly string[] = [
  ROUTES.login,
  ROUTES.signup,
  ROUTES.forgotPassword,
];

/*
 * The routes the gate never renews a session on. session-ended's whole job is clearing the cookies
 * of a caller who still looks signed in; renewing them first would undo it.
 */
export const SESSION_CLEARING_ROUTES: readonly string[] = [ROUTES.sessionEnded];

/*
 * Which of the three surfaces a path belongs to: the public site, the sign-in screens (the 404 shares
 * their frame) or the app. Crossing from one to another is a page-level change, animated as one.
 */
export function surfaceOf(pathname: string): 'public' | 'auth' | 'app' {
  if (PUBLIC_ROUTES.includes(pathname)) return 'public';
  if (isProtectedPath(pathname)) return 'app';
  return 'auth';
}

// The app's own route handlers, which answer only a signed-in caller.
const API_ROOT = '/api';

/*
 * Everything in ROUTES that is neither an auth route nor the public site, plus the route handlers.
 * Computed rather than listed, so a new route is protected the moment it is registered.
 */
export const PROTECTED_ROUTES: readonly string[] = [
  ...Object.values(ROUTES).flatMap((route) =>
    typeof route === 'string' && !AUTH_ROUTES.includes(route) && !PUBLIC_ROUTES.includes(route)
      ? [route]
      : [],
  ),
  API_ROOT,
];

/*
 * Whether a path is one the gate guards. Derived from ROUTES, so a new route is guarded the moment it
 * is registered; a path that matches nothing is not, and reaches the 404 instead of the login screen.
 */
export function isProtectedPath(pathname: string): boolean {
  return PROTECTED_ROUTES.some((root) => pathname === root || pathname.startsWith(`${root}/`));
}

export const LOGIN_ROUTE = ROUTES.login;

export const NEXT_PARAM = 'next';

// Why a session ended, carried through session-ended to the login screen so it can say so. Only
// the known value is ever forwarded.
export const REASON_PARAM = 'reason';
export const LOCKED_REASON = 'locked';

/*
 * Where to send the caller after they log in: a same-origin path to a page the gate guards, which
 * is the only kind `next` is ever produced for. Resolving against a throwaway origin is what makes
 * it sound: `/\evil.com` and `/\/evil.com` both survive a startsWith check, because the URL parser
 * treats a backslash as a slash and reads them as a host.
 */
export function safeNextPath(raw: string | null | undefined): string {
  if (!raw) return ROUTES.home;
  const resolved = URL.parse(raw, SAME_ORIGIN);
  if (!resolved || resolved.origin !== SAME_ORIGIN || !isProtectedPath(resolved.pathname)) {
    return ROUTES.home;
  }
  return resolved.pathname + resolved.search;
}

const SAME_ORIGIN = 'https://coti.invalid';
