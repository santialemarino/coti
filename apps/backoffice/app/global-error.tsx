'use client';

import { TriangleAlertIcon } from 'lucide-react';

import { Button, Card, StatusScreen } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { Brand } from '@/components/brand';
import { inter, poppins } from '@/lib/fonts';
import messages from '@/translations/es.json';

import './globals.css';

/*
 * Replaces the root layout when that layout itself fails, so it brings the document, the fonts and
 * its copy: no translation provider sits above it, and a router refresh would re-run what just broke.
 */
export default function GlobalError({ reset }: { error: Error; reset: () => void }) {
  return (
    <html className={cn(inter.variable, poppins.variable)} lang="es">
      <body className="min-h-screen bg-body-background font-sans text-paragraph text-foreground antialiased">
        <main className="flex flex-col min-h-screen items-center justify-center px-4 py-10">
          <div className="flex flex-col w-full max-w-auth-card items-center gap-y-8">
            <Brand variant="lockup" size="xl" label={messages.common.appName} />
            <Card>
              <StatusScreen icon={TriangleAlertIcon} tone="danger" title={messages.errors.INTERNAL}>
                <Button onClick={reset} size="lg">
                  {messages.common.retry}
                </Button>
              </StatusScreen>
            </Card>
          </div>
        </main>
      </body>
    </html>
  );
}
