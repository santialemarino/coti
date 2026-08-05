import type { Mock } from 'vitest';

// Hand-written for the same reason config.d.ts is: the module is plain JS and the suites that
// consume it are type-checked TypeScript.

/* The subset of Next's cookie options the session and branch writers pass. */
export interface CookieJarOptions {
  maxAge?: number;
}

export interface CookieJar {
  get: Mock<(name: string) => { name: string; value: string } | undefined>;
  set: Mock<(name: string, value: string, options?: CookieJarOptions) => void>;
  delete: Mock<(name: string) => void>;
}

export declare function cookieJar(initial?: Record<string, string>): CookieJar;
