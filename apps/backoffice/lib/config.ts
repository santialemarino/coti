// Runtime settings read from the environment, with the defaults in one place.

const DEFAULT_API_URL = 'http://localhost:8000';
const DEFAULT_REFRESH_SKEW_SECONDS = 60;
const DEFAULT_REMEMBERED_SESSION_DAYS = 30;
const DEFAULT_RESEND_COOLDOWN_SECONDS = 30;

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_URL;

// How long before an access token expires the session is renewed, so a request
// cannot start with a token that expires mid-flight.
export const REFRESH_SKEW_SECONDS = readInt(
  process.env.AUTH_REFRESH_SKEW_SECONDS,
  DEFAULT_REFRESH_SKEW_SECONDS,
);

// How long a remembered session's cookies survive. Mirrors the API's
// AUTH_REFRESH_REMEMBER_DAYS; the API is still what decides if the token is live.
export const REMEMBERED_SESSION_MAX_AGE_SECONDS =
  readInt(process.env.AUTH_REMEMBERED_SESSION_DAYS, DEFAULT_REMEMBERED_SESSION_DAYS) * 24 * 60 * 60;

/*
 * How many intermediaries sit in front of *this* app, used to work out the browser's address
 * so the API can rate-limit per user rather than per Next server. Zero locally, where nothing
 * is in front.
 */
export const TRUSTED_PROXY_HOPS = readInt(process.env.WEB_TRUSTED_PROXY_HOPS, 0);

/*
 * How long the "send me another link" button stays shut after a send. Read in the browser, so it
 * has to be a NEXT_PUBLIC_ key. It exists to keep a caller from spending the API's mail allowance
 * on one impatient minute — the per-address cap is 3 in a 60s window and answers 202 either way,
 * so without it the fourth click looks like it worked and no mail is sent. Tune it with that cap,
 * and keep at least two sends inside a window.
 */
export const RESEND_COOLDOWN_SECONDS = readInt(
  process.env.NEXT_PUBLIC_AUTH_RESEND_COOLDOWN_SECONDS,
  DEFAULT_RESEND_COOLDOWN_SECONDS,
);

function readInt(raw: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(raw ?? '', 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}
