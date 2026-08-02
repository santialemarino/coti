// Runtime settings read from the environment, with the defaults in one place.

const DEFAULT_API_URL = 'http://localhost:8000';
const DEFAULT_REFRESH_SKEW_SECONDS = 60;

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_API_URL;

// How long before an access token expires the session is renewed, so a request
// cannot start with a token that expires mid-flight.
export const REFRESH_SKEW_SECONDS = readInt(
  process.env.AUTH_REFRESH_SKEW_SECONDS,
  DEFAULT_REFRESH_SKEW_SECONDS,
);

function readInt(raw: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(raw ?? '', 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}
