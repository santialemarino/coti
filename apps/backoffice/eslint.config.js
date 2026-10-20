import { nextJsConfig } from '@repo/eslint-config/next-js';

/** @type {import("eslint").Linter.Config[]} */
export default [
  ...nextJsConfig,
  {
    // ImageResponse requires native image elements outside the Next.js image pipeline.
    files: ['app/opengraph-image.tsx'],
    rules: { '@next/next/no-img-element': 'off' },
  },
];
