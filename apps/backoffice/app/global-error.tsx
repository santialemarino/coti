'use client';

import { TriangleAlertIcon } from 'lucide-react';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { Brand } from '@/components/brand';
import { inter, poppins } from '@/lib/fonts';
import { GLOBAL_ERROR_COPY } from '@/lib/i18n/global-error-copy';

import './globals.css';

/*
 * Replaces the root layout when that layout itself fails, so it brings the document, the fonts and
 * its copy (`GLOBAL_ERROR_COPY`, since no translation provider sits above it). A router refresh would
 * re-run what just broke, so its retry only resets.
 */
export default function GlobalError({ reset }: { error: Error; reset: () => void }) {
  return (
    <html className={cn(inter.variable, poppins.variable)} lang="es">
      <body className="min-h-screen bg-body-background font-sans text-paragraph text-foreground antialiased">
        <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
          <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8">
            <Brand variant="lockup" size="xl" label={GLOBAL_ERROR_COPY.appName} />
            <Card>
              <StatusScreen icon={TriangleAlertIcon} tone="danger" title={GLOBAL_ERROR_COPY.title}>
                <Button onClick={reset} size="lg">
                  {GLOBAL_ERROR_COPY.retry}
                </Button>
              </StatusScreen>
            </Card>
          </div>
        </main>
      </body>
    </html>
  );
}
