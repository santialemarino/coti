import type { Metadata } from 'next';

import './globals.css';

export const metadata: Metadata = {
  title: 'Coti — Backoffice',
  description: 'Vendor and admin workspace for AI-assisted quoting.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="es">
      <body>{children}</body>
    </html>
  );
}
