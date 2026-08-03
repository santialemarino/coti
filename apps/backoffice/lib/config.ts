// Runtime settings read from the environment, with the defaults in one place.

const DEFAULT_API_URL = 'http://localhost:8000';
const DEFAULT_REFRESH_SKEW_SECONDS = 60;
const DEFAULT_REMEMBERED_SESSION_DAYS = 30;

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

function readInt(raw: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(raw ?? '', 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}
