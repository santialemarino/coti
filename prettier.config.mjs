/** @see https://prettier.io/docs/en/options.html */
export default {
  printWidth: 100,
  tabWidth: 2,
  useTabs: false,
  semi: true,
  singleQuote: true,
  trailingComma: 'all',
  bracketSpacing: true,
  arrowParens: 'always',
  endOfLine: 'lf',

  plugins: ['@ianvs/prettier-plugin-sort-imports'],

  importOrder: [
    '^react$',
    '^next(/.*)?$',
    '<THIRD_PARTY_MODULES>',
    '',
    '^@repo/(.*)$',
    '^@/(.*)$',
    '^[./]',
  ],
};
