import type { Metadata, Viewport } from 'next';
import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';

import { cn } from '@repo/ui/lib';
import { Toaster } from '@/components/toaster';
import { inter, poppins } from '@/lib/fonts';

import './globals.css';

export const metadata: Metadata = {
  title: 'Coti — Backoffice',
  description: 'Vendor and admin workspace for AI-assisted quoting.',
  applicationName: 'Coti',
};

/*
 * The browser chrome takes the app's own surface rather than the brand blue: iOS tints from the page
 * background and ignores a custom theme-color, so a tinted bar would only ever appear on Android.
 *
 * Next serialises this into a meta tag at build time, so it cannot read a CSS variable — this is the
 * one place a colour is a literal. It is `--body-background` and must be changed with it; manifest.ts
 * carries the same value.
 */
export const viewport: Viewport = {
  themeColor: '#F2F7FB',
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html className={cn(inter.variable, poppins.variable)} lang={locale}>
      <body className="min-h-screen bg-body-background font-sans text-paragraph text-foreground antialiased">
        <NextIntlClientProvider messages={messages}>
          {children}
          <Toaster />
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
