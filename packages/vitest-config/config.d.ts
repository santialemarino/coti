import type { UserConfig } from 'vitest/config';

// Hand-written because the config itself is plain JS, and the two apps consume it from a
// TypeScript vitest.config.ts that their tsconfig type-checks.
export declare const vitestConfig: UserConfig;
