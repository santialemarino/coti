import { Inter, Poppins } from 'next/font/google';

/*
 * Two faces, applied as CSS variables that --font-sans and --font-display resolve to in the design
 * system's theme. Inter is the workhorse: it holds up at the 11–12px the quote and catalog tables
 * run at, where the display face does not. Poppins is geometric like the wordmark and carries the
 * heading scale, which is why every text-heading-* utility sets the family itself.
 *
 * `latin` covers the accents and inverted punctuation Argentine Spanish needs (á é í ó ú ñ ¿ ¡).
 * Poppins is not a variable font on Google Fonts, so its weights are enumerated; only the two the
 * heading scale and the wordmark actually use are fetched.
 */
export const inter = Inter({
  subsets: ['latin'],
  variable: '--font-inter',
  display: 'swap',
});

export const poppins = Poppins({
  subsets: ['latin'],
  weight: ['500', '600'],
  variable: '--font-poppins',
  display: 'swap',
});
